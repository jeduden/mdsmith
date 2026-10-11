package requiredstructure

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/jeduden/mdsmith/internal/fieldinterp"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// docHeading represents a heading found in the document being checked.
type docHeading struct {
	Level int
	Text  string
	Line  int
}

// extractHeadings walks the AST and collects all headings. Memoized
// per File via lint.File.MemoFile: within one Check, the document
// File sees this called once, from checkSingleFileSchemaFromData,
// when the rule has a single schema source, or once per source from
// checkComposedSources's bodySyncDiagnostics loop when the rule
// composes N>=2 extends sources — Check's dispatch on len(sources)
// makes the two paths mutually exclusive, so a document composing N
// extends sources would otherwise re-walk the same AST N times per
// Check. fixBodySyncIn (the Fix path, single-source only) calls this
// too. parseSchemaWithRootFS also calls this, but on a throwaway
// schema-content File built fresh per call, so the memo there is a
// no-op rather than a shared cache.
func extractHeadings(f *lint.File) []docHeading {
	return f.MemoFile("requiredstructure.docHeadings", buildDocHeadings).([]docHeading)
}

// buildDocHeadings is the MemoFile-style builder for extractHeadings.
func buildDocHeadings(f *lint.File) any {
	var headings []docHeading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		text := headingText(h, f.Source)
		line := f.LineOfOffset(h.Lines().At(0).Start)

		headings = append(headings, docHeading{
			Level: h.Level,
			Text:  text,
			Line:  line,
		})
		return ast.WalkContinue, nil
	})
	return headings
}

// headingText extracts the plain text content of a heading node.
func headingText(h *ast.Heading, source []byte) string {
	var buf strings.Builder
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		writeNodeText(c, source, &buf)
	}
	return buf.String()
}

// writeNodeText recursively writes the text content of an AST node.
func writeNodeText(n ast.Node, source []byte, buf *strings.Builder) {
	if t, ok := n.(*ast.Text); ok {
		buf.Write(t.Segment.Value(source))
		return
	}
	if _, ok := n.(*ast.CodeSpan); ok {
		// Code spans store text in child nodes.
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			writeNodeText(c, source, buf)
		}
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		writeNodeText(c, source, buf)
	}
}

// headingMatchesLine checks if a docHeading matches a raw heading line.
func headingMatchesLine(h docHeading, line string) bool {
	// Strip leading # and spaces.
	stripped := strings.TrimLeft(line, "#")
	stripped = strings.TrimSpace(stripped)
	return stripped == h.Text
}

// checkStructure verifies required headings are present and in order.
func checkStructure(
	f *lint.File,
	sch *parsedSchema,
	docHeadings []docHeading,
	schemaSource string,
) []lint.Diagnostic {
	ref := buildSchemaRefForLegacy(schemaSource)
	requiredByText := buildRequiredByTextLegacy(sch.Headings)

	diags, docIdx, allowExtra := walkRequiredHeadings(
		f, sch, docHeadings, requiredByText, ref,
	)
	if !allowExtra {
		diags = append(diags, flagTrailingExtras(f, docHeadings, docIdx, ref)...)
	}
	return diags
}

// buildRequiredByTextLegacy maps each literal required heading text
// to its schema indices, so a doc heading seen at the "wrong"
// position can be recognized as an out-of-order required section
// rather than double-counted as both "unexpected" and "missing".
// Wildcard, `?`, and field-interpolated headings are excluded
// because their match depends on context.
func buildRequiredByTextLegacy(headings []schemaHeading) map[string][]int {
	out := map[string][]int{}
	for i, req := range headings {
		if isSectionWildcard(req) || req.Text == "?" {
			continue
		}
		if fieldinterp.ContainsField(req.Text) {
			continue
		}
		out[req.Text] = append(out[req.Text], i)
	}
	return out
}

