package descriptivelinktext

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/internal/setutil"
	"github.com/yuin/goldmark/ast"
)

var defaultBanned = []string{"click here", "here", "link", "more"}

func init() {
	rule.Register(&Rule{Banned: append([]string(nil), defaultBanned...)})
}

// Rule flags links whose visible text is a non-descriptive phrase such as
// "click here", "here", "link", or "more".
//
// The lookup form of Banned is memoised on the rule instance behind
// `bannedSetPtr` + `bannedSetMu` (a double-checked-lock pattern, not
// sync.Once: ApplySettings is allowed to swap Banned and the cache
// must follow). Rule instances are shared across concurrent LSP
// calls — cmd/mdsmith/lsp.go reuses rule.All() and ConfigureRule
// does not clone when cfg.Settings is nil — but every concurrent
// reader during a Check sees the same set; ApplySettings runs only
// during config load, before any Check, so the swap path never
// races a reader. Moving the cache off the per-Check *lint.File
// memo is plan 195 task 9's MDS063 fix: the build (4 normalised
// banned strings + map setup) was paying ~13 allocs per Check on
// the alloc-budget gate fixture; the per-rule cache pays them
// once per rule instance.
type Rule struct {
	Banned []string

	bannedSetPtr atomic.Pointer[map[string]struct{}]
	bannedSetMu  sync.Mutex
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS063" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "descriptive-link-text" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable. MDS063 is opt-in.
func (r *Rule) EnabledByDefault() bool { return false }

// ApplySettings implements rule.Configurable.
// banned replaces (not appends to) the default phrase list.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "banned":
			ss, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("descriptive-link-text: banned must be a list of strings, got %T", v)
			}
			r.Banned = ss
		default:
			return fmt.Errorf("descriptive-link-text: unknown setting %q", k)
		}
	}
	// Invalidate the lookup cache so the next Check rebuilds against
	// the new Banned slice; ApplySettings is the only path that
	// mutates r.Banned, so clearing here is sufficient.
	r.bannedSetPtr.Store(nil)
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"banned": append([]string(nil), defaultBanned...),
	}
}

// Check implements rule.Rule. The per-link logic is pure and
// stateless, so it is expressed as CheckNode and the engine can fold
// this rule into one shared AST walk; a direct call still works via
// rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	if len(r.Banned) == 0 {
		return nil
	}
	link, ok := n.(*ast.Link)
	if !ok {
		return nil
	}
	if isOnlyImageChild(link) || isOnlyCodeSpanChild(link) {
		return nil
	}

	text := collectLinkText(link, f.Source)
	if !setutil.Contains(r.cachedBannedSet(), normalizeText(text)) {
		return nil
	}
	line := linkLine(link, f)
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("link text %q is not descriptive", text),
	}}
}

// cachedBannedSet returns the lookup form of r.Banned, memoised on
// the rule instance behind an atomic pointer guarded by a mutex.
// The warm path is a single atomic load and serves every Check
// after the cache is populated. The cold path serialises on the
// mutex so concurrent first-callers see one another's build
// instead of multiple racing builds; on the extremely narrow
// window where two goroutines see the pointer nil before either
// acquires the mutex, the second one will rebuild the same
// 4-entry map after the first releases (a 13-alloc one-shot
// cost, vastly cheaper than the test-only hook a deterministic
// double-checked-lock would require). Living on the rule (one
// set per configured-banned-list) collapses what the previous
// per-File memo paid every Check (build the map + normalise
// 4 strings) to "once per rule instance for the program's
// lifetime, plus at most one redundant build on the race".
func (r *Rule) cachedBannedSet() map[string]struct{} {
	if p := r.bannedSetPtr.Load(); p != nil {
		return *p
	}
	r.bannedSetMu.Lock()
	defer r.bannedSetMu.Unlock()
	m := make(map[string]struct{}, len(r.Banned))
	for _, b := range r.Banned {
		m[normalizeText(b)] = struct{}{}
	}
	r.bannedSetPtr.Store(&m)
	return m
}

// normalizeText trims, lowercases, and collapses internal whitespace.
// Single-pass to avoid the three intermediate allocations of the
// strings.Fields → strings.Join → strings.ToLower chain.
func normalizeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	needSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if b.Len() > 0 {
				needSpace = true
			}
		} else {
			if needSpace {
				b.WriteByte(' ')
				needSpace = false
			}
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// isOnlyImageChild reports whether link's sole child is an image node.
func isOnlyImageChild(link *ast.Link) bool {
	c := link.FirstChild()
	return c != nil && c.NextSibling() == nil && c.Kind() == ast.KindImage
}

// isOnlyCodeSpanChild reports whether link's sole child is a code span.
func isOnlyCodeSpanChild(link *ast.Link) bool {
	c := link.FirstChild()
	return c != nil && c.NextSibling() == nil && c.Kind() == ast.KindCodeSpan
}

// collectLinkText returns all plain text within the link node, including
// text nested inside emphasis or other inline formatting.
func collectLinkText(n ast.Node, source []byte) string {
	var b strings.Builder
	collectText(&b, n, source)
	return b.String()
}

func collectText(b *strings.Builder, n ast.Node, source []byte) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		} else {
			collectText(b, c, source)
		}
	}
}

// linkLine returns the 1-based source line of the first text node inside
// the link, falling back to 1 if none exists.
func linkLine(link *ast.Link, f *lint.File) int {
	line := 1
	_ = ast.Walk(link, func(n ast.Node, _ bool) (ast.WalkStatus, error) {
		t, ok := n.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		line = f.LineOfOffset(t.Segment.Start)
		return ast.WalkStop, nil
	})
	return line
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.NodeChecker  = (*Rule)(nil)
)
