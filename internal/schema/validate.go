package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
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

	_, sd := validateScopes(f, sch, sch.Sections, sch.Closed, body, 0, rootLevel, docFM, mkDiag)
	diags = append(diags, sd...)

	diags = append(diags, ValidateContent(f, sch, docFM, mkDiag)...)

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
	anchor := nonBodyDiagLine(f)
	// Route the schema-side CompileString through RunCache.CompiledCUE
	// when one is in scope so N host files sharing a schema compile its
	// CUE source exactly once per Run. The wrapper carries the
	// *cue.Context the value was compiled against; the document
	// front-matter CompileBytes below must reuse that context because
	// cue.Value cannot cross contexts.
	var cache *lint.RunCache
	if f != nil {
		cache = f.RunCache
	}
	compiled := CachedCompile(cache, expr)
	schemaVal := compiled.Value
	ctx := compiled.Ctx
	if err := compiled.Err(); err != nil {
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
		// Skip the docFrontmatterKeyLines YAML re-parse when there
		// is nothing for the deprecation walker to do — the common
		// success path then pays no per-file overhead.
		if len(sch.FrontmatterMeta) == 0 || len(docFM) == 0 {
			return nil
		}
		return validateDeprecatedFieldsWithLines(
			f, sch, docFM, docFrontmatterKeyLines(f), mkDiag)
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
	out := dedupedCUEErrorDiags(f, sch, docFM, cueErrs, keyLines, mkDiag)
	return append(out, validateDeprecatedFieldsWithLines(f, sch, docFM, keyLines, mkDiag)...)
}

