package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"github.com/jeduden/mdsmith/internal/fieldinterp"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/yuin/goldmark/ast"
)

// DocHeading is a heading collected from the document under
// validation.
type DocHeading struct {
	Level int
	Text  string
	Line  int
}

// ExtractDocHeadings walks the document AST and collects every
// heading in source order, with its source line.
func ExtractDocHeadings(f *lint.File) []DocHeading {
	var out []DocHeading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		text := headingText(h, f.Source)
		line := headingLine(h, f)
		out = append(out, DocHeading{Level: h.Level, Text: text, Line: line})
		return ast.WalkContinue, nil
	})
	return out
}

// headingLine returns the 1-based line number of h. Goldmark
// occasionally produces ATX headings with an empty Lines() slice;
// when that happens we walk inline descendants for the first Text
// segment, matching the fallback in internal/rules/astutil. A
// truly empty heading (no Lines, no Text descendants) reports line
// 1 so callers that filter by line windows never lose the
// heading.
func headingLine(h *ast.Heading, f *lint.File) int {
	if h.Lines().Len() > 0 {
		return f.LineOfOffset(h.Lines().At(0).Start)
	}
	line := 1
	_ = ast.Walk(h, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n == h {
			return ast.WalkContinue, nil
		}
		t, ok := n.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		line = f.LineOfOffset(t.Segment.Start)
		return ast.WalkStop, nil
	})
	return line
}

// MakeDiag is the diagnostic constructor the validator uses. Callers
// supply it so the schema package stays free of rule-ID coupling.
type MakeDiag func(file string, line int, msg string) lint.Diagnostic

// Validate walks the document AST against sch, emitting diagnostics
// for missing/extra/out-of-order sections, level mismatches,
// frontmatter that fails the schema's CUE constraints, and filename
// patterns. mkDiag builds the diagnostic with the caller's rule ID.
//
// docFM is the document's parsed front matter (nil when absent).
// When fmIsCUE is true, the front-matter values are themselves CUE
// expressions (the `cue-frontmatter` placeholder); the CUE check is
// skipped because the values are not concrete data.
func Validate(
	f *lint.File, sch *Schema, docFM map[string]any, fmIsCUE bool,
	mkDiag MakeDiag,
) []lint.Diagnostic {
	if sch == nil || sch.IsEmpty() {
		return nil
	}
	var diags []lint.Diagnostic

	diags = append(diags, validateFilename(f, sch, mkDiag)...)

	if !fmIsCUE {
		diags = append(diags, validateFrontmatterDiags(f, sch, docFM, mkDiag)...)
	}

	rootLevel := sch.EffectiveRootLevel()
	heads := ExtractDocHeadings(f)
	body := skipBelow(heads, rootLevel)

	_, sd := validateScopes(f, sch, sch.Sections, sch.Closed, body, 0, rootLevel, mkDiag)
	diags = append(diags, sd...)

	diags = append(diags, ValidateContent(f, sch, mkDiag)...)

	return diags
}

// validateFrontmatterDiags compiles the schema's CUE expression,
// unifies it with the document front matter, and emits one
// diagnostic per resulting CUE error (rather than collapsing all
// of them into a single line). Each diagnostic is a
// SchemaDiagnostic rendered through Format(): the field that
// failed, the value the user wrote, the constraint they
// violated, and — when applicable — a hint.
//
// A schema-side compile or marshal failure surfaces as a single
// fallback diagnostic at line 1 so users still see a signal
// without needing to chase the underlying CUE error. The
// formerly-flat "front matter does not satisfy schema CUE
// constraints" message is intentionally retired; see plan 147.
func validateFrontmatterDiags(
	f *lint.File, sch *Schema, docFM map[string]any, mkDiag MakeDiag,
) []lint.Diagnostic {
	expr := sch.FrontmatterCUE()
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	ctx := cuecontext.New()
	anchor := nonBodyDiagLine(f)
	schemaVal := ctx.CompileString(expr)
	if err := schemaVal.Err(); err != nil {
		return []lint.Diagnostic{mkDiag(f.Path, anchor,
			compileFailureDiag(sch, "schema", "valid schema CUE", err).Format())}
	}
	if docFM == nil {
		docFM = map[string]any{}
	}
	data, err := json.Marshal(docFM)
	if err != nil {
		return []lint.Diagnostic{mkDiag(f.Path, anchor,
			compileFailureDiag(sch, "front matter", "JSON-marshalable front matter", err).Format())}
	}
	dataVal := ctx.CompileBytes(data)
	if err := dataVal.Err(); err != nil {
		return []lint.Diagnostic{mkDiag(f.Path, anchor,
			compileFailureDiag(sch, "front matter", "valid front matter", err).Format())}
	}
	merged := schemaVal.Unify(dataVal)
	verr := merged.Validate(cue.Concrete(true))
	if verr == nil {
		return nil
	}
	cueErrs := errors.Errors(verr)
	if len(cueErrs) == 0 {
		return []lint.Diagnostic{mkDiag(f.Path, anchor,
			SchemaDiagnostic{
				Field:     "front matter",
				Actual:    fmt.Sprintf("%v", verr),
				Expected:  "valid CUE",
				SchemaRef: schemaRef(sch, ""),
			}.Format())}
	}
	keyLines := docFrontmatterKeyLines(f)
	out := make([]lint.Diagnostic, 0, len(cueErrs))
	// A struct dedup key avoids accidental collisions when one of
	// the components (notably the raw-CUE-expression Expected
	// fallback and a placeholder-bearing Field) legitimately
	// contains the same delimiter we would have used in a flat
	// string key.
	type dedupKey struct{ field, actual, expected string }
	seen := make(map[dedupKey]bool, len(cueErrs))
	for _, ce := range cueErrs {
		d := schemaDiagFromCUEError(sch, docFM, ce)
		key := dedupKey{field: d.Field, actual: d.Actual, expected: d.Expected}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, mkDiag(f.Path, fmDiagLine(f, ce.Path(), keyLines), d.Format()))
	}
	return out
}

