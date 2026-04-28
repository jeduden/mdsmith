package emphasisstyle

import (
	"fmt"
	"sort"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule implements MDS042, enforcing consistent delimiter characters for bold
// and italic emphasis and optionally forbidding cross-delimiter nesting.
type Rule struct {
	Bold               string // "asterisk" | "underscore" | "" (not configured)
	Italic             string // "asterisk" | "underscore" | "" (not configured)
	ForbidMixedNesting bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS042" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "emphasis-style" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// EnabledByDefault implements rule.Defaultable. MDS042 is opt-in.
func (r *Rule) EnabledByDefault() bool { return false }

// wantChar returns the expected delimiter byte for the given emphasis level,
// or 0 if that role is not configured.
func (r *Rule) wantChar(level int) byte {
	var setting string
	if level == 2 {
		setting = r.Bold
	} else {
		setting = r.Italic
	}
	switch setting {
	case "asterisk":
		return '*'
	case "underscore":
		return '_'
	}
	return 0
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if r.Bold == "" && r.Italic == "" && !r.ForbidMixedNesting {
		return nil
	}
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		em, ok := n.(*ast.Emphasis)
		if !ok {
			return ast.WalkContinue, nil
		}
		diags = append(diags, r.checkEmphasis(em, f)...)
		return ast.WalkContinue, nil
	})
	return diags
}

func (r *Rule) checkEmphasis(em *ast.Emphasis, f *lint.File) []lint.Diagnostic {
	delim := emphDelim(em, f.Source)
	if delim == 0 {
		return nil
	}
	var diags []lint.Diagnostic
	if d := r.styleViolation(em, f, delim); d != nil {
		diags = append(diags, *d)
	}
	if d := r.mixedNestingViolation(em, f, delim); d != nil {
		diags = append(diags, *d)
	}
	return diags
}

func (r *Rule) styleViolation(em *ast.Emphasis, f *lint.File, delim byte) *lint.Diagnostic {
	want := r.wantChar(em.Level)
	if want == 0 || delim == want {
		return nil
	}
	role := "italic"
	if em.Level == 2 {
		role = "bold"
	}
	return &lint.Diagnostic{
		File:     f.Path,
		Line:     emphLine(em, f),
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message: fmt.Sprintf(
			"%s uses %s; configured style is %s",
			role, delimName(delim), delimName(want),
		),
	}
}

func (r *Rule) mixedNestingViolation(em *ast.Emphasis, f *lint.File, delim byte) *lint.Diagnostic {
	if !r.ForbidMixedNesting {
		return nil
	}
	parent, ok := em.Parent().(*ast.Emphasis)
	if !ok {
		return nil
	}
	parentDelim := emphDelim(parent, f.Source)
	if parentDelim == 0 || parentDelim == delim {
		return nil
	}
	return &lint.Diagnostic{
		File:     f.Path,
		Line:     emphLine(em, f),
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message: fmt.Sprintf(
			"mixed emphasis delimiters: %s wraps %s",
			delimName(parentDelim), delimName(delim),
		),
	}
}

func emphLine(em *ast.Emphasis, f *lint.File) int {
	pos := emphOpenStart(em, f.Source)
	if pos >= 0 {
		return f.LineOfOffset(pos)
	}
	return 1
}

