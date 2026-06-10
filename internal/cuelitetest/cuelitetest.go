// Package cuelitetest is the differential-testing harness behind the
// cue/cuelite façade. It runs one schema-plus-data case through two
// validation paths — the in-house path (the cuelite façade, the
// eventual pure-Go engine) and the CUE-backed oracle path (direct
// cuelang.org/go) — and reports whether the two agree on accept/reject,
// on the stage at which a rejection happened, and on the field path of
// the rejection.
//
// It is module-internal on purpose. The harness imports cuelang.org/go
// from non-test files, which is the dependency plan 218 phase 4 deletes;
// living under internal/ keeps that import invisible outside the module
// (no external user can take a dependency on a package slated for
// deletion) and lets every package in the module reuse the harness in
// its own differential tests.
//
// Phase 0 (plan 236) has no in-house engine yet, so the in-house path is
// itself the CUE-backed cuelite façade. The two paths therefore agree by
// construction and the harness runs green in CI as an ordinary go test.
// The per-surface phases that flip cuelite to the in-house engine reuse
// this harness to prove the flip preserves behaviour against the oracle.
// The Case/Outcome shape is stable but not frozen: surface D (ParsePath)
// adds a path-parse case and surface C (row-expression → string) adds an
// evaluation outcome, so later phases extend these types rather than
// treating them as final.
package cuelitetest

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	cuejson "cuelang.org/go/encoding/json"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// Stage names the point at which a path resolved a Case: where a
// rejection happened, or that the data was accepted. It keeps a parse
// failure (a schema or data the engine could not even compile) from
// masquerading as agreement with an oracle that merely rejected
// conforming-but-failing data — the two are different bugs.
type Stage int

const (
	// StageAccepted means the data satisfied the schema.
	StageAccepted Stage = iota
	// StageCompileSchema means the schema source failed to compile.
	StageCompileSchema
	// StageCompileData means the data document failed to compile.
	StageCompileData
	// StageValidate means schema and data compiled but the data did not
	// satisfy the schema.
	StageValidate
	// StageError means a path returned an error of an unexpected shape —
	// for the cuelite path, a validation error that was not a *PathError.
	// It exists so the harness can record the misbehaviour as a diff
	// instead of panicking.
	StageError
)

// String renders a Stage as its constant name, so a disagreement printed
// by Compare names the stage instead of an opaque integer.
func (s Stage) String() string {
	switch s {
	case StageAccepted:
		return "Accepted"
	case StageCompileSchema:
		return "CompileSchema"
	case StageCompileData:
		return "CompileData"
	case StageValidate:
		return "Validate"
	case StageError:
		return "Error"
	default:
		return "Stage(" + strconv.Itoa(int(s)) + ")"
	}
}

// Case is one differential-test input: a CUE schema source and a JSON
// data document to validate against it. Name labels the case in failure
// messages.
type Case struct {
	Name   string
	Schema string
	Data   string
}

// Outcome is the result of validating a Case through one path. Stage
// records where the case resolved. When it resolved at StageValidate,
// Paths carries the field path of every rejecting leaf — not only the
// first — sorted deterministically, so the two paths can be compared not
// just on accept/reject but on every leaf they reject and where. An
// engine that gets the first leaf right but drops a later one therefore
// shows a disagreement instead of passing.
type Outcome struct {
	Stage Stage
	Paths [][]string
}

// Accepted reports whether the data satisfied the schema.
func (o Outcome) Accepted() bool { return o.Stage == StageAccepted }

// Equal reports whether two Outcomes agree on the resolution stage and
// on every field path they carry. Paths are compared at EVERY stage, not
// only StageValidate: a later phase may attach a payload at another stage
// (surface D's parsed path segments at StageAccepted), and an Equal that
// ignored Paths off the validate stage would silently always-pass those
// — masking a real disagreement. A nil Paths equals an empty Paths.
//
// Equal compares sorted COPIES of the two path sets and never mutates
// its operands, so an Outcome built directly by a later phase — not
// through validatePaths, which sorts for deterministic display — still
// compares order-insensitively. The two engines may surface the same
// rejecting leaves in different orders; only the set of leaves matters.
func (o Outcome) Equal(other Outcome) bool {
	if o.Stage != other.Stage {
		return false
	}
	return slices.EqualFunc(sortedPaths(o.Paths), sortedPaths(other.Paths), slices.Equal[[]string])
}