// NonBodyDiagLine returns the body-coord line value that, after
// lint.File.AdjustDiagnostics adds f.LineOffset, lands on the
// absolute first line of the file (typically the opening `---`
// fence of stripped front matter). It is the canonical anchor
// for diagnostics that do not correspond to a specific body
// line — schema-level compile failures, filename pattern
// violations, and structure diagnostics for sections that are
// missing entirely.
//
// The previous "anchor at line 1" pattern landed on the first
// body line in front-matter-stripped mode, which
// engine.filterGeneratedDiags could mistakenly drop if the
// document body started with a generated section (e.g. a
// leading <?catalog?> directive). Using `1 - LineOffset`
// produces a non-positive body-coord that filterGeneratedDiags
// cannot match against any generated line range, and the
// engine's AdjustDiagnostics adds the offset back so the
// surfaced diagnostic still anchors at the file's first line.
//
// When f.LineOffset == 0 (the non-stripped path, used by tests
// via lint.NewFile) the return value is just 1, matching the
// previous behaviour.
func NonBodyDiagLine(f *lint.File) int {
	return 1 - f.LineOffset
}

// nonBodyDiagLine is the package-internal alias used inside the
// schema package; external callers reach for NonBodyDiagLine.
func nonBodyDiagLine(f *lint.File) int {
	return NonBodyDiagLine(f)
}

// fmDiagLine returns the line to anchor a front-matter diagnostic
// at, expressed in the body-line coordinate system the engine
// uses before lint.File.AdjustDiagnostics fires. When the doc's
// FM has a tracked source line for the offending key, the line
// is the file-relative line of that key minus f.LineOffset; the
// engine's AdjustDiagnostics shift then lands the diagnostic on
// the absolute file line of the key.
//
// In front-matter-stripped mode (f.LineOffset > 0) the body-
// coordinate value is non-positive for keys inside the FM block.
// That is intentional: AdjustDiagnostics adds LineOffset back to
// reach the absolute line, which is positive. Callers that
// consume Diagnostic.Line without running it through the engine
// must normalise the value first (see ValidateFrontmatterDiags
// for the contract). In unstripped mode (LineOffset == 0) the
// returned value already equals the absolute file line.
//
// When no per-key line is known the function falls back to
// nonBodyDiagLine(f) — a non-positive body coordinate in
// stripped mode that AdjustDiagnostics resolves to the first
// absolute line of the file. The fallback used to be a flat
// "1", which landed on the first body line in stripped mode
// and could be silently dropped by filterGeneratedDiags when
// the document body started with a generated section
// (PR #284 Copilot review).
func fmDiagLine(f *lint.File, path []string, keyLines map[string]int) int {
	if len(path) == 0 || len(keyLines) == 0 {
		return nonBodyDiagLine(f)
	}
	line, ok := keyLines[path[0]]
	if !ok {
		// Top-level path may carry an optional-key suffix in the
		// schema; the doc itself never does, so a miss here means
		// the key was absent from the document (and therefore has
		// no source line to point at).
		return nonBodyDiagLine(f)
	}
	return line - f.LineOffset
}

// docFrontmatterKeyLines parses the document's front matter and
// returns the file-relative line of each top-level key. Works in
// both front-matter-stripped mode (FM lives in f.FrontMatter) and
// unstripped mode (FM still at the top of f.Source) so the same
// helper serves the LSP / CLI path and the rule's unit-test
// callers that build files via lint.NewFile directly.
func docFrontmatterKeyLines(f *lint.File) map[string]int {
	if fm := f.FrontMatter; len(fm) > 0 {
		return parseFMBlockKeyLines(fm)
	}
	if bytes.HasPrefix(f.Source, []byte("---\n")) {
		if fm, _ := lint.StripFrontMatter(f.Source); fm != nil {
			return parseFMBlockKeyLines(fm)
		}
	}
	return nil
}

