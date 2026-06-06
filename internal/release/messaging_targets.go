package release

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// MessagingTarget pairs a tracked file with the patcher that
// rewrites one field in it and a function that pulls the
// canonical value from a loaded Messaging.
type MessagingTarget struct {
	// Label is the human-readable name used in summary output
	// and drift diagnostics.
	Label string
	// Path is the on-disk path to the tracked file, joined
	// against the repo root by MessagingTargets at
	// construction time. Drift output and error messages
	// print the joined path so the caller's working directory
	// is visible.
	Path string
	// Patcher reads / rewrites the tracked field in Path.
	Patcher Patcher
	// ValueOf extracts the canonical value for this target from
	// a loaded Messaging.
	ValueOf func(*Messaging) string
}

// MessagingTargets returns every tracked surface in a stable
// order. The order is the output order for `sync-messaging`
// summaries and the drift-check diagnostics, so it has to stay
// deterministic.
//
// Fragments come first because the READMEs <?include?> them;
// running the apply path top-down guarantees the source of any
// included content is up to date before the consuming file is
// re-read.
func MessagingTargets(root string) []MessagingTarget {
	out := make([]MessagingTarget, 0, 16)
	out = append(out, messagingFragmentTargets(root)...)
	out = append(out, messagingWebsiteTargets(root)...)
	out = append(out, messagingPackageTargets(root)...)
	out = append(out, messagingEditorTargets(root)...)
	return out
}

func messagingPath(root string, parts ...string) string {
	return filepath.Join(append([]string{root}, parts...)...)
}

// messagingFragmentTargets returns the generated Markdown
// fragments. READMEs <?include?> them, so they live first in
// the apply order.
func messagingFragmentTargets(root string) []MessagingTarget {
	return []MessagingTarget{
		{
			Label:   "tagline fragment",
			Path:    messagingPath(root, "docs", "brand", "fragments", "tagline.fragment.md"),
			Patcher: MarkdownFragment{},
			ValueOf: func(m *Messaging) string { return m.Tagline },
		},
		{
			Label:   "lead fragment",
			Path:    messagingPath(root, "docs", "brand", "fragments", "lead.fragment.md"),
			Patcher: MarkdownFragment{},
			ValueOf: func(m *Messaging) string { return m.Lead },
		},
		{
			Label:   "vscode overview fragment",
			Path:    messagingPath(root, "docs", "brand", "fragments", "vscode-overview.fragment.md"),
			Patcher: MarkdownFragment{},
			ValueOf: func(m *Messaging) string { return m.VSCodeOverview },
		},
		{
			Label:   "headline fragment",
			Path:    messagingPath(root, "docs", "brand", "fragments", "headline.fragment.md"),
			Patcher: MarkdownFragment{},
			ValueOf: func(m *Messaging) string { return m.Headline() },
		},
		{
			Label:   "eyebrow fragment",
			Path:    messagingPath(root, "docs", "brand", "fragments", "eyebrow.fragment.md"),
			Patcher: MarkdownFragment{},
			ValueOf: func(m *Messaging) string { return m.Eyebrow },
		},
	}
}