// walkRequiredHeadings iterates the schema's heading list, matching
// each required entry against the document and emitting
// missing/unexpected/out-of-order diagnostics as it goes.
func walkRequiredHeadings(
	f *lint.File, sch *parsedSchema, docHeadings []docHeading,
	requiredByText map[string][]int, ref string,
) ([]lint.Diagnostic, int, bool) {
	var diags []lint.Diagnostic
	claimed := make(map[int]struct{})
	docIdx := 0
	allowExtra := false
	for schIdx, req := range sch.Headings {
		if isSectionWildcard(req) {
			allowExtra = true
			continue
		}
		if schema.IsClaimed(claimed, schIdx) {
			continue
		}
		// Save the position before the scan so that a missing-section
		// anchor uses the last correctly-placed heading, not the last
		// heading consumed (which may be an out-of-order entry).
		preScanIdx := docIdx
		reqDiags, newIdx, found := matchRequired(
			f, sch, docHeadings, docIdx, schIdx,
			requiredByText, claimed, allowExtra, ref,
		)
		diags = append(diags, reqDiags...)
		docIdx = newIdx
		if found {
			allowExtra = false
		}
		if !found && !schema.IsClaimed(claimed, schIdx) {
			diags = append(diags, missingSectionDiagLegacy(
				f, req, ref, legacyPrecedingLine(docHeadings, preScanIdx)))
		}
	}
	return diags, docIdx, allowExtra
}

// flagTrailingExtras emits one diagnostic per leftover document
// heading when no wildcard allows trailing extras.
func flagTrailingExtras(
	f *lint.File, docHeadings []docHeading, docIdx int, ref string,
) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for docIdx < len(docHeadings) {
		dh := docHeadings[docIdx]
		d := schema.SchemaDiagnostic{
			Field:     formatHeading(dh.Level, dh.Text),
			Actual:    "<present>",
			Expected:  "not declared in schema",
			SchemaRef: ref,
		}
		diags = append(diags, d.Emit(makeDiag, f.Path, dh.Line))
		docIdx++
	}
	return diags
}

// missingSectionDiagLegacy builds the SchemaDiagnostic for a
// required heading that the document lacks.
func missingSectionDiagLegacy(
	f *lint.File, req schemaHeading, ref string, precedingLine int,
) lint.Diagnostic {
	d := schema.SchemaDiagnostic{
		Field:     formatHeading(req.Level, req.Text),
		Actual:    "<missing>",
		Expected:  "section to be present",
		SchemaRef: ref,
	}
	// Anchor at the heading the missing section should follow so the
	// squiggle lands where the section belongs; MissingSectionAnchor
	// falls back to the non-body anchor when there is no preceding
	// heading or the insertion point sits inside a generated section
	// (where filterGeneratedDiags would otherwise drop it).
	return d.Emit(makeDiag, f.Path, schema.MissingSectionAnchor(f, precedingLine))
}

// legacyPrecedingLine returns the line of the document heading just
// before docIdx (the section a missing required heading should
// follow), or 0 when there is none.
func legacyPrecedingLine(docHeadings []docHeading, docIdx int) int {
	if idx := docIdx - 1; idx >= 0 && idx < len(docHeadings) {
		return docHeadings[idx].Line
	}
	return 0
}

// buildSchemaRefForLegacy returns the schema reference string
// used by SchemaDiagnostic in the file-schema (proto.md) code
// path. An empty source falls back to the generic "schema"
// label so every diagnostic still carries a reference suffix.
func buildSchemaRefForLegacy(source string) string {
	if source == "" {
		return "schema"
	}
	return source
}