// sortedPaths returns a sorted copy of paths, leaving the input
// untouched so Equal can normalize without mutating either operand.
func sortedPaths(paths [][]string) [][]string {
	out := slices.Clone(paths)
	slices.SortFunc(out, slices.Compare[[]string])
	return out
}

// validatePaths builds a StageValidate Outcome from a set of rejecting
// field paths, sorted deterministically and de-duplicated so the two engines
// compare on the SET of rejecting leaves regardless of order or multiplicity.
// CUE emits a disjunction conflict once per failed branch (so `"x" | "y"`
// rejected yields the same path several times), while the in-house engine
// reports each failing leaf once; both arms reject the same leaf, so the
// harness compares the leaf SET. The MDS020 diagnostic path already
// de-duplicates per field (dedupedCUEErrorDiags), so this matches the
// behavior consumers actually observe.
func validatePaths(paths [][]string) Outcome {
	slices.SortFunc(paths, slices.Compare[[]string])
	paths = slices.CompactFunc(paths, slices.Equal[[]string])
	return Outcome{Stage: StageValidate, Paths: paths}
}

// Path is a validation strategy: it validates a Case and reports the
// Outcome. The in-house path and the oracle path are both Paths, so the
// harness can call either uniformly.
type Path func(c Case) Outcome

// CueLitePath validates a Case through the cue/cuelite façade — the
// in-house path. In phase 0 the façade still delegates to CUE; later
// phases flip it to the pure-Go engine without changing this function.
func CueLitePath(c Case) Outcome {
	schema, err := cuelite.Compile(c.Schema)
	if err != nil {
		return Outcome{Stage: StageCompileSchema}
	}
	data, err := cuelite.CompileJSON([]byte(c.Data))
	if err != nil {
		return Outcome{Stage: StageCompileData}
	}
	return validateOutcome(schema.Unify(data).Validate())
}

// validateOutcome maps the result of cuelite.Validate to an Outcome.
// cuelite.Errors enumerates every per-field *PathError (bare or joined),
// so the Outcome carries all rejecting leaves, not only the first. A nil
// error is an acceptance; a non-nil error that carries no *PathError —
// which a future engine bug could produce — becomes StageError so the
// harness reports a diff rather than dropping the rejection.
func validateOutcome(verr error) Outcome {
	if verr == nil {
		return Outcome{Stage: StageAccepted}
	}
	leaves := cuelite.Errors(verr)
	if len(leaves) == 0 {
		return Outcome{Stage: StageError}
	}
	paths := make([][]string, len(leaves))
	for i, leaf := range leaves {
		paths[i] = leaf.Path()
	}
	return validatePaths(paths)
}

// OraclePath validates a Case directly through cuelang.org/go — the
// oracle the in-house path is measured against. It mirrors the cuelite
// path stage for stage: a per-call context (matching cuelite's per-Value
// context), strict JSON extraction for the data arm, and the field path
// of every CUE error leaf on a validation rejection — not only the
// first — so the oracle and the in-house path compare leaf for leaf.
func OraclePath(c Case) Outcome {
	ctx := cuecontext.New()
	schema := ctx.CompileString(c.Schema)
	if schema.Err() != nil {
		return Outcome{Stage: StageCompileSchema}
	}
	data, err := oracleData(ctx, []byte(c.Data))
	if err != nil {
		return Outcome{Stage: StageCompileData}
	}
	verr := schema.Unify(data).Validate(cue.Concrete(true))
	if verr == nil {
		return Outcome{Stage: StageAccepted}
	}
	return oracleValidate(errors.Errors(verr))
}