// parseFMBlockKeyLines extracts top-level key lines from a YAML
// front-matter block that includes its `---` delimiters. yaml.Node
// line numbers are 1-based within the body between the fences; the
// file-relative line is one more (the opening `---` occupies the
// first source line).
//
// The closing fence is removed via TrimSuffix rather than a
// bytes.Index scan: callers feed us a block returned by
// lint.StripFrontMatter which always ends with `---\n`, and a
// scan-for-first-match would truncate the body early if a YAML
// block scalar legitimately contained the `---\n` sequence as a
// value.
func parseFMBlockKeyLines(fm []byte) map[string]int {
	body := bytes.TrimPrefix(fm, []byte("---\n"))
	body = bytes.TrimSuffix(body, []byte("---\n"))
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	node, err := yamlutil.UnmarshalNodeSafe(body)
	if err != nil {
		return nil
	}
	return yamlutil.TopLevelMappingLines(&node, 1)
}

// schemaDiagFromCUEError converts one CUE-error leaf into a
// SchemaDiagnostic. The CUE error's Path() names the offending
// field; we look up the raw constraint expression on the schema
// to render an "expected" string in user vocabulary, and pull
// the actual value out of docFM so the message shows exactly
// what the user wrote.
//
// Precision note: lookupConstraint and schemaRef both resolve
// against path[0] (the top-level frontmatter key the schema's
// Frontmatter map indexes by). For nested CUE errors — e.g.
// `meta.owner` against a schema whose `meta:` value is itself
// a struct constraint — Field still shows the full dotted
// path so the reader can locate the failing leaf, but Expected
// renders the parent constraint (the top-level CUE expression)
// rather than the leaf. Today every shipped schema in
// mdsmith uses single-segment frontmatter constraints, so
// this asymmetry is hypothetical; option (a) from the PR #284
// review (CUE LookupPath on ce.Path() for leaf-level
// resolution) is the natural follow-up if nested frontmatter
// constraints land later.
func schemaDiagFromCUEError(
	sch *Schema, docFM map[string]any, ce errors.Error,
) SchemaDiagnostic {
	path := ce.Path()
	field := "front matter"
	if len(path) > 0 {
		field = strings.Join(path, ".")
	}
	d := SchemaDiagnostic{
		Field:     field,
		SchemaRef: schemaRef(sch, schemaKeyForPath(sch, path)),
	}
	actualVal, hasActual := lookupFM(docFM, path)
	if hasActual {
		d.Actual = formatActual(actualVal)
	}
	if expr := lookupConstraint(sch, path); expr != "" {
		d.Expected = RenderExpected(expr)
		if hasActual {
			d.Hint = RenderHint(expr, actualVal)
		} else {
			// A required field that the document omitted. Show
			// the same <missing> sentinel structure diagnostics
			// use so every diagnostic answers the same three
			// questions: which field, what value, what's
			// expected.
			d.Actual = "<missing>"
		}
	} else {
		// Extra field: close() rejected a key that is not in the
		// schema's frontmatter map. There is no per-field
		// constraint to render, and the schema source already
		// names the declared set; the diagnostic body says so
		// explicitly so the reader can compare against the
		// schema file. <extra field> is the actual-slot sentinel
		// for the (rare) case where the key has no value to show
		// (e.g. an empty mapping entry).
		if !hasActual {
			d.Actual = "<extra field>"
		}
		d.Expected = "not declared in schema"
	}
	return d
}

// schemaKeyForPath finds the Frontmatter map key (with the
// optional "?" suffix preserved) that owns the given CUE error
// path. The required-key form "x" and optional-key form "x?"
// produce identical CUE paths, so we accept either when looking
// up the constraint and the source line.
func schemaKeyForPath(sch *Schema, path []string) string {
	if len(path) == 0 {
		return ""
	}
	first := path[0]
	if _, ok := sch.Frontmatter[first]; ok {
		return first
	}
	if _, ok := sch.Frontmatter[first+"?"]; ok {
		return first + "?"
	}
	return ""
}

func lookupConstraint(sch *Schema, path []string) string {
	if key := schemaKeyForPath(sch, path); key != "" {
		return sch.Frontmatter[key]
	}
	return ""
}