// messagingWebsiteTargets returns the Hugo site surfaces: the
// hugo.toml [params].description and the home page hero
// frontmatter (summary + 5 hero subfields).
func messagingWebsiteTargets(root string) []MessagingTarget {
	indexMD := messagingPath(root, "website", "content", "_index.md")
	return []MessagingTarget{
		{
			Label:   "hugo.toml [params].description",
			Path:    messagingPath(root, "website", "hugo.toml"),
			Patcher: TOMLStringField{Table: []string{"params"}, Key: "description"},
			ValueOf: func(m *Messaging) string { return m.Tagline },
		},
		{
			Label:   "_index.md summary",
			Path:    indexMD,
			Patcher: YAMLFrontmatterField{Path: []string{"summary"}},
			ValueOf: func(m *Messaging) string { return m.Tagline },
		},
		{
			Label:   "_index.md hero.eyebrow",
			Path:    indexMD,
			Patcher: YAMLFrontmatterField{Path: []string{"hero", "eyebrow"}},
			ValueOf: func(m *Messaging) string { return m.Eyebrow },
		},
		{
			Label:   "_index.md hero.headline_pre",
			Path:    indexMD,
			Patcher: YAMLFrontmatterField{Path: []string{"hero", "headline_pre"}},
			ValueOf: func(m *Messaging) string { return m.HeadlinePre },
		},
		{
			Label:   "_index.md hero.headline_em",
			Path:    indexMD,
			Patcher: YAMLFrontmatterField{Path: []string{"hero", "headline_em"}},
			ValueOf: func(m *Messaging) string { return m.HeadlineEm },
		},
		{
			Label:   "_index.md hero.headline_post",
			Path:    indexMD,
			Patcher: YAMLFrontmatterField{Path: []string{"hero", "headline_post"}},
			ValueOf: func(m *Messaging) string { return m.HeadlinePost },
		},
		{
			Label:   "_index.md hero.lead",
			Path:    indexMD,
			Patcher: YAMLFrontmatterField{Path: []string{"hero", "lead"}},
			ValueOf: func(m *Messaging) string { return m.Lead },
		},
	}
}

// messagingPackageTargets returns the language-ecosystem
// manifests: npm and PyPI.
func messagingPackageTargets(root string) []MessagingTarget {
	return []MessagingTarget{
		{
			Label:   "npm/mdsmith/package.json description",
			Path:    messagingPath(root, "npm", "mdsmith", "package.json"),
			Patcher: JSONStringField{Key: "description"},
			ValueOf: func(m *Messaging) string { return m.Tagline },
		},
		{
			Label:   "pyproject.toml [project].description",
			Path:    messagingPath(root, "python", "pyproject.toml"),
			Patcher: TOMLStringField{Table: []string{"project"}, Key: "description"},
			ValueOf: func(m *Messaging) string { return m.Tagline },
		},
	}
}

// messagingEditorTargets returns the editor manifests: the
// VS Code extension and the Claude Code plugins that carry
// product framing.
func messagingEditorTargets(root string) []MessagingTarget {
	return []MessagingTarget{
		{
			Label:   "vscode/package.json description",
			Path:    messagingPath(root, "editors", "vscode", "package.json"),
			Patcher: JSONStringField{Key: "description"},
			ValueOf: func(m *Messaging) string { return m.VSCodeDescription },
		},
		{
			Label:   "claude-code plugin.json description",
			Path:    messagingPath(root, "editors", "claude-code", ".claude-plugin", "plugin.json"),
			Patcher: JSONStringField{Key: "description"},
			ValueOf: func(m *Messaging) string { return m.ClaudeCodeLSPDescription },
		},
		{
			Label:   "claude-code-skills plugin.json description",
			Path:    messagingPath(root, "editors", "claude-code-skills", ".claude-plugin", "plugin.json"),
			Patcher: JSONStringField{Key: "description"},
			ValueOf: func(m *Messaging) string { return m.ClaudeCodeSkillsDescription },
		},
		{
			Label:   "claude-code-audit plugin.json description",
			Path:    messagingPath(root, "editors", "claude-code-audit", ".claude-plugin", "plugin.json"),
			Patcher: JSONStringField{Key: "description"},
			ValueOf: func(m *Messaging) string { return m.ClaudeCodeAuditDescription },
		},
	}
}

// ApplyResult records the outcome of one target's apply call.
type ApplyResult struct {
	Target  MessagingTarget
	Changed bool
}

// ApplyMessaging patches every target with its canonical value
// from m. Only generated-fragment targets (MarkdownFragment
// patchers) are created when missing — the on-first-run
// behavior. Every other target's file must exist; a missing
// non-fragment surface is a hard "required file missing"
// error. Idempotent: rerunning produces no further writes.
func (t *Toolkit) ApplyMessaging(root string, m *Messaging) ([]ApplyResult, error) {
	results := make([]ApplyResult, 0, len(MessagingTargets(root)))
	for _, tg := range MessagingTargets(root) {
		r, err := t.applyTarget(tg, m)
		if err != nil {
			return results, fmt.Errorf("%s: %w", tg.Label, err)
		}
		results = append(results, r)
	}
	return results, nil
}