// matchRequired advances docIdx to find a doc heading matching req.
// It emits diagnostics for intervening doc headings: "unexpected" when
// they don't match any required section, or "out of order" when they
// match a later required section (which is then claimed to avoid a
// follow-up "missing required" for the same text).
func matchRequired(
	f *lint.File,
	sch *parsedSchema,
	docHeadings []docHeading,
	docIdx, schIdx int,
	requiredByText map[string][]int,
	claimed map[int]struct{},
	allowExtra bool,
	schemaRef string,
) ([]lint.Diagnostic, int, bool) {
	var diags []lint.Diagnostic
	req := sch.Headings[schIdx]
	for docIdx < len(docHeadings) {
		dh := docHeadings[docIdx]
		if matchesSchema(req, dh) {
			if dh.Level != req.Level {
				diags = append(diags, levelMismatchDiag(f, dh, req, schemaRef))
			}
			claimed[schIdx] = struct{}{}
			return diags, docIdx + 1, true
		}
		if ooIdx := nextUnclaimed(requiredByText[dh.Text], claimed, schIdx+1); ooIdx >= 0 {
			other := sch.Headings[ooIdx]
			ooDiag := schema.SchemaDiagnostic{
				Field:     formatHeading(dh.Level, dh.Text),
				Actual:    "<out of order>",
				Expected:  "in declared order",
				Hint:      fmt.Sprintf("expected after %q", formatHeading(req.Level, req.Text)),
				SchemaRef: schemaRef,
			}
			diags = append(diags, ooDiag.Emit(makeDiag, f.Path, dh.Line))
			if dh.Level != other.Level {
				diags = append(diags, levelMismatchDiag(f, dh, other, schemaRef))
			}
			claimed[ooIdx] = struct{}{}
			docIdx++
			continue
		}
		if !allowExtra {
			unexpDiag := schema.SchemaDiagnostic{
				Field:     formatHeading(dh.Level, dh.Text),
				Actual:    "<present>",
				Expected:  "not declared in schema",
				Hint:      fmt.Sprintf("expected %q here instead", formatHeading(req.Level, req.Text)),
				SchemaRef: schemaRef,
			}
			diags = append(diags, unexpDiag.Emit(makeDiag, f.Path, dh.Line))
		}
		docIdx++
	}
	return diags, docIdx, false
}

// levelMismatchDiag builds a heading level-mismatch diagnostic that
// names the offending heading so readers can locate it quickly.
func levelMismatchDiag(
	f *lint.File, dh docHeading, req schemaHeading, schemaRef string,
) lint.Diagnostic {
	d := schema.SchemaDiagnostic{
		Field:     dh.Text,
		Actual:    "h" + strconv.Itoa(dh.Level),
		Expected:  "h" + strconv.Itoa(req.Level),
		SchemaRef: schemaRef,
	}
	return d.Emit(makeDiag, f.Path, dh.Line)
}

// nextUnclaimed returns the first index in candidates that is >= minIdx
// and not yet claimed, or -1 if none qualifies.
func nextUnclaimed(candidates []int, claimed map[int]struct{}, minIdx int) int {
	for _, idx := range candidates {
		if idx < minIdx {
			continue
		}
		if !schema.IsClaimed(claimed, idx) {
			return idx
		}
	}
	return -1
}

// matchesSchema checks if a document heading matches a schema heading.
// For headings with {field} references the regex was compiled once at
// schema parse time and stored in req.compiled, so no per-call NFA build.
// Invariant: req.compiled is non-nil whenever ContainsField(req.Text) is
// true — buildSchemaHeading guarantees this.
func matchesSchema(req schemaHeading, doc docHeading) bool {
	if req.Text == "?" {
		return true
	}
	if req.compiled != nil {
		return req.compiled.MatchString(doc.Text)
	}
	return doc.Text == req.Text
}

func isSectionWildcard(req schemaHeading) bool {
	return strings.TrimSpace(req.Text) == sectionWildcard
}

// resolveFields replaces {field} placeholders with frontmatter values
// using CUE path resolution for nested access.
func resolveFields(text string, docFM map[string]any) string {
	return fieldinterp.Interpolate(text, docFM)
}

// advanceToMatch advances docIdx to the next heading matching req.
// Returns the matched index (or -1) and the new docIdx.
func advanceToMatch(
	req schemaHeading, docHeadings []docHeading, docIdx int,
) (int, int) {
	for docIdx < len(docHeadings) {
		if matchesSchema(req, docHeadings[docIdx]) {
			return docIdx, docIdx + 1
		}
		docIdx++
	}
	return -1, docIdx
}

