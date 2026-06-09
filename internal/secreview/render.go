package secreview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// renderFiles names the three artifacts Render writes, in the order
// render_findings.py prints them.
var renderFiles = []string{"findings.sarif", "security-review.md", "inline-annotations.json"}

// Render writes the three review outputs (findings.sarif,
// security-review.md, inline-annotations.json) into outDir, creating the
// directory if needed. It mirrors render_findings.py.
func Render(r *Report, outDir string) error {
	return RenderStem(r, outDir, "")
}

// RenderStem is Render with a filename stem. A non-empty stem names the
// artifacts <stem>.sarif, <stem>.md, and <stem>.inline-annotations.json
// so successive dated reviews can coexist in one directory (the
// docs/security/ convention). An empty stem keeps the legacy fixed names.
func RenderStem(r *Report, outDir, stem string) error {
	if err := validateStem(stem); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}
	sarif, err := marshalJSON(buildSARIF(r))
	if err != nil {
		return fmt.Errorf("marshal sarif: %w", err)
	}
	annotations, err := marshalJSON(buildAnnotations(r))
	if err != nil {
		return fmt.Errorf("marshal annotations: %w", err)
	}
	// names and data are parallel: RenderFileNamesStem returns the
	// SARIF, report, and annotation basenames in that fixed order.
	names := RenderFileNamesStem(stem)
	data := [][]byte{sarif, []byte(buildReport(r, time.Now())), annotations}
	for i, name := range names {
		if err := os.WriteFile(filepath.Join(outDir, name), data[i], 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

// validateStem rejects a stem that is anything other than a single safe
// filename component. Path separators or a parent ref ("..") would let
// the stem escape outDir; whitespace and a leading dot break the
// docs/security/*.md catalog glob the rendered report is indexed by. An
// empty stem is allowed — it selects the legacy fixed names.
func validateStem(stem string) error {
	if stem == "" {
		return nil
	}
	if stem != filepath.Base(stem) || stem == "." || stem == ".." {
		return fmt.Errorf("invalid --stem %q: must be a bare filename, not a path", stem)
	}
	if strings.ContainsAny(stem, `/\`) || strings.ContainsRune(stem, os.PathSeparator) {
		return fmt.Errorf("invalid --stem %q: must not contain a path separator", stem)
	}
	if strings.ContainsFunc(stem, unicode.IsSpace) {
		return fmt.Errorf("invalid --stem %q: must not contain whitespace", stem)
	}
	if strings.HasPrefix(stem, ".") {
		return fmt.Errorf("invalid --stem %q: must not start with a dot", stem)
	}
	return nil
}

// RenderFileNames returns the basenames Render writes, in print order, so
// callers can report them without hardcoding the list.
func RenderFileNames() []string {
	return RenderFileNamesStem("")
}

// RenderFileNamesStem returns the basenames RenderStem writes for the
// given stem, in print order: SARIF, report, annotations. An empty stem
// returns the legacy fixed names.
func RenderFileNamesStem(stem string) []string {
	if stem == "" {
		out := make([]string, len(renderFiles))
		copy(out, renderFiles)
		return out
	}
	return []string{
		stem + ".sarif",
		stem + ".md",
		stem + ".inline-annotations.json",
	}
}

// marshalJSON renders v as 2-space-indented JSON with HTML escaping
// disabled, so directive and Go tokens like `<?build?>`, `<`, `>`, and `&`
// survive verbatim instead of being escaped to `<` etc. Non-ASCII
// (em dash, `·`, `§`) is emitted as raw UTF-8 — valid JSON, and the literal
// characters the output-formats doc shows. (Python's json.dumps default
// escapes non-ASCII to \uXXXX instead; the decoded data is identical.) The
// single trailing newline json.Encoder appends is trimmed.
func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// sortedFindings returns findings ordered by severity (high to low), then
// confidence (confirmed, likely, tentative), then id — matching
// render_findings.py's sort_key.
func sortedFindings(fs []Finding) []Finding {
	out := make([]Finding, len(fs))
	copy(out, fs)
	sort.SliceStable(out, func(i, j int) bool {
		return less(out[i], out[j])
	})
	return out
}

// less reports whether finding a sorts before finding b.
func less(a, b Finding) bool {
	if severityOrder[a.Severity] != severityOrder[b.Severity] {
		return severityOrder[a.Severity] < severityOrder[b.Severity]
	}
	ca, cb := confOrder(a.Confidence), confOrder(b.Confidence)
	if ca != cb {
		return ca < cb
	}
	return a.ID < b.ID
}

// confOrder ranks a confidence label, with unknown values last (rank 3),
// matching the Python dict's default.
func confOrder(c string) int {
	if rank, ok := confidenceOrder[c]; ok {
		return rank
	}
	return 3
}

// locStr renders a location as "file:start" or "file:start-end", or an
// em dash when there is no location, matching render_findings.py.
func locStr(loc *Location) string {
	if loc == nil {
		return "—"
	}
	s := loc.File
	if s == "" {
		s = "?"
	}
	if loc.StartLine != 0 {
		s += fmt.Sprintf(":%d", loc.StartLine)
		if loc.EndLine != 0 && loc.EndLine != loc.StartLine {
			s += fmt.Sprintf("-%d", loc.EndLine)
		}
	}
	return s
}