// ApplyMessaging is the package-level entry point used by the
// cmd binary.
func ApplyMessaging(root string, m *Messaging) ([]ApplyResult, error) {
	return New().ApplyMessaging(root, m)
}

func (t *Toolkit) applyTarget(tg MessagingTarget, m *Messaging) (ApplyResult, error) {
	want := tg.ValueOf(m)
	body, err := t.fs.ReadFile(tg.Path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return ApplyResult{Target: tg}, fmt.Errorf("read %s: %w", tg.Path, err)
		}
		// Missing file is fatal for everything except generated
		// fragments — those are created here on first run.
		if _, isFragment := tg.Patcher.(MarkdownFragment); !isFragment {
			return ApplyResult{Target: tg}, fmt.Errorf("required file missing: %s", tg.Path)
		}
		body = nil
	}
	out, err := tg.Patcher.PatchValue(body, want)
	if err != nil {
		return ApplyResult{Target: tg}, err
	}
	if bytes.Equal(out, body) {
		return ApplyResult{Target: tg, Changed: false}, nil
	}
	if err := t.fs.MkdirAll(filepath.Dir(tg.Path), 0o755); err != nil {
		return ApplyResult{Target: tg}, fmt.Errorf("mkdir %s: %w", tg.Path, err)
	}
	if err := t.fs.WriteFile(tg.Path, out, 0o644); err != nil {
		return ApplyResult{Target: tg}, fmt.Errorf("write %s: %w", tg.Path, err)
	}
	return ApplyResult{Target: tg, Changed: true}, nil
}

// MessagingDrift describes one target whose on-disk value
// disagrees with the source.
type MessagingDrift struct {
	Target MessagingTarget
	Have   string
	Want   string
}

// CheckMessaging compares every target's on-disk value to the
// source Messaging. Returns drift entries in MessagingTargets'
// order (the same apply-path order ApplyMessaging walks). An
// empty list means the tree is clean.
func (t *Toolkit) CheckMessaging(root string, m *Messaging) ([]MessagingDrift, error) {
	var drifts []MessagingDrift
	for _, tg := range MessagingTargets(root) {
		body, err := t.fs.ReadFile(tg.Path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				drifts = append(drifts, MessagingDrift{
					Target: tg,
					Have:   "<missing>",
					Want:   tg.ValueOf(m),
				})
				continue
			}
			return drifts, fmt.Errorf("%s: read %s: %w", tg.Label, tg.Path, err)
		}
		have, err := tg.Patcher.ReadValue(body)
		if err != nil {
			return drifts, fmt.Errorf("%s: %w", tg.Label, err)
		}
		want := tg.ValueOf(m)
		if have != want {
			drifts = append(drifts, MessagingDrift{
				Target: tg, Have: have, Want: want,
			})
		}
	}
	return drifts, nil
}

// CheckMessaging is the package-level entry point.
func CheckMessaging(root string, m *Messaging) ([]MessagingDrift, error) {
	return New().CheckMessaging(root, m)
}

// FormatDrift renders drifts as a multi-line diff-style report
// suitable for stderr or PR comments.
func FormatDrift(drifts []MessagingDrift) string {
	if len(drifts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("messaging drift detected:\n")
	for _, d := range drifts {
		fmt.Fprintf(&b, "  %s (%s)\n", d.Target.Label, d.Target.Path)
		fmt.Fprintf(&b, "    have: %s\n", oneLineForDrift(d.Have))
		fmt.Fprintf(&b, "    want: %s\n", oneLineForDrift(d.Want))
	}
	b.WriteString("run `mdsmith-release sync-messaging` to update.\n")
	return b.String()
}

// oneLineForDrift collapses newlines (LF and CRLF) and
// truncates the value to roughly 120 columns for the drift
// report. Truncation is rune-aware so multi-byte runes (the
// messaging copy uses em-dashes) cannot be sliced in half.
func oneLineForDrift(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	const maxRunes = 117
	runes := []rune(s)
	if len(runes) > maxRunes+3 {
		return string(runes[:maxRunes]) + "..."
	}
	return s
}