// checkSyncPoint checks a single sync point against the document.
func checkSyncPoint(
	f *lint.File, sp syncPoint, req schemaHeading,
	dh docHeading, matchedDoc int, docHeadings []docHeading,
	docFM map[string]any,
) []lint.Diagnostic {
	// Check if the field exists in front matter using CUE path resolution.
	path := fieldinterp.ParseCUEPath(sp.Field)
	if path == nil {
		return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
			fmt.Sprintf("invalid CUE path in sync placeholder: %q", sp.Field))}
	}
	if _, err := fieldinterp.ResolvePath(docFM, path); err != nil {
		return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
			fmt.Sprintf("sync placeholder %q refers to missing or invalid frontmatter path: %v",
				sp.Field, err))}
	}
	if !sp.InBody {
		expected := resolveFields(req.Text, docFM)
		if dh.Text != expected {
			return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
				fmt.Sprintf("heading does not match frontmatter: expected %q (from %s), got %q",
					expected, sp.Field, dh.Text))}
		}
		return nil
	}
	expected := resolveFields(sp.BodyText, docFM)
	return checkBodySync(f, dh, matchedDoc, docHeadings, expected, sp.Field)
}

// checkSync verifies frontmatter-body synchronization.
func checkSync(
	f *lint.File,
	sch *parsedSchema,
	docHeadings []docHeading,
	docFM map[string]any,
) []lint.Diagnostic {
	if len(docFM) == 0 {
		return nil
	}

	var diags []lint.Diagnostic
	docIdx := 0

	for schIdx, req := range sch.Headings {
		if isSectionWildcard(req) {
			continue
		}

		syncs := sch.SyncPoints[schIdx]
		if len(syncs) == 0 {
			_, docIdx = advanceToMatch(req, docHeadings, docIdx)
			continue
		}

		matchedDoc, newIdx := advanceToMatch(req, docHeadings, docIdx)
		docIdx = newIdx
		if matchedDoc < 0 {
			continue
		}

		dh := docHeadings[matchedDoc]
		for _, sp := range syncs {
			diags = append(diags,
				checkSyncPoint(f, sp, req, dh, matchedDoc, docHeadings, docFM)...)
		}
	}

	return diags
}

// checkBodySync checks that expected body text appears under the heading.
// It joins consecutive non-blank lines into paragraphs so that soft-wrapped
// descriptions still match their single-line frontmatter value.
func checkBodySync(
	f *lint.File,
	dh docHeading,
	headingIdx int,
	allHeadings []docHeading,
	expected string,
	field string,
) []lint.Diagnostic {
	// Determine the line range for body content under this heading.
	startLine := dh.Line + 1
	endLine := len(f.Lines)
	if headingIdx+1 < len(allHeadings) {
		endLine = allHeadings[headingIdx+1].Line - 1
	}

	// Convert expected once so per-line comparisons need no string() cast.
	expectedBytes := []byte(expected)

	// Single pass: trim each line once, check for a single-line match (fast path),
	// and accumulate consecutive non-blank lines into paragraphs for a joined match.
	// Pre-size para to the section line count to avoid growth allocs.
	para := make([][]byte, 0, max(0, endLine-startLine+1))
	for i := startLine - 1; i <= endLine && i <= len(f.Lines); i++ {
		var lineB []byte
		if i < endLine && i < len(f.Lines) {
			lineB = bytes.TrimFunc(f.Lines[i], unicode.IsSpace)
			if bytes.Equal(lineB, expectedBytes) {
				return nil
			}
		}
		if len(lineB) == 0 || i == endLine || i == len(f.Lines) {
			if len(para) > 0 {
				if bytes.Equal(bytes.Join(para, spaceSep), expectedBytes) {
					return nil
				}
				para = para[:0]
			}
			continue
		}
		para = append(para, lineB)
	}

	return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
		fmt.Sprintf("body does not match frontmatter field %q: expected %q", field, expected))}
}