// dedupedCUEErrorDiags maps each unique CUE error to a
// SchemaDiagnostic. A struct dedup key avoids accidental collisions
// when one of the components (notably the raw-CUE-expression
// Expected fallback and a placeholder-bearing Field) legitimately
// contains the same delimiter a flat string key would have used.
func dedupedCUEErrorDiags(
	f *lint.File, sch *Schema, docFM map[string]any,
	cueErrs []errors.Error, keyLines map[string]int, mkDiag MakeDiag,
) []lint.Diagnostic {
	type dedupKey struct{ field, actual, expected string }
	seen := make(map[dedupKey]bool, len(cueErrs))
	out := make([]lint.Diagnostic, 0, len(cueErrs))
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

// validateDeprecatedFieldsWithLines walks sch.FrontmatterMeta and
// emits one Warning-severity diagnostic per deprecated field that
// still appears in docFM. The keyLines argument is reused from the
// CUE error loop so the deprecation diagnostic anchors at the same
// source line as a co-occurring type-mismatch error.
//
// `replaced-by:` rides on the lint.Diagnostic so LSP clients and CI
// scripts can route the warning without scanning the message body;
// the human-facing text honours `message:` first per plan 136. The
// parser guarantees every FrontmatterMeta entry has Deprecated=true
// (ExtractFieldMeta accepts the metadata form only when the
// `deprecated: true` discriminator is present), so the walker does
// not re-check the flag.
func validateDeprecatedFieldsWithLines(
	f *lint.File, sch *Schema, docFM map[string]any,
	keyLines map[string]int, mkDiag MakeDiag,
) []lint.Diagnostic {
	if len(sch.FrontmatterMeta) == 0 || len(docFM) == 0 {
		return nil
	}
	// Walk in sorted key order so a schema with two deprecated
	// fields and both present in the document emits diagnostics in
	// a stable order on every run.
	keys := make([]string, 0, len(sch.FrontmatterMeta))
	for k := range sch.FrontmatterMeta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []lint.Diagnostic
	for _, k := range keys {
		meta := sch.FrontmatterMeta[k]
		bare := strings.TrimSuffix(k, "?")
		if _, present := docFM[bare]; !present {
			continue
		}
		d := SchemaDiagnostic{
			Field:              bare,
			Deprecated:         true,
			ReplacedBy:         meta.ReplacedBy,
			DeprecationMessage: meta.Message,
			SchemaRef:          schemaRef(sch, k),
		}
		warn := mkDiag(f.Path, fmDiagLine(f, []string{bare}, keyLines), d.Format())
		warn.Severity = lint.Warning
		warn.Deprecated = true
		warn.ReplacedBy = meta.ReplacedBy
		out = append(out, warn)
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
// tolerated. A slot scope (`regex: '.+', repeat: {min: 0}`) always
// tolerates unlisted headings at its position.
func validateScopes(
	f *lint.File, sch *Schema, scopes []Scope, closed bool, docHeads []DocHeading,
	docIdx int, expectedLevel int, docFM map[string]any,
	mkDiag MakeDiag,
) (int, []lint.Diagnostic) {
	var diags []lint.Diagnostic
	claimed := make(map[int]bool)
	claimCounts := make(map[int]int)
	allowExtra := false

	for i, sc := range scopes {
		if sc.Preamble {
			// The preamble has no heading to match. Its rules: are
			// applied by the per-scope walker in MDS020 against the
			// [parent-start, first-child-heading) line range.
			claimed[i] = true
			continue
		}
		if isSlotMatcher(sc.Matcher) {
			allowExtra = true
			claimed[i] = true
			continue
		}
		if claimed[i] {
			continue
		}
		newIdx, scDiags, claimedThis := matchScope(
			f, sch, scopes, i, expectedLevel, docHeads, docIdx,
			claimed, claimCounts, allowExtra, closed, docFM, mkDiag)
		diags = append(diags, scDiags...)
		docIdx = newIdx
		if claimedThis {
			allowExtra = false
		} else if !claimed[i] && sc.Required() {
			// Missing sections have no body line to point at;
			// use the non-body anchor so filterGeneratedDiags
			// can't drop the diagnostic if body line 1 sits
			// inside a generated section.
			diags = append(diags, mkDiag(f.Path, nonBodyDiagLine(f),
				missingSectionDiag(
					formatHeading(expectedLevel, displayHeading(sc)), sch).Format()))
		}
	}

	newIdx, leftoverDiags := handleLeftoverHeadings(
		f, sch, scopes, claimed, claimCounts, docHeads, docIdx, expectedLevel,
		closed, allowExtra, docFM, mkDiag)
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
	f *lint.File, sch *Schema, scopes []Scope, claimed map[int]bool, claimCounts map[int]int,
	docHeads []DocHeading, docIdx, expectedLevel int,
	closed, allowExtra bool, docFM map[string]any, mkDiag MakeDiag,
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
		if idx := unclaimedListedScope(scopes, dh, claimed, docFM); idx >= 0 {
			newIdx, claimDiags := claimLateScope(
				f, sch, scopes, idx, expectedLevel, docHeads, docIdx,
				claimed, claimCounts, docFM, mkDiag)
			diags = append(diags, claimDiags...)
			docIdx = newIdx
			continue
		}
		// A heading that matches an already-claimed scope is an
		// extra occurrence after the scope's run has closed. The
		// trailing pass runs after every in-order matchScope has
		// returned, so every claimed scope here is finalised:
		// either the heading pushes the scope past its `max`
		// ("exceeds allowed occurrences") or it appears outside
		// the contiguous run ("out of order"). Both shapes are
		// surfaced explicitly rather than letting an open schema
		// absorb the heading silently.
		if idx, exceeded := claimedScopeMatches(
			scopes, dh, claimed, claimCounts, docFM); idx >= 0 {
			sc := scopes[idx]
			msg := fmt.Sprintf(
				"section %q out of order: matcher runs must be "+
					"contiguous with scope %q's earlier run",
				formatHeading(dh.Level, dh.Text),
				formatHeading(expectedLevel, displayHeading(sc)))
			if exceeded {
				msg = fmt.Sprintf(
					"section %q exceeds scope %q's allowed occurrences",
					formatHeading(dh.Level, dh.Text),
					formatHeading(expectedLevel, displayHeading(sc)))
			}
			diags = append(diags, mkDiag(f.Path, dh.Line, msg))
			docIdx++
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

// claimedScopeMatches reports the first already-claimed non-slot,
// non-preamble scope whose matcher accepts dh, increments that
// scope's claim count, and returns whether the new count pushes
// the scope past its `max`. Returns (-1, false) when no claimed
// scope matches.
//
// Callers decide the diagnostic wording: an `exceeded == true`
// match is a max-exceeded extra ("exceeds allowed occurrences"),
// while a within-max match is a non-contiguous extra ("out of
// order: matcher runs must be contiguous") that the caller may
// still need to suppress when it represents a contiguous
// continuation of a run still being assembled (see
// matchRun.handleNonMatch).
//
// Unbounded matchers (`max == 0`) never exceed; they return
// `(idx, false)` so callers can still detect non-contiguity.
func claimedScopeMatches(
	scopes []Scope, dh DocHeading, claimed map[int]bool,
	claimCounts map[int]int, docFM map[string]any,
) (int, bool) {
	for i, sc := range scopes {
		if !claimed[i] {
			continue
		}
		if sc.Preamble || isSlotMatcher(sc.Matcher) {
			continue
		}
		if !scopeMatchesHeading(sc, dh, docFM) {
			continue
		}
		_, max := sc.Matcher.Repeat.Bounds()
		claimCounts[i]++
		return i, max > 0 && claimCounts[i] > max
	}
	return -1, false
}

// claimLateScope marks a late-arriving listed scope as claimed,
// emits its out-of-order diagnostic, and recurses into the scope's
// nested children so missing-required-section diagnostics still
// surface beneath a late parent.
//
// Known limitation: this path consumes one heading per call.
// `repeat.max` exceeded by a subsequent same-text heading is
// flagged separately by handleLeftoverHeadings via
// claimedScopeMatches. A `repeat.min > 1` shortfall on a
// late-claimed scope is structurally unreachable here — the
// only paths that can leave a repeated scope unclaimed are
// already covered by matchScope's finishRun (in-order) or
// claimOutOfOrder's min check (out-of-order during iteration).
// Sequential ordering is not enforced for recovery paths; the
// in-order matchScope path is the canonical enforcement
// surface.
func claimLateScope(
	f *lint.File, sch *Schema, scopes []Scope, idx, expectedLevel int,
	docHeads []DocHeading, docIdx int,
	claimed map[int]bool, claimCounts map[int]int,
	docFM map[string]any, mkDiag MakeDiag,
) (int, []lint.Diagnostic) {
	sc := scopes[idx]
	dh := docHeads[docIdx]
	diags := []lint.Diagnostic{mkDiag(f.Path, dh.Line,
		outOfOrderDiag(formatHeading(dh.Level, dh.Text), "", sch).Format())}
	claimed[idx] = true
	claimCounts[idx]++
	docIdx++
	if len(sc.Sections) > 0 {
		newIdx, childDiags := validateScopes(
			f, sch, sc.Sections, sc.Closed,
			docHeads, docIdx, expectedLevel+1, docFM, mkDiag)
		diags = append(diags, childDiags...)
		docIdx = newIdx
	}
	return docIdx, diags
}

// unclaimedListedScope returns the index of the first unclaimed
// non-slot, non-preamble scope whose matcher accepts dh, or -1
// when no listed scope is a candidate.
func unclaimedListedScope(
	scopes []Scope, dh DocHeading, claimed map[int]bool,
	docFM map[string]any,
) int {
	for i, sc := range scopes {
		if claimed[i] || sc.Preamble || isSlotMatcher(sc.Matcher) {
			continue
		}
		if scopeMatchesHeading(sc, dh, docFM) {
			return i
		}
	}
	return -1
}

// matchScope advances docIdx looking for a run of headings that
// matches scopes[idx]'s matcher. Intervening doc headings either
// belong to a later listed scope (out-of-order), are unexpected
// (closed + no wildcard), or are descended into as part of an
// earlier scope's subtree. Returns the new docIdx, diagnostics,
// and whether the scope was claimed at least once.
func matchScope(
	f *lint.File, sch *Schema, scopes []Scope, idx, expectedLevel int,
	docHeads []DocHeading, docIdx int,
	claimed map[int]bool, claimCounts map[int]int,
	allowExtra, closed bool,
	docFM map[string]any, mkDiag MakeDiag,
) (int, []lint.Diagnostic, bool) {
	state := matchRun{
		f: f, sch: sch, scopes: scopes, idx: idx, expectedLevel: expectedLevel,
		claimed: claimed, claimCounts: claimCounts,
		allowExtra: allowExtra, closed: closed,
		docFM: docFM, mkDiag: mkDiag,
	}
	state.min, state.max = scopes[idx].Matcher.Repeat.Bounds()

	for docIdx < len(docHeads) && (state.max == 0 || state.consumed < state.max) {
		done, next, ok := state.step(docHeads, docIdx)
		docIdx = next
		if done {
			state.finishRun()
			return docIdx, state.diags, ok
		}
	}

	state.finishRun()
	// A run that hit its max while more matching headings remain
	// must flag the extras. Without this loop, the trailing-leftover
	// pass only flags them in closed schemas — silently accepting
	// `repeat: { max: N }` violations under the default open scope.
	docIdx = state.flagExtrasBeyondMax(docHeads, docIdx)
	return docIdx, state.diags, state.consumed > 0
}

// finishRun emits the per-run diagnostics that depend on the
// final consumed/digitsSeen state — `sequential:` ordering and
// the "matched N times, required at least M" guard. Called from
// every matchScope return path so a run that terminates on a
// non-matching heading (not just EOF or max) still gets these
// checks.
func (s *matchRun) finishRun() {
	sc := s.scopes[s.idx]
	if sc.Matcher.Sequential && len(s.digitsSeen) > 0 {
		if msg := sequentialDiagMessage(s.digitsSeen); msg != "" {
			s.diags = append(s.diags, s.mkDiag(s.f.Path, 1, msg))
		}
	}
	// A run that consumed fewer than min matches falls short of
	// the matcher's cardinality contract. The outer loop's
	// missing-required check only fires when `claimed[idx]` is
	// false; a partially-claimed run needs its own diagnostic.
	if s.consumed > 0 && s.consumed < s.min {
		s.diags = append(s.diags, s.mkDiag(s.f.Path, 1,
			fmt.Sprintf("section %q matched %d times, required at least %d",
				formatHeading(s.expectedLevel, displayHeading(sc)),
				s.consumed, s.min)))
	}
}

// flagExtrasBeyondMax walks doc headings still ahead of docIdx at
// the scope's expected level and emits a diagnostic for each one
// that matches the current matcher and isn't claimable by a later
// listed scope. The walk stops at the first heading that doesn't
// match the matcher so the caller sees the next "real" boundary.
// Returns the advanced docIdx (past any flagged extras).
func (s *matchRun) flagExtrasBeyondMax(docHeads []DocHeading, docIdx int) int {
	if s.max == 0 || s.consumed < s.max {
		return docIdx
	}
	sc := s.scopes[s.idx]
	for docIdx < len(docHeads) {
		dh := docHeads[docIdx]
		// Shallower headings belong to a parent scope — let the
		// outer walker decide. Deeper headings are body content of
		// a section we've already claimed; skip them so a later
		// same-level extra isn't hidden behind a nested heading.
		if dh.Level < s.expectedLevel {
			return docIdx
		}
		if dh.Level > s.expectedLevel {
			docIdx++
			continue
		}
		matched, _ := matchHeading(sc.Matcher, dh, s.docFM)
		if !matched {
			return docIdx
		}
		if claimsLaterLiteral(s.scopes, s.idx+1, dh, s.claimed, s.docFM) {
			return docIdx
		}
		s.diags = append(s.diags, s.mkDiag(s.f.Path, dh.Line,
			fmt.Sprintf("section %q matched %d times, allowed at most %d",
				formatHeading(dh.Level, dh.Text),
				s.consumed+1, s.max)))
		s.consumed++
		docIdx++
	}
	return docIdx
}

// matchRun threads the per-iteration state of matchScope through
// helper methods so the top-level loop reads as a straight pipe of
// "advance, claim, or break" decisions.
type matchRun struct {
	f             *lint.File
	sch           *Schema
	scopes        []Scope
	idx           int
	expectedLevel int
	claimed       map[int]bool
	claimCounts   map[int]int
	allowExtra    bool
	closed        bool
	docFM         map[string]any
	mkDiag        MakeDiag
	min, max      int
	consumed      int
	digitsSeen    []string
	diags         []lint.Diagnostic
}

// step processes one doc heading. Returns (done, newDocIdx, ok)
// where done==true exits the outer loop early.
func (s *matchRun) step(docHeads []DocHeading, docIdx int) (bool, int, bool) {
	sc := s.scopes[s.idx]
	dh := docHeads[docIdx]
	if dh.Level < s.expectedLevel {
		// Broad matchers (`.+`) match everything, including
		// parent-level siblings. Claiming such a heading here
		// would consume the parent walker's next sibling and
		// flag it as a level mismatch even though it never
		// belonged to this nested run. Restrict the
		// shallower-heading recovery to non-broad matchers so
		// only an authored wrong-level heading (e.g. an `## X`
		// where the schema wanted `### X`) is salvaged.
		if matched, captured := matchHeading(sc.Matcher, dh, s.docFM); matched &&
			!isBroadMatcher(sc.Matcher) {
			// A shallower-than-expected heading that still matches
			// the matcher counts toward the run's consumed/digits
			// state — claimMatch emits the level-mismatch
			// diagnostic on its own. Returning done=false lets the
			// outer loop continue scanning for additional matches.
			return false, s.claimMatch(docHeads, docIdx, captured), false
		}
		return true, docIdx, s.consumed > 0
	}
	matched, captured := matchHeading(sc.Matcher, dh, s.docFM)
	if matched {
		// Yield to a later specific scope whenever the current
		// matcher is broad (`.+`) — even before reaching its min.
		// A broad matcher's job is to absorb leftovers; stealing
		// a heading the user named separately would either flag
		// that section as missing or hide its content/rules. The
		// repeat.min shortfall surfaces in finishRun if the
		// broad matcher couldn't collect enough leftovers. For
		// non-broad matchers, only yield once min is satisfied,
		// so a required specific scope still claims its first
		// occurrence even when the heading text overlaps a later
		// literal entry.
		if (isBroadMatcher(sc.Matcher) || s.consumed >= s.min) &&
			claimsLaterLiteral(s.scopes, s.idx+1, dh, s.claimed, s.docFM) {
			return true, docIdx, s.consumed > 0
		}
		return false, s.claimMatch(docHeads, docIdx, captured), false
	}
	// A deeper-than-expected heading is part of an earlier claimed
	// scope's body, not a sibling boundary — skip it so the run
	// keeps scanning for the next same-level match.
	if dh.Level > s.expectedLevel {
		return false, docIdx + 1, false
	}
	if s.consumed > 0 {
		// An active run is contiguous; the first same-level
		// non-match closes it.
		return true, docIdx, true
	}
	if s.consumed >= s.min {
		// Optional matcher (min=0) hasn't started yet. Yield to a
		// later scope when the heading would claim it — including
		// a later broad matcher, since the broad matcher is the
		// natural absorber for a heading the optional scope did
		// not match. Without yielding to broad here, the broad
		// scope would later report "missing required" because the
		// optional scope already advanced past its heading.
		if anyLaterScopeClaims(s.scopes, s.idx+1, dh, s.claimed, s.docFM) {
			return true, docIdx, false
		}
		if !s.allowExtra && s.closed {
			s.diags = append(s.diags, s.mkDiag(s.f.Path, dh.Line,
				unexpectedSectionDiag(
					formatHeading(dh.Level, dh.Text),
					formatHeading(s.expectedLevel, displayHeading(sc)),
					s.sch).Format()))
		}
		return false, docIdx + 1, false
	}
	return s.handleNonMatch(docHeads, docIdx)
}

func (s *matchRun) claimMatch(docHeads []DocHeading, docIdx int, captured string) int {
	sc := s.scopes[s.idx]
	dh := docHeads[docIdx]
	s.diags = append(s.diags, levelDiagIfNeeded(s.f, s.sch, dh, s.expectedLevel, s.mkDiag)...)
	s.claimed[s.idx] = true
	s.claimCounts[s.idx]++
	if len(sc.Sections) > 0 {
		newIdx, childDiags := validateScopes(
			s.f, s.sch, sc.Sections, sc.Closed, docHeads, docIdx+1,
			s.expectedLevel+1, s.docFM, s.mkDiag)
		s.diags = append(s.diags, childDiags...)
		docIdx = newIdx
	} else {
		docIdx++
	}
	s.consumed++
	if captured != "" {
		s.digitsSeen = append(s.digitsSeen, captured)
	}
	return docIdx
}

// handleNonMatch processes a heading that didn't match the current
// matcher and that the run hasn't yet satisfied its `min` for. The
// caller (step) has already filtered out deeper-than-expected
// headings, so dh is always at the expected level here.
func (s *matchRun) handleNonMatch(docHeads []DocHeading, docIdx int) (bool, int, bool) {
	dh := docHeads[docIdx]
	if ooIdx := findOutOfOrderIdx(s.scopes, dh, s.claimed, s.idx+1, s.docFM); ooIdx >= 0 {
		newIdx, ooDiags := claimOutOfOrder(
			s.f, s.sch, s.scopes, s.idx, ooIdx, s.expectedLevel, docHeads, docIdx,
			s.claimed, s.claimCounts, s.docFM, s.mkDiag)
		s.diags = append(s.diags, ooDiags...)
		return false, newIdx, false
	}
	// A heading that matches an already-claimed scope falls into
	// one of three buckets:
	//   1. Contiguous continuation of a run still being assembled
	//      in this very iteration (e.g. [A, B] with doc [B, B, A]
	//      where claimOutOfOrder opened B's run; the second B
	//      belongs to that same run). idx >= s.idx and the new
	//      count is within max — silently absorb.
	//   2. Past `max`: emit "exceeds allowed occurrences".
	//   3. Non-contiguous extra: the scope's run was closed by an
	//      earlier scope iteration (idx < s.idx) and the new
	//      heading is within max. Emit "out of order: matcher
	//      runs must be contiguous" so the user sees the ordering
	//      violation even in an open schema.
	if idx, exceeded := claimedScopeMatches(
		s.scopes, dh, s.claimed, s.claimCounts, s.docFM); idx >= 0 {
		if !exceeded && idx >= s.idx {
			// Contiguous continuation of a run still being
			// assembled. Recurse into the scope's children so
			// nested required sections still surface for every
			// occurrence of the run, matching claimMatch's
			// in-order behavior — otherwise a repeated scope
			// with required children would only validate the
			// first occurrence's subtree.
			contSc := s.scopes[idx]
			if len(contSc.Sections) > 0 {
				newIdx, childDiags := validateScopes(
					s.f, s.sch, contSc.Sections, contSc.Closed,
					docHeads, docIdx+1, s.expectedLevel+1,
					s.docFM, s.mkDiag)
				s.diags = append(s.diags, childDiags...)
				return false, newIdx, false
			}
			return false, docIdx + 1, false
		}
		sc := s.scopes[idx]
		msg := fmt.Sprintf(
			"section %q out of order: matcher runs must be "+
				"contiguous with scope %q's earlier run",
			formatHeading(dh.Level, dh.Text),
			formatHeading(s.expectedLevel, displayHeading(sc)))
		if exceeded {
			msg = fmt.Sprintf(
				"section %q exceeds scope %q's allowed occurrences",
				formatHeading(dh.Level, dh.Text),
				formatHeading(s.expectedLevel, displayHeading(sc)))
		}
		s.diags = append(s.diags, s.mkDiag(s.f.Path, dh.Line, msg))
		return false, docIdx + 1, false
	}
	if !s.allowExtra && s.closed {
		sc := s.scopes[s.idx]
		s.diags = append(s.diags, s.mkDiag(s.f.Path, dh.Line,
			unexpectedSectionDiag(
				formatHeading(dh.Level, dh.Text),
				formatHeading(s.expectedLevel, displayHeading(sc)),
				s.sch).Format()))
	}
	return false, docIdx + 1, false
}

// laterScopeMatches reports whether dh matches any later scope in
// the same parent window that is more specific than a broad
// matcher. Used by the per-scope walkers (rules, content,
// acronyms) so a broad repeated matcher does not consume a
// heading that a later named scope would claim. The walker
// version does not need a scope-index `claimed` map because each
// walker iterates scopes in declared order and acts immediately.
func laterScopeMatches(
	scopes []Scope, startIdx int, dh DocHeading,
	docFM map[string]any,
) bool {
	for i := startIdx; i < len(scopes); i++ {
		sc := scopes[i]
		if sc.Preamble ||
			isSlotMatcher(sc.Matcher) || isBroadMatcher(sc.Matcher) {
			continue
		}
		if scopeMatchesHeading(sc, dh, docFM) {
			return true
		}
	}
	return false
}

// anyLaterScopeClaims reports whether dh matches any unclaimed
// non-slot, non-preamble later scope — including broad
// matchers. Used by the unmatched-optional yield path so an
// optional scope that doesn't match the current heading still
// yields to a broad matcher waiting to absorb leftovers
// (e.g. an `A optional` + `.+ min=1` pair against a `## Body`
// heading). Differs from `claimsLaterLiteral` only in that it
// keeps broad matchers in the search.
func anyLaterScopeClaims(
	scopes []Scope, startIdx int, dh DocHeading,
	claimed map[int]bool, docFM map[string]any,
) bool {
	for i := startIdx; i < len(scopes); i++ {
		sc := scopes[i]
		if claimed[i] || sc.Preamble || isSlotMatcher(sc.Matcher) {
			continue
		}
		if scopeMatchesHeading(sc, dh, docFM) {
			return true
		}
	}
	return false
}

// claimsLaterLiteral reports whether dh matches any unclaimed
// later scope that is more specific than a broad/slot matcher.
// Used so a slot or repeat-run does not absorb a heading the
// user named separately further down the list. Optional scopes
// participate (a `regex: 'A'` with `repeat: { min: 0, max: 1 }`
// still wants the heading), but later scopes with a broad `.+`
// regex are skipped — yielding to them would let a generic
// catch-all steal a heading from a specific predecessor.
func claimsLaterLiteral(
	scopes []Scope, startIdx int, dh DocHeading,
	claimed map[int]bool, docFM map[string]any,
) bool {
	for i := startIdx; i < len(scopes); i++ {
		sc := scopes[i]
		if claimed[i] || sc.Preamble ||
			isSlotMatcher(sc.Matcher) || isBroadMatcher(sc.Matcher) {
			continue
		}
		if scopeMatchesHeading(sc, dh, docFM) {
			return true
		}
	}
	return false
}

// sequentialDiagMessage scans captured digit strings for ordering
// violations: each number must be strictly greater than its
// predecessor, no gaps allowed.
func sequentialDiagMessage(captured []string) string {
	prev := -1
	for _, s := range captured {
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Sprintf(
				"sequential captures must be integers; got %q", s)
		}
		if prev < 0 {
			prev = n
			continue
		}
		if n != prev+1 {
			return fmt.Sprintf(
				"sequential numbering out of order: expected %d, got %d",
				prev+1, n)
		}
		prev = n
	}
	return ""
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

// claimOutOfOrder records that docHeads[docIdx] matches scopes[ooIdx]
// (a later listed scope), emits the out-of-order diagnostic, and
// recurses into the matched scope's child sections.
func claimOutOfOrder(
	f *lint.File, sch *Schema, scopes []Scope, idx, ooIdx, expectedLevel int,
	docHeads []DocHeading, docIdx int,
	claimed map[int]bool, claimCounts map[int]int,
	docFM map[string]any, mkDiag MakeDiag,
) (int, []lint.Diagnostic) {
	sc := scopes[idx]
	ooSc := scopes[ooIdx]
	dh := docHeads[docIdx]
	diags := []lint.Diagnostic{mkDiag(f.Path, dh.Line,
		outOfOrderDiag(
			formatHeading(dh.Level, dh.Text),
			formatHeading(expectedLevel, displayHeading(sc)),
			sch).Format())}
	diags = append(diags, levelDiagIfNeeded(f, sch, dh, expectedLevel, mkDiag)...)
	// Count the contiguous run of same-level headings starting at
	// docIdx that match ooSc. The recovery path still claims only
	// the leading occurrence (so child recursion has a single
	// anchor), but counting the full run avoids reporting a false
	// `repeat.min` shortfall when the document does contain enough
	// occurrences — they are simply in the wrong position, which
	// the out-of-order diagnostic already calls out.
	runLen := countMatchingRun(ooSc, docHeads, docIdx, expectedLevel, docFM)
	if ooSc.Matcher != nil && runLen < ooSc.Matcher.Repeat.Min {
		diags = append(diags, mkDiag(f.Path, dh.Line,
			fmt.Sprintf(
				"section %q matched %d times, required at least %d",
				formatHeading(expectedLevel, displayHeading(ooSc)),
				runLen, ooSc.Matcher.Repeat.Min)))
	}
	if ooSc.Matcher != nil && ooSc.Matcher.Sequential {
		diags = append(diags, mkDiag(f.Path, dh.Line,
			fmt.Sprintf(
				"section %q sequential ordering is not enforced "+
					"on out-of-order claims; move the scope to its "+
					"natural position for sequential checks",
				formatHeading(expectedLevel, displayHeading(ooSc)))))
	}
	claimed[ooIdx] = true
	claimCounts[ooIdx]++
	docIdx++
	if len(ooSc.Sections) > 0 {
		newIdx, childDiags := validateScopes(
			f, sch, ooSc.Sections, ooSc.Closed, docHeads, docIdx,
			expectedLevel+1, docFM, mkDiag)
		diags = append(diags, childDiags...)
		docIdx = newIdx
	}
	return docIdx, diags
}

// countMatchingRun returns the number of contiguous same-level
// headings starting at docIdx that match sc's matcher. Deeper
// headings (level > expectedLevel) are skipped as body content of
// an already-matched section, mirroring matchScope's run
// semantics. The walk stops at the first same-level non-match or
// the first shallower-than-expected heading.
func countMatchingRun(
	sc Scope, docHeads []DocHeading, docIdx, expectedLevel int,
	docFM map[string]any,
) int {
	run := 0
	for i := docIdx; i < len(docHeads); i++ {
		dh := docHeads[i]
		if dh.Level < expectedLevel {
			return run
		}
		if dh.Level > expectedLevel {
			continue
		}
		if matched, _ := matchHeading(sc.Matcher, dh, docFM); matched {
			run++
			continue
		}
		return run
	}
	return run
}

// findOutOfOrderIdx returns the first unclaimed scope at index >=
// minIdx that matches dh.
func findOutOfOrderIdx(
	scopes []Scope, dh DocHeading,
	claimed map[int]bool, minIdx int, docFM map[string]any,
) int {
	for i := minIdx; i < len(scopes); i++ {
		sc := scopes[i]
		if claimed[i] || sc.Preamble || isSlotMatcher(sc.Matcher) {
			continue
		}
		if scopeMatchesHeading(sc, dh, docFM) {
			return i
		}
	}
	return -1
}

// ScopeRunIndices returns the doc-heading indices that
// scopes[currentIdx] would claim inside the
// [parentStart, parentEnd) window, following the same
// contiguous-run semantics matchScope uses: scan forward from
// the first match for additional same-level matches, but stop at
// the first same-level heading that does not match (deeper
// headings inside the matched section are skipped silently).
//
// The helper also mirrors matchScope's yield rules so per-scope
// walkers stay in step with structural validation:
//   - A broad matcher (`regex: '.+'`) yields to any later named
//     scope whose matcher would claim the heading.
//   - A non-broad matcher yields to a later named scope only
//     after its `repeat.min` has been satisfied.
//
// When no in-level match is found the helper falls back to the
// first wrong-level match in the window. Used by the per-scope
// walkers (acronyms, content, rules) so they only visit
// occurrences the structural validator would also have claimed
// as part of the same run.
func ScopeRunIndices(
	scopes []Scope, currentIdx int, heads []DocHeading,
	expectedLevel, parentStart, parentEnd int,
	claimed map[int]bool, docFM map[string]any,
) []int {
	sc := scopes[currentIdx]
	if sc.Matcher == nil {
		return nil
	}
	out := scanScopeRunAtLevel(
		scopes, currentIdx, heads, expectedLevel, parentStart, parentEnd,
		claimed, docFM)
	if len(out) > 0 {
		return out
	}
	// No in-level match — fall back to the first wrong-level
	// match. Wrong-level matches never form a run (the heading
	// already deviates from the schema's level expectation).
	if idx := firstWrongLevelMatch(
		sc, heads, expectedLevel, parentStart, parentEnd, claimed, docFM); idx >= 0 {
		return []int{idx}
	}
	return nil
}

// scanScopeRunAtLevel scans heads in source order for contiguous
// matches of scopes[currentIdx] at expectedLevel. The run ends
// when its bounds are exhausted, when a same-level non-match
// closes the run, or when a heading would be claimed by a more
// specific later scope (broad matchers yield from the start;
// non-broad yield only after `repeat.min` is satisfied).
func scanScopeRunAtLevel(
	scopes []Scope, currentIdx int, heads []DocHeading,
	expectedLevel, parentStart, parentEnd int,
	claimed map[int]bool, docFM map[string]any,
) []int {
	sc := scopes[currentIdx]
	min, max := sc.Matcher.Repeat.Bounds()
	isBroad := isBroadMatcher(sc.Matcher)
	var out []int
	started := false
	for i, h := range heads {
		if claimed[i] {
			continue
		}
		if h.Line < parentStart || h.Line >= parentEnd {
			continue
		}
		if h.Level != expectedLevel {
			if h.Level < expectedLevel && started {
				break
			}
			continue
		}
		if scopeMatchesHeading(sc, h, docFM) {
			if (isBroad || len(out) >= min) &&
				laterScopeMatches(scopes, currentIdx+1, h, docFM) {
				break
			}
			out = append(out, i)
			started = true
			if max == 1 || (max > 0 && len(out) >= max) {
				break
			}
			continue
		}
		if started {
			break
		}
	}
	return out
}

// firstWrongLevelMatch returns the index of the first heading in
// the window that matches sc at any level other than
// expectedLevel, or -1 when none exists. The walker uses this
// fallback so a section authored at the wrong heading depth still
// gets its per-scope checks applied. Broad matchers (`.+`) are
// skipped — they would otherwise pair the slot with any sibling
// in the parent window, making per-scope rule/content/acronym
// checks fire against the wrong section.
func firstWrongLevelMatch(
	sc Scope, heads []DocHeading,
	expectedLevel, parentStart, parentEnd int,
	claimed map[int]bool, docFM map[string]any,
) int {
	if isBroadMatcher(sc.Matcher) {
		return -1
	}
	for i, h := range heads {
		if claimed[i] {
			continue
		}
		if h.Line < parentStart || h.Line >= parentEnd {
			continue
		}
		if h.Level == expectedLevel {
			continue
		}
		if scopeMatchesHeading(sc, h, docFM) {
			return i
		}
	}
	return -1
}

// MatchesHeading reports whether sc matches dh's heading text.
// Exported so callers outside the validator (notably the per-scope
// rule walker in internal/rules/requiredstructure) reuse the same
// matching semantics. fm is the document's parsed front matter and
// must be supplied so `\#(fmvar(...))` patterns resolve correctly;
// pass nil only when the schema is known to use literal-only
// matchers.
func MatchesHeading(sc Scope, dh DocHeading, fm map[string]any) bool {
	return scopeMatchesHeading(sc, dh, fm)
}

func scopeMatchesHeading(sc Scope, dh DocHeading, fm map[string]any) bool {
	if sc.Preamble || sc.Matcher == nil {
		return false
	}
	if isSlotMatcher(sc.Matcher) {
		// Slots are positional placeholders, not identity claimants;
		// callers that want to know whether a heading falls inside
		// a slot use the walker's logic instead.
		return false
	}
	matched, _ := matchHeading(sc.Matcher, dh, fm)
	return matched
}

// displayHeading renders a scope's label for diagnostics. The bare
// heading text is preferred when it survives parsing; otherwise
// the matcher's regex body stands in.
func displayHeading(sc Scope) string {
	if sc.Heading != "" {
		return sc.Heading
	}
	if sc.Matcher != nil {
		return sc.Matcher.Regex
	}
	return ""
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
	pattern := sch.Filename
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
