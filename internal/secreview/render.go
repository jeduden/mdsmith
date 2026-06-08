package secreview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// renderFiles names the three artifacts Render writes, in the order
// render_findings.py prints them.
var renderFiles = []string{"findings.sarif", "security-review.md", "inline-annotations.json"}

// Render writes the three review outputs (findings.sarif,
// security-review.md, inline-annotations.json) into outDir, creating the
// directory if needed. It mirrors render_findings.py.
func Render(r *Report, outDir string) error {
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
	contents := map[string][]byte{
		"findings.sarif":          sarif,
		"security-review.md":      []byte(buildReport(r, time.Now())),
		"inline-annotations.json": annotations,
	}
	for _, name := range renderFiles {
		if err := os.WriteFile(filepath.Join(outDir, name), contents[name], 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

// RenderFileNames returns the basenames Render writes, in print order, so
// callers can report them without hardcoding the list.
func RenderFileNames() []string {
	out := make([]string, len(renderFiles))
	copy(out, renderFiles)
	return out
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