// oracleValidate maps a CUE validation error's per-leaf decomposition
// into a StageValidate Outcome, mirroring cuelite's joinValidationErrors
// fail-safe: an EMPTY decomposition falls back to a single nil path
// rather than zero paths. cuelite's bottom path always surfaces one
// nil-path *PathError, so without this fallback an empty leaf slice would
// yield a StageValidate Outcome with zero paths against cuelite's one — a
// phantom divergence in the fail-safe path even though both reject.
func oracleValidate(leaves []errors.Error) Outcome {
	if len(leaves) == 0 {
		return validatePaths([][]string{nil})
	}
	paths := make([][]string, len(leaves))
	for i, leaf := range leaves {
		paths[i] = normalizePath(leaf.Path())
	}
	return validatePaths(paths)
}

// normalizePath strips the quote-wrapping CUE applies to a path segment that
// is not a bare identifier (a numeric-looking key "0" comes back as `"0"`),
// so the oracle compares the RAW field key — the same unquoted key the
// in-house engine carries and that MDS020 indexes docFM with. Without this,
// the two arms diverge on the rendering of a quote-needing key even though
// they reject the same field.
func normalizePath(segs []string) []string {
	if segs == nil {
		return nil
	}
	out := make([]string, len(segs))
	for i, s := range segs {
		if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
			if unq, err := strconv.Unquote(s); err == nil {
				out[i] = unq
				continue
			}
		}
		out[i] = s
	}
	return out
}

// oracleData lifts the data document into ctx through CUE's own strict-JSON
// path — an independent duplicate-key scan, then cuejson.Extract plus a
// built-value bottom check — so the oracle rejects non-JSON data and
// duplicate keys exactly where the in-house engine does. It returns the lift
// error so OraclePath branches on it rather than on a sentinel bottom value,
// and takes []byte so the benchmark hoists the conversion out of its timed
// loop, keeping both arms symmetric.
//
// The duplicate-key rejection is an INDEPENDENT reimplementation of
// cuelite's rule (rawDuplicateKeys), not a call into it: the harness's value
// is that two separate implementations of the same contract agree. CUE's
// JSON lift would silently unify same-named object keys into a phantom
// merged object, so without this check the oracle would accept a document
// the in-house arm rejects at StageCompileData — a phantom divergence.
//
// The post-flip in-house lifter accepts a lone-surrogate escape ("\ud800")
// as a U+FFFD string, where this CUE lift rejects it; that deliberate
// divergence (plan 238) is kept out of the differential corpus and pinned by
// the cuelite package's own unit tests instead, so this oracle stays a
// faithful direct-CUE check on every corpus row.
func oracleData(ctx *cue.Context, data []byte) (cue.Value, error) {
	if err := rawDuplicateKeys(data); err != nil {
		return cue.Value{}, err
	}
	expr, err := extractJSONSafely(data)
	if err != nil {
		return cue.Value{}, err
	}
	val := ctx.BuildExpr(expr)
	if err := val.Err(); err != nil {
		return cue.Value{}, err
	}
	return val, nil
}

// extractJSONSafely wraps cuejson.Extract with a panic recovery: cuelang's
// JSON-via-expression parser panics (rather than erroring) on some malformed
// inputs (e.g. "0..."), which would crash the differential run instead of
// recording a data-compile rejection. Recovering converts the panic to the
// data-compile error the in-house arm also produces for malformed data, so
// the two arms agree on rejection rather than the oracle aborting.
func extractJSONSafely(data []byte) (expr ast.Expr, err error) {
	defer func() {
		if r := recover(); r != nil {
			expr = nil
			err = fmt.Errorf("cuejson.Extract panicked: %v", r)
		}
	}()
	return cuejson.Extract("", data)
}