// lookupFM walks docFM along path and returns the leaf value.
// The boolean reports whether the path resolved; a present-but-
// nil value still reports true so the diagnostic can show
// "null" rather than "<missing>".
//
// path segments come from CUE's error.Path() and may be mixed
// map keys and numeric list indices (e.g. "tags", "1" for a
// failing element of a list-shaped field). lookupFM understands
// both: it descends through `map[string]any` by key and through
// `[]any` by parsing the segment as a non-negative integer.
// Falling back to <missing> on a list-shaped path would
// misreport "field present but a particular index failed" as
// "field absent" — see the Copilot review comment on PR #284.
func lookupFM(docFM map[string]any, path []string) (any, bool) {
	if len(path) == 0 {
		return nil, false
	}
	cur := any(docFM)
	for _, p := range path {
		switch typed := cur.(type) {
		case map[string]any:
			v, ok := typed[p]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(typed) {
				return nil, false
			}
			cur = typed[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// schemaRef builds the `schema: ...` suffix from the schema's
// Source label and (when known) the line of the named key. An
// unknown source falls back to a generic "schema" label so
// every diagnostic still carries a reference field.
func schemaRef(sch *Schema, key string) string {
	src := sch.Source
	if src == "" {
		src = "schema"
	}
	if key != "" {
		if line, ok := sch.FrontmatterLines[key]; ok && line > 0 {
			return fmt.Sprintf("%s:%d", src, line)
		}
	}
	return src
}

// compileFailureDiag builds a SchemaDiagnostic for the
// early-return CUE / JSON failure paths in
// validateFrontmatterDiags. The `Field` names what failed to
// process ("schema", "front matter"), `expected` carries the
// shape-specific contract (e.g. "valid schema CUE",
// "JSON-marshalable front matter"), the `Actual` carries the
// underlying error message, and the schema reference points the
// reader at the source so the diagnostic stays consistent with
// the rest of MDS020's output. Threading `expected` through the
// helper keeps each early-return path's message accurate —
// using a single generic phrase would have made the
// json.Marshal failure path read as "Expected: compilable CUE"
// even though CUE isn't involved at that step.
func compileFailureDiag(sch *Schema, field, expected string, err error) SchemaDiagnostic {
	return SchemaDiagnostic{
		Field:     field,
		Actual:    fmt.Sprintf("%v", err),
		Expected:  expected,
		SchemaRef: schemaRef(sch, ""),
	}
}

// ValidateFrontmatterDiags exposes the per-error CUE-diagnostic
// walker to callers outside the validator (notably the
// requiredstructure rule's legacy file-schema path, which has
// its own heading-template parser but reuses the schema
// package's actionable front-matter diagnostics).
//
// Line numbers on returned diagnostics are in the engine's
// body-coordinate system: they are the absolute file line of
// the offending front-matter key minus f.LineOffset. In
// front-matter-stripped mode (f.LineOffset > 0) the body-
// coordinate value is non-positive for keys inside the FM
// block; lint.File.AdjustDiagnostics then shifts it back into
// the absolute file line. Callers that bypass the engine — for
// instance unit tests inspecting the raw slice — must either
// run the result through f.AdjustDiagnostics or normalise the
// values themselves before treating them as 1-based positions.
func ValidateFrontmatterDiags(
	f *lint.File, sch *Schema, docFM map[string]any, mkDiag MakeDiag,
) []lint.Diagnostic {
	return validateFrontmatterDiags(f, sch, docFM, mkDiag)
}

// FormatSchemaRef builds the "source:line" suffix used by every
// SchemaDiagnostic so callers outside the schema package emit
// the same shape. An unknown source falls back to "schema".
func FormatSchemaRef(sch *Schema, key string) string {
	return schemaRef(sch, key)
}

// skipBelow returns a filtered slice that omits every heading
// whose level is shallower than rootLevel. The previous
// truncate-at-first-deep-heading variant only stripped a leading
// title, but an out-of-place shallower heading in the middle of the
// document would later terminate matchScope at the root and leave
// subsequent required scopes unmatched. Filtering throughout
// removes those terminators so the root walk continues across
// stray H1-level headings.
func skipBelow(heads []DocHeading, rootLevel int) []DocHeading {
	out := make([]DocHeading, 0, len(heads))
	for _, h := range heads {
		if h.Level >= rootLevel {
			out = append(out, h)
		}
	}
	return out
}

// validateScopes walks scopes (the listed children of a single level)
// against docHeads starting at docIdx. expectedLevel is the heading
// level these scopes should appear at. Returns the new docIdx
// (position after consuming this scope-list) and emitted diagnostics.
//
// closed controls handling of unlisted headings at this level: when
// true, an unlisted heading flags a diagnostic; when false, it is
// tolerated. A wildcard scope ("...") always tolerates unlisted
// headings at its position.
func validateScopes(
	f *lint.File, sch *Schema, scopes []Scope, closed bool, docHeads []DocHeading,
	docIdx int, expectedLevel int, mkDiag MakeDiag,
) (int, []lint.Diagnostic) {
	var diags []lint.Diagnostic
	requiredByText := buildRequiredByText(scopes)
	claimed := make(map[int]bool)
	allowExtra := false

	for i, sc := range scopes {
		if sc.Preamble {
			// The preamble has no heading to match. Its rules: are
			// applied by the per-scope walker in MDS020 against the
			// [parent-start, first-child-heading) line range. The
			// validator itself only needs to mark the entry as
			// processed; plan 149 adds content-shape checks.
			claimed[i] = true
			continue
		}
		if sc.Wildcard {
			allowExtra = true
			continue
		}
		if claimed[i] {
			continue
		}
		newIdx, scDiags, found := matchScope(
			f, sch, scopes, i, expectedLevel, docHeads, docIdx,
			requiredByText, claimed, allowExtra, closed, mkDiag)
		diags = append(diags, scDiags...)
		docIdx = newIdx
		if found {
			allowExtra = false
		} else if !claimed[i] && sc.Required && !sc.Repeats {
			// Missing sections have no body line to point at;
			// use the non-body anchor so filterGeneratedDiags
			// can't drop the diagnostic if body line 1 sits
			// inside a generated section.
			diags = append(diags, mkDiag(f.Path, nonBodyDiagLine(f),
				missingSectionDiag(formatHeading(expectedLevel, sc.Heading), sch).Format()))
		}
	}

	newIdx, leftoverDiags := handleLeftoverHeadings(
		f, sch, scopes, claimed, docHeads, docIdx, expectedLevel,
		closed, allowExtra, mkDiag)
	diags = append(diags, leftoverDiags...)
	return newIdx, diags
}

// missingSectionDiag builds a SchemaDiagnostic for a required
// section that is absent from the document. The field carries
// the heading marker form (`## Goal`) so the reader can grep for
// the exact text; the actual is "<missing>" and the schema
// reference points back at the schema source.
func missingSectionDiag(heading string, sch *Schema) SchemaDiagnostic {
	return SchemaDiagnostic{
		Field:     heading,
		Actual:    "<missing>",
		Expected:  "section to be present",
		SchemaRef: schemaRef(sch, ""),
	}
}

// unexpectedSectionDiag builds a SchemaDiagnostic for a heading
// that appears in the document but is not declared in the
// schema. expected, when non-empty, names the section the
// validator was looking for at that position; the message
// surfaces it as a hint so the reader can decide whether they
// added the wrong heading or omitted a different one.
func unexpectedSectionDiag(heading, expected string, sch *Schema) SchemaDiagnostic {
	d := SchemaDiagnostic{
		Field:     heading,
		Actual:    "<present>",
		Expected:  "not declared in schema",
		SchemaRef: schemaRef(sch, ""),
	}
	if expected != "" {
		d.Hint = fmt.Sprintf("expected %q here instead", expected)
	}
	return d
}

// outOfOrderDiag builds a SchemaDiagnostic for a heading whose
// text matches a declared section but appears at the wrong
// position relative to its siblings. expectedAfter, when
// non-empty, names the section it should follow.
func outOfOrderDiag(heading, expectedAfter string, sch *Schema) SchemaDiagnostic {
	d := SchemaDiagnostic{
		Field:     heading,
		Actual:    "<out of order>",
		Expected:  "in declared order",
		SchemaRef: schemaRef(sch, ""),
	}
	if expectedAfter != "" {
		d.Hint = fmt.Sprintf("expected after %q", expectedAfter)
	} else {
		d.Hint = "expected before this position"
	}
	return d
}

// levelMismatchSchemaDiag builds a SchemaDiagnostic for a
// heading whose text matches a declared section but appears at
// the wrong heading level (e.g. `## Step` where the schema
// declared `### Step`).
func levelMismatchSchemaDiag(text string, expectedLevel, actualLevel int, sch *Schema) SchemaDiagnostic {
	return SchemaDiagnostic{
		Field:     text,
		Actual:    fmt.Sprintf("h%d", actualLevel),
		Expected:  fmt.Sprintf("h%d", expectedLevel),
		SchemaRef: schemaRef(sch, ""),
	}
}

// handleLeftoverHeadings processes doc headings that survived the
// scope iteration. A leftover that matches an unclaimed listed
// scope is flagged as out-of-order regardless of open/closed — the
// user listed the section, so its position is still a constraint —
// and its child sections are validated recursively so nested
// required sections still surface. Other leftovers depend on
// closed: flagged as unexpected in closed scopes, silently
// consumed in open ones.
func handleLeftoverHeadings(
	f *lint.File, sch *Schema, scopes []Scope, claimed map[int]bool,
	docHeads []DocHeading, docIdx, expectedLevel int,
	closed, allowExtra bool, mkDiag MakeDiag,
) (int, []lint.Diagnostic) {
	var diags []lint.Diagnostic
	for docIdx < len(docHeads) {
		dh := docHeads[docIdx]
		if dh.Level < expectedLevel {
			break
		}
		if dh.Level != expectedLevel {
			docIdx++
			continue
		}
		if idx := unclaimedListedScope(scopes, dh, claimed); idx >= 0 {
			newIdx, claimDiags := claimLateScope(
				f, sch, scopes, idx, expectedLevel, docHeads, docIdx, claimed, mkDiag)
			diags = append(diags, claimDiags...)
			docIdx = newIdx
			continue
		}
		if !allowExtra && closed {
			diags = append(diags, mkDiag(f.Path, dh.Line,
				unexpectedSectionDiag(formatHeading(dh.Level, dh.Text), "", sch).Format()))
		}
		docIdx++
	}
	return docIdx, diags
}

// claimLateScope marks a late-arriving listed scope as claimed,
// emits its out-of-order diagnostic, and recurses into the scope's
// nested children so missing-required-section diagnostics still
// surface beneath a late parent.
func claimLateScope(
	f *lint.File, sch *Schema, scopes []Scope, idx, expectedLevel int,
	docHeads []DocHeading, docIdx int, claimed map[int]bool,
	mkDiag MakeDiag,
) (int, []lint.Diagnostic) {
	dh := docHeads[docIdx]
	diags := []lint.Diagnostic{mkDiag(f.Path, dh.Line,
		outOfOrderDiag(formatHeading(dh.Level, dh.Text), "", sch).Format())}
	claimed[idx] = true
	docIdx++
	if len(scopes[idx].Sections) > 0 {
		newIdx, childDiags := validateScopes(
			f, sch, scopes[idx].Sections, scopes[idx].Closed,
			docHeads, docIdx, expectedLevel+1, mkDiag)
		diags = append(diags, childDiags...)
		docIdx = newIdx
	}
	return docIdx, diags
}

// unclaimedListedScope returns the index of the first unclaimed
// non-wildcard scope whose text matches dh, or -1 when no listed
// scope is a candidate.
func unclaimedListedScope(
	scopes []Scope, dh DocHeading, claimed map[int]bool,
) int {
	for i, sc := range scopes {
		if claimed[i] || sc.Wildcard {
			continue
		}
		if scopeMatchesHeading(sc, dh) {
			return i
		}
	}
	return -1
}

func buildRequiredByText(scopes []Scope) map[string][]int {
	out := map[string][]int{}
	for i, sc := range scopes {
		if sc.Wildcard || sc.Preamble {
			// Preambles have no heading text; wildcards by design.
			continue
		}
		// Skip the "?" wildcard and placeholder patterns — neither
		// can sit in a literal-text map; the findOutOfOrderIdx
		// fallback handles them via scopeMatchesHeading.
		if !indexableLiteral(sc.Heading) {
			// no-op
		} else {
			out[sc.Heading] = append(out[sc.Heading], i)
		}
		for _, a := range sc.Aliases {
			if !indexableLiteral(a) {
				continue
			}
			out[a] = append(out[a], i)
		}
	}
	return out
}

// indexableLiteral reports whether text is a fully-literal heading
// that can be used as a map key. "?" and patterns containing
// placeholders match many doc texts and cannot be pre-indexed; the
// fallback scan handles those.
func indexableLiteral(text string) bool {
	if text == "?" {
		return false
	}
	return !fieldinterp.ContainsField(text)
}

// matchScope advances docIdx looking for a heading matching the
// scope at scopes[idx]. Intervening doc headings either belong to a
// later listed scope (out-of-order), are unexpected (closed + no
// wildcard), or are descended into as part of an earlier scope's
// subtree. Returns the new docIdx, diagnostics, and whether the
// scope was matched.
func matchScope(
	f *lint.File, sch *Schema, scopes []Scope, idx, expectedLevel int,
	docHeads []DocHeading, docIdx int,
	requiredByText map[string][]int, claimed map[int]bool,
	allowExtra, closed bool, mkDiag MakeDiag,
) (int, []lint.Diagnostic, bool) {
	sc := scopes[idx]
	var diags []lint.Diagnostic

	for docIdx < len(docHeads) {
		dh := docHeads[docIdx]
		// Shallower than us belongs to an ancestor — unless the text
		// still matches, in which case we claim it here with a
		// level-mismatch diagnostic. Without this branch a wrong-level
		// match would surface as both "missing required" (here) and
		// "unexpected" (when the caller revisits it).
		if dh.Level < expectedLevel {
			if scopeMatchesHeading(sc, dh) {
				return claimMatch(f, sch, sc, idx, expectedLevel, docHeads, docIdx, claimed, mkDiag, diags)
			}
			return docIdx, diags, false
		}
		if scopeMatchesHeading(sc, dh) {
			return claimMatch(f, sch, sc, idx, expectedLevel, docHeads, docIdx, claimed, mkDiag, diags)
		}
		if ooIdx := findOutOfOrderIdx(scopes, dh, requiredByText, claimed, idx+1); ooIdx >= 0 {
			if !sc.Required {
				// The current scope is optional — its absence is not
				// a violation, so dh matching a later listed scope is
				// not "out of order". Return without claiming so the
				// outer loop advances to the matching scope, which
				// will pair dh on its own iteration.
				return docIdx, diags, false
			}
			newIdx, ooDiags := claimOutOfOrder(
				f, sch, scopes, idx, ooIdx, expectedLevel, docHeads, docIdx, claimed, mkDiag)
			diags = append(diags, ooDiags...)
			docIdx = newIdx
			continue
		}
		// dh did not match this scope or any later listed scope by
		// text. Deeper than expected: orphan child of some unmatched
		// parent — consume silently. Same level: treat as unexpected
		// when closed and no wildcard has opened the door.
		if dh.Level > expectedLevel {
			docIdx++
			continue
		}
		if !allowExtra && closed {
			diags = append(diags, mkDiag(f.Path, dh.Line,
				unexpectedSectionDiag(
					formatHeading(dh.Level, dh.Text),
					formatHeading(expectedLevel, sc.Heading),
					sch).Format()))
		}
		docIdx++
	}
	return docIdx, diags, false
}

func levelDiagIfNeeded(
	f *lint.File, sch *Schema, dh DocHeading, expectedLevel int, mkDiag MakeDiag,
) []lint.Diagnostic {
	if dh.Level == expectedLevel {
		return nil
	}
	return []lint.Diagnostic{mkDiag(f.Path, dh.Line,
		levelMismatchSchemaDiag(dh.Text, expectedLevel, dh.Level, sch).Format())}
}

// claimMatch marks scopes[idx] as matched against docHeads[docIdx],
// appending the level-mismatch diagnostic when applicable and
// recursing into the scope's child sections. Returns the advanced
// docIdx, combined diagnostics, and true.
func claimMatch(
	f *lint.File, sch *Schema, sc Scope, idx, expectedLevel int,
	docHeads []DocHeading, docIdx int, claimed map[int]bool,
	mkDiag MakeDiag, prior []lint.Diagnostic,
) (int, []lint.Diagnostic, bool) {
	diags := append(prior, levelDiagIfNeeded(f, sch, docHeads[docIdx], expectedLevel, mkDiag)...)
	claimed[idx] = true
	docIdx++
	if len(sc.Sections) > 0 {
		newIdx, childDiags := validateScopes(
			f, sch, sc.Sections, sc.Closed, docHeads, docIdx,
			expectedLevel+1, mkDiag)
		diags = append(diags, childDiags...)
		docIdx = newIdx
	}
	return docIdx, diags, true
}

// claimOutOfOrder records that docHeads[docIdx] matches scopes[ooIdx]
// (a later listed scope), emits the out-of-order diagnostic, and
// recurses into the matched scope's child sections.
func claimOutOfOrder(
	f *lint.File, sch *Schema, scopes []Scope, idx, ooIdx, expectedLevel int,
	docHeads []DocHeading, docIdx int, claimed map[int]bool,
	mkDiag MakeDiag,
) (int, []lint.Diagnostic) {
	sc := scopes[idx]
	ooSc := scopes[ooIdx]
	dh := docHeads[docIdx]
	diags := []lint.Diagnostic{mkDiag(f.Path, dh.Line,
		outOfOrderDiag(
			formatHeading(dh.Level, dh.Text),
			formatHeading(expectedLevel, sc.Heading),
			sch).Format())}
	diags = append(diags, levelDiagIfNeeded(f, sch, dh, expectedLevel, mkDiag)...)
	claimed[ooIdx] = true
	docIdx++
	if len(ooSc.Sections) > 0 {
		newIdx, childDiags := validateScopes(
			f, sch, ooSc.Sections, ooSc.Closed, docHeads, docIdx,
			expectedLevel+1, mkDiag)
		diags = append(diags, childDiags...)
		docIdx = newIdx
	}
	return docIdx, diags
}

func nextUnclaimed(cands []int, claimed map[int]bool, minIdx int) int {
	for _, i := range cands {
		if i >= minIdx && !claimed[i] {
			return i
		}
	}
	return -1
}

// findOutOfOrderIdx returns the first unclaimed scope at index >=
// minIdx that matches dh, scanning placeholder-bearing scopes too.
// requiredByText keys only fully-literal heading/alias text; a
// scope with placeholder interpolation in either its Heading or
// any of its Aliases falls through to the scopeMatchesHeading
// scan, so out-of-order detection still picks it up.
func findOutOfOrderIdx(
	scopes []Scope, dh DocHeading,
	requiredByText map[string][]int, claimed map[int]bool, minIdx int,
) int {
	if i := nextUnclaimed(requiredByText[dh.Text], claimed, minIdx); i >= 0 {
		return i
	}
	for i := minIdx; i < len(scopes); i++ {
		sc := scopes[i]
		if claimed[i] || sc.Wildcard {
			continue
		}
		if !scopeNeedsMatchScan(sc) {
			// Fully-literal scopes are already indexed in
			// requiredByText; nothing the fallback can find.
			continue
		}
		if scopeMatchesHeading(sc, dh) {
			return i
		}
	}
	return -1
}

// scopeNeedsMatchScan reports whether scopeMatchesHeading must be
// invoked to decide if a scope claims a heading. Fully-literal
// scopes are pre-indexed in requiredByText and don't need the
// fallback; scopes with placeholder interpolation in either
// Heading or Aliases do — and so does the "?" wildcard, which
// matches any text but can't appear in a literal-text map.
func scopeNeedsMatchScan(sc Scope) bool {
	if sc.Heading == "?" || fieldinterp.ContainsField(sc.Heading) {
		return true
	}
	for _, a := range sc.Aliases {
		if a == "?" || fieldinterp.ContainsField(a) {
			return true
		}
	}
	return false
}

// MatchesHeading reports whether sc matches the heading text in dh.
// Exported so callers outside the validator (notably the per-scope
// rule walker in internal/rules/requiredstructure) reuse the same
// matching semantics — anchored regex for field-interpolated
// patterns, exact text otherwise, plus aliases and the "?"
// wildcard.
func MatchesHeading(sc Scope, dh DocHeading) bool {
	return scopeMatchesHeading(sc, dh)
}

func scopeMatchesHeading(sc Scope, dh DocHeading) bool {
	if sc.Wildcard || sc.Preamble {
		// Wildcards never match a specific heading directly; the
		// preamble has no heading text to compare against.
		return false
	}
	if sc.Heading == "?" {
		return true
	}
	if matchesText(sc.Heading, dh.Text) {
		return true
	}
	for _, a := range sc.Aliases {
		if matchesText(a, dh.Text) {
			return true
		}
	}
	return false
}

// patternRegexCache memoises compiled regexes for field-interpolated
// heading patterns. Recompiling per-call would be O(scopes ×
// headings) on every validation pass; caching by pattern string
// keeps the hot loop allocation-free after warm-up.
//
// Stored values are *regexp.Regexp. A compile error is signalled
// by storing the patternCompileFailed sentinel — a dedicated
// non-nil pointer that the loader distinguishes from a successful
// entry by identity, avoiding the typed-nil-interface trap that
// would make `v == nil` silently fail.
var patternRegexCache sync.Map

// patternCompileFailed is the sentinel value stored in
// patternRegexCache when regexp.Compile failed. A separate value
// (instead of a typed-nil *regexp.Regexp) lets the loader
// distinguish "never tried" from "tried and failed" via a regular
// type assertion.
var patternCompileFailed = &regexp.Regexp{}

func matchesText(pattern, text string) bool {
	if !fieldinterp.ContainsField(pattern) {
		return pattern == text
	}
	re := patternRegex(pattern)
	if re == nil {
		return false
	}
	return re.MatchString(text)
}

func patternRegex(pattern string) *regexp.Regexp {
	if v, ok := patternRegexCache.Load(pattern); ok {
		re, ok := v.(*regexp.Regexp)
		if !ok || re == patternCompileFailed {
			return nil
		}
		return re
	}
	parts := fieldinterp.SplitOnFields(pattern)
	var b strings.Builder
	b.WriteString("^")
	for i, p := range parts {
		b.WriteString(regexp.QuoteMeta(p))
		if i < len(parts)-1 {
			b.WriteString(".+")
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		patternRegexCache.Store(pattern, patternCompileFailed)
		return nil
	}
	patternRegexCache.Store(pattern, re)
	return re
}

func formatHeading(level int, text string) string {
	return strings.Repeat("#", level) + " " + text
}

// validateFilename checks that the document basename matches the
// schema's filename pattern (if configured). A mismatched
// basename surfaces as a SchemaDiagnostic so the message mirrors
// the front-matter and path-pattern diagnostics: the "field" is
// "filename", the "actual" is the basename the user wrote, and
// the "expected" is the glob spelled out as a pattern-matching
// constraint.
func validateFilename(
	f *lint.File, sch *Schema, mkDiag MakeDiag,
) []lint.Diagnostic {
	pattern := sch.Require.Filename
	if pattern == "" {
		return nil
	}
	// Filename and path diagnostics describe the document as a
	// whole, not a body line; use the non-body anchor so the
	// engine's filterGeneratedDiags can't drop them when the
	// document body starts with a generated section.
	anchor := nonBodyDiagLine(f)
	base := filepath.Base(f.Path)
	matched, err := filepath.Match(pattern, base)
	if err != nil {
		// Malformed glob in the schema. Surface it via the same
		// SchemaDiagnostic shape so the message carries a
		// schema reference and the user can jump to the
		// offending pattern.
		d := SchemaDiagnostic{
			Field:     "filename pattern",
			Actual:    fmt.Sprintf("%q", pattern),
			Expected:  "valid glob",
			Hint:      err.Error(),
			SchemaRef: schemaRef(sch, ""),
		}
		return []lint.Diagnostic{mkDiag(f.Path, anchor, d.Format())}
	}
	if !matched {
		// `glob` makes the constraint syntax explicit: users
		// occasionally read `string matching <pattern>` as a regex
		// requirement, which filepath.Match does not accept. The
		// wording also lines up with the kind-level `path-pattern`
		// diagnostic ("path matching glob ...") so the user
		// vocabulary is consistent across both surfaces.
		d := SchemaDiagnostic{
			Field:     "filename",
			Actual:    fmt.Sprintf("%q", base),
			Expected:  fmt.Sprintf("filename matching glob %s", pattern),
			SchemaRef: schemaRef(sch, ""),
		}
		return []lint.Diagnostic{mkDiag(f.Path, anchor, d.Format())}
	}
	return nil
}

// ValidateFrontmatter compiles sch.Frontmatter into a CUE schema and
// unifies it with fm (the document's parsed front matter).
func ValidateFrontmatter(sch *Schema, fm map[string]any) error {
	expr := sch.FrontmatterCUE()
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	ctx := cuecontext.New()
	schemaVal := ctx.CompileString(expr)
	if err := schemaVal.Err(); err != nil {
		return fmt.Errorf("invalid CUE schema: %w", err)
	}
	if fm == nil {
		fm = map[string]any{}
	}
	data, err := json.Marshal(fm)
	if err != nil {
		return fmt.Errorf("serialize front matter: %w", err)
	}
	dataVal := ctx.CompileBytes(data)
	if err := dataVal.Err(); err != nil {
		return fmt.Errorf("compile front matter: %w", err)
	}
	merged := schemaVal.Unify(dataVal)
	if err := merged.Validate(cue.Concrete(true)); err != nil {
		return err
	}
	return nil
}

// ValidateFrontmatterSyntax checks that the schema's frontmatter
// constraints compile as CUE. Returns nil if there are no
// constraints.
func ValidateFrontmatterSyntax(sch *Schema) error {
	expr := sch.FrontmatterCUE()
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(expr)
	if err := v.Err(); err != nil {
		return fmt.Errorf("invalid schema frontmatter CUE: %w", err)
	}
	return nil
}
