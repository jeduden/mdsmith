// Package tableformat implements MDS025, the single table rule.
// It owns table parsing, the three structural checks (MD055
// table-pipe-style, MD056 table-column-count, MD058
// blanks-around-tables), and the prettier-style alignment pass that
// gives the rule its name.
package tableformat

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/internal/rules/tablefmt"
)

func init() {
	rule.Register(&Rule{
		Pad:            1,
		SeparatorStyle: tablefmt.SeparatorSpaced,
		Style:          StyleConsistent,
	})
}

// Rule gates table well-formedness: edge-pipe style (MD055), column
// count vs the header (MD056), surrounding blank lines (MD058), and
// the column-alignment / padding pass that gives the rule its name.
type Rule struct {
	Pad            int // spaces on each side of cell content
	SeparatorStyle tablefmt.SeparatorStyle
	Style          string // edge-pipe style: one of the Style* constants
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS025" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "table-format" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "table" }

// GetPad returns the current pad setting.
func (r *Rule) GetPad() int { return r.Pad }

// GetSeparatorStyle returns the active separator style.
func (r *Rule) GetSeparatorStyle() tablefmt.SeparatorStyle { return r.SeparatorStyle }

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "pad":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf("table-format: pad must be an integer, got %T", v)
			}
			if n < 0 {
				return fmt.Errorf("table-format: pad must be non-negative, got %d", n)
			}
			r.Pad = n
		case "separator-style":
			style, err := tablefmt.ParseSeparatorStyle(v, "table-format")
			if err != nil {
				return err
			}
			r.SeparatorStyle = style
		case "style":
			str, ok := v.(string)
			if !ok {
				return fmt.Errorf("table-format: style must be a string, got %T", v)
			}
			switch str {
			case StyleConsistent, StyleLeadingAndTrailing, StyleNoLeadingOrTrailing:
				r.Style = str
			default:
				return fmt.Errorf(
					"table-format: invalid style %q (valid: %s, %s, %s)",
					str, StyleConsistent, StyleLeadingAndTrailing, StyleNoLeadingOrTrailing)
			}
		default:
			return fmt.Errorf("table-format: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"pad":             1,
		"separator-style": "spaced",
		"style":           StyleConsistent,
	}
}

// Check implements rule.Rule. It emits both the structural diagnostics
// (MD055/056/058) and the alignment diagnostics produced by the
// prettier-style format pass.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	// Early-exit: a GFM table requires `|` somewhere in the source.
	// Skipping the AST-walk for code lines and the per-line table
	// detection on files without a pipe byte avoids the rule's
	// dominant per-Check allocator on table-free corpora.
	if bytes.IndexByte(f.Source, '|') < 0 {
		return nil
	}
	skipLines := formatSkipLines(f)
	var diags []lint.Diagnostic
	for _, v := range tablefmt.Violations(f.Lines, skipLines, r.config()) {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     v.StartLine,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  v.Message,
		})
	}
	diags = append(diags, structureDiagnostics(f, r.style(), r.ID(), r.Name())...)
	// Format diagnostics anchor at each table's start line; structure
	// diagnostics anchor at the offending row. With multiple tables in
	// one file the two streams interleave by line, so a final sort
	// puts the combined slice in source order — what fixture tests
	// and editors expect.
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Line != diags[j].Line {
			return diags[i].Line < diags[j].Line
		}
		return diags[i].Column < diags[j].Column
	})
	return diags
}

// Fix implements rule.FixableRule. The structure pass runs first
// (edge normalization for MD055, blank-line insertion for MD058) so
// the alignment pass then sees the structurally-normalized bytes and
// canonicalizes the remaining bordered tables. GeneratedRanges are
// recomputed on the reparsed buffer: a blank line inserted before a
// downstream generated section shifts that section's line numbers,
// so copying the pre-fix ranges would point the alignment pass's
// skip set at the wrong lines. tablefmt is CRLF-aware, so no
// post-pass normalisation is needed for mixed-ending output.
func (r *Rule) Fix(f *lint.File) []byte {
	intermediate := applyStructureFix(f, r.style())
	parsed, _ := lint.NewFile(f.Path, intermediate) // NewFile never errors today
	parsed.GeneratedRanges = gensection.FindAllGeneratedRanges(parsed)
	skipLines := formatSkipLines(parsed)
	return tablefmt.FormatLines(parsed.Source, parsed.Lines, skipLines, r.config())
}

func (r *Rule) config() tablefmt.Config {
	return tablefmt.Config{Pad: r.Pad, SeparatorStyle: r.SeparatorStyle}
}

// formatSkipLines returns the line numbers the alignment pass must
// ignore: fenced/indented code, processing-instruction blocks, and
// generated-section bodies. When there are no PI blocks or generated
// ranges, the cached code-block map is returned directly — tablefmt
// only reads the skip set, so handing back the cache avoids a
// per-Check allocation on the hot path. The merged map is built only
// when one of the other inputs is non-empty.
func formatSkipLines(f *lint.File) map[int]struct{} {
	code := lint.CollectCodeBlockLines(f)
	pi := lint.CollectPIBlockLines(f)
	gen := f.GeneratedRanges
	if len(pi) == 0 && len(gen) == 0 {
		return code
	}
	skip := make(map[int]struct{}, len(code)+len(pi))
	for n := range code {
		skip[n] = struct{}{}
	}
	for n := range pi {
		skip[n] = struct{}{}
	}
	for _, gr := range gen {
		for n := gr.From; n <= gr.To; n++ {
			skip[n] = struct{}{}
		}
	}
	return skip
}

// style returns the configured pipe style, defaulting to consistent
// so a Rule literal without an explicit Style (legacy callers and
// tests) keeps working.
func (r *Rule) style() string {
	if r.Style == "" {
		return StyleConsistent
	}
	return r.Style
}

var (
	_ rule.FixableRule  = (*Rule)(nil)
	_ rule.Configurable = (*Rule)(nil)
)