// rawDuplicateKeys rejects the first object key repeated within one
// object at any depth, mirroring cuelite's strict-JSON rule with an
// INDEPENDENT walk: a recursive token consumer driven by sentinel
// signals rather than cuelite's flat parity stack, so the two arms
// agreeing is real cross-checking and not one calling the other. A
// malformed document yields no duplicate error — any token error is
// reported as errMalformed and swallowed at the top, leaving
// cuejson.Extract to report the syntax error and keep one definition of
// "not JSON". It also independently mirrors cuelite's lossy-decode
// guards: invalid-UTF-8 input defers to Extract (a utf8.Valid pre-check)
// and a decoded key containing U+FFFD is skipped for dup tracking, so
// neither arm fabricates a duplicate from a key the decoder folded.
func rawDuplicateKeys(data []byte) error {
	// Invalid UTF-8 would make the decoder fold distinct raw keys onto one
	// U+FFFD; leave such input to cuejson.Extract, matching cuelite.
	if !utf8.Valid(data) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	// UseNumber keeps a number outside float64 range (1e999, valid JSON)
	// from erroring mid-walk and being misread as malformed, which would
	// let a duplicate beside it slip past — matching cuelite's scanner so
	// the two arms do not diverge.
	dec.UseNumber()
	if err := walkJSONValue(dec); err != nil && !stderrors.Is(err, errMalformed) {
		return err
	}
	return nil
}

// errMalformed signals that the token stream was not well-formed JSON.
// rawDuplicateKeys swallows it (deferring to cuejson.Extract); a genuine
// duplicate-key error is a different, non-errMalformed error that
// propagates.
var errMalformed = stderrors.New("malformed JSON")

// walkJSONValue consumes exactly one JSON value from dec — a scalar, or a
// whole object/array it recurses into — and returns the first duplicate
// object key it finds. A token error becomes errMalformed.
func walkJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return errMalformed
	}
	// json.Decoder only ever hands an OPENING delimiter as a value token:
	// a bare '}' or ']' is a syntax error it reports at Token() above, never
	// a value here. So a delim is '{' or '[', and anything else is a scalar.
	switch tok {
	case json.Delim('{'):
		return walkJSONObject(dec)
	case json.Delim('['):
		return walkJSONArray(dec)
	default:
		return nil // a scalar value
	}
}

// walkJSONObject consumes object members up to the matching '}', rejecting
// the first key that repeats within this object and recursing into each
// value. The opening '{' has already been consumed by the caller.
func walkJSONObject(dec *json.Decoder) error {
	seen := map[string]struct{}{}
	for dec.More() {
		// dec.More() is true, so the next token is a string key: a JSON
		// object key is always a string, and a non-string would be a syntax
		// error Token() reports here, not a wrong-typed token. So a token
		// error is the only failure to guard, and the assertion is safe.
		keyTok, err := dec.Token()
		if err != nil {
			return errMalformed
		}
		key := keyTok.(string)
		// A key whose decode produced U+FFFD (a lone-surrogate escape or a
		// literal "�") cannot be reliably distinguished from another such
		// key, so skip dup tracking for it — matching cuelite's scanner.
		if strings.ContainsRune(key, utf8.RuneError) {
			if err := walkJSONValue(dec); err != nil {
				return err
			}
			continue
		}
		if _, dup := seen[key]; dup {
			return fmt.Errorf("duplicate JSON key %q", key)
		}
		seen[key] = struct{}{}
		if err := walkJSONValue(dec); err != nil {
			return err
		}
	}
	// Consume the closing '}'.
	if _, err := dec.Token(); err != nil {
		return errMalformed
	}
	return nil
}

// walkJSONArray consumes array elements up to the matching ']', recursing
// into each. The opening '[' has already been consumed by the caller.
func walkJSONArray(dec *json.Decoder) error {
	for dec.More() {
		if err := walkJSONValue(dec); err != nil {
			return err
		}
	}
	// Consume the closing ']'.
	if _, err := dec.Token(); err != nil {
		return errMalformed
	}
	return nil
}

// Compare runs one Case through both inHouse and oracle and reports a
// failure on t when the two Outcomes disagree. It returns true when they
// agree.
func Compare(t testing.TB, inHouse, oracle Path, c Case) bool {
	t.Helper()
	got := inHouse(c)
	want := oracle(c)
	if got.Equal(want) {
		return true
	}
	t.Errorf("case %q: in-house path %+v disagrees with oracle %+v", c.Name, got, want)
	return false
}

// Run compares every Case in cases through the in-house and oracle
// paths, reporting each disagreement on t. It is the entry point a
// phase's differential test calls over its corpus.
func Run(t testing.TB, cases []Case) {
	t.Helper()
	for _, c := range cases {
		Compare(t, CueLitePath, OraclePath, c)
	}
}