type replacement struct {
	start, end int
	newText    []byte
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	if r.Bold == "" && r.Italic == "" {
		return f.Source
	}

	var reps []replacement
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		em, ok := n.(*ast.Emphasis)
		if !ok {
			return ast.WalkContinue, nil
		}

		want := r.wantChar(em.Level)
		if want == 0 {
			return ast.WalkContinue, nil
		}

		delim := emphDelim(em, f.Source)
		if delim == 0 || delim == want {
			return ast.WalkContinue, nil
		}

		// Triple-delimiter runs (e.g. ***x*** or ___x___) are ambiguous;
		// skip auto-fix for the outer and any directly nested inner emphasis.
		if isTripleRun(em, f.Source) {
			return ast.WalkContinue, nil
		}
		if parent, ok := em.Parent().(*ast.Emphasis); ok && isTripleRun(parent, f.Source) {
			return ast.WalkContinue, nil
		}

		openStart := emphOpenStart(em, f.Source)
		closeStart := emphCloseStart(em)
		if openStart < 0 || closeStart < 0 {
			return ast.WalkContinue, nil
		}

		newDelim := make([]byte, em.Level)
		for i := range newDelim {
			newDelim[i] = want
		}
		reps = append(reps, replacement{openStart, openStart + em.Level, newDelim})
		reps = append(reps, replacement{closeStart, closeStart + em.Level, newDelim})

		return ast.WalkContinue, nil
	})

	if len(reps) == 0 {
		return f.Source
	}

	sort.Slice(reps, func(i, j int) bool { return reps[i].start < reps[j].start })

	result := make([]byte, len(f.Source))
	copy(result, f.Source)
	for i := len(reps) - 1; i >= 0; i-- {
		rep := reps[i]
		result = append(result[:rep.start], append(rep.newText, result[rep.end:]...)...)
	}
	return result
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "bold":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("emphasis-style: bold must be a string, got %T", v)
			}
			if s != "" && s != "asterisk" && s != "underscore" {
				return fmt.Errorf("emphasis-style: invalid bold %q (valid: asterisk, underscore)", s)
			}
			r.Bold = s
		case "italic":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("emphasis-style: italic must be a string, got %T", v)
			}
			if s != "" && s != "asterisk" && s != "underscore" {
				return fmt.Errorf("emphasis-style: invalid italic %q (valid: asterisk, underscore)", s)
			}
			r.Italic = s
		case "forbid-mixed-nesting":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("emphasis-style: forbid-mixed-nesting must be a bool, got %T", v)
			}
			r.ForbidMixedNesting = b
		default:
			return fmt.Errorf("emphasis-style: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"bold":                 "",
		"italic":               "",
		"forbid-mixed-nesting": false,
	}
}

// emphInfo returns the delimiter byte and open-start index for em.
// It walks down the leftmost emphasis-or-text chain.
// Returns (0, -1) if the delimiter cannot be determined.
func emphInfo(em *ast.Emphasis, source []byte) (delim byte, openStart int) {
	totalLevels := em.Level
	child := em.FirstChild()
	for child != nil {
		switch v := child.(type) {
		case *ast.Text:
			pos := v.Segment.Start - totalLevels
			if pos >= 0 && pos < len(source) {
				return source[pos], pos
			}
			return 0, -1
		case *ast.Emphasis:
			totalLevels += v.Level
			child = v.FirstChild()
		default:
			start := firstTextStart(child)
			if start < 0 {
				return 0, -1
			}
			pos := start - totalLevels
			if pos >= 0 && pos < len(source) {
				return source[pos], pos
			}
			return 0, -1
		}
	}
	return 0, -1
}

func emphDelim(em *ast.Emphasis, source []byte) byte {
	d, _ := emphInfo(em, source)
	return d
}

func emphOpenStart(em *ast.Emphasis, source []byte) int {
	_, s := emphInfo(em, source)
	return s
}

// emphCloseStart returns the byte index where the closing delimiter of em begins.
func emphCloseStart(em *ast.Emphasis) int {
	last := em.LastChild()
	if last == nil {
		return -1
	}
	switch v := last.(type) {
	case *ast.Text:
		return v.Segment.Stop
	case *ast.Emphasis:
		inner := emphCloseStart(v)
		if inner < 0 {
			return -1
		}
		return inner + v.Level
	default:
		return -1
	}
}

// firstTextStart returns the Segment.Start of the first *ast.Text in n's
// subtree, or -1 if none is found.
func firstTextStart(n ast.Node) int {
	result := -1
	_ = ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if t, ok := child.(*ast.Text); ok {
				result = t.Segment.Start
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	return result
}

// isTripleRun reports whether em directly wraps a single inner Emphasis and
// their opening delimiters form a consecutive run of the same character
// (e.g. *** or ___), making byte-level substitution ambiguous.
func isTripleRun(em *ast.Emphasis, source []byte) bool {
	inner, ok := em.FirstChild().(*ast.Emphasis)
	if !ok || em.FirstChild() != em.LastChild() {
		return false
	}
	openStart := emphOpenStart(em, source)
	if openStart < 0 {
		return false
	}
	total := em.Level + inner.Level
	if openStart+total > len(source) {
		return false
	}
	c := source[openStart]
	if c != '*' && c != '_' {
		return false
	}
	for i := 1; i < total; i++ {
		if source[openStart+i] != c {
			return false
		}
	}
	return true
}

func delimName(c byte) string {
	if c == '*' {
		return "asterisk"
	}
	return "underscore"
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.FixableRule  = (*Rule)(nil)
)
