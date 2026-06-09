package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// MessagingKind is the kind name registered in .mdsmith.yml that
// drives `mdsmith extract`.
const MessagingKind = "messaging"

// MessagingSourceFile is the canonical brand source path,
// relative to the repo root.
const MessagingSourceFile = "docs/brand/messaging.md"

// Messaging is the flat, in-process Go value every patcher
// consumes. The JSON shape `mdsmith extract messaging`
// produces (frontmatter scalars + an inline-span headline
// section + paragraph sections for the prose fields) is
// decoded via the internal `messagingDoc` envelope in
// LoadMessaging and copied here; no field on Messaging is
// JSON-decoded directly, so there are no JSON tags.
//
// Adding a field is a coordinated edit with .mdsmith.yml's
// messaging kind, docs/brand/messaging.md, messagingDoc /
// LoadMessaging in this file, and
// cmd/mdsmith/e2e_extract_messaging_test.go's
// expected-fields list.
type Messaging struct {
	Title                       string
	Summary                     string
	Eyebrow                     string
	HeadlinePre                 string
	HeadlineEm                  string
	HeadlinePost                string
	Lead                        string
	Tagline                     string
	VSCodeDescription           string
	VSCodeOverview              string
	ClaudeCodeLSPDescription    string
	ClaudeCodeSkillsDescription string
	ClaudeCodeAuditDescription  string
}

// LoadMessaging projects MessagingSourceFile through
// `mdsmith extract <MessagingKind> --format json` and decodes
// it into a typed Messaging value. The JSON shape mirrors the
// kind's schema: `title` and `summary` live under `frontmatter`;
// the headline lives under a top-level `headline` object whose
// `inline` array is the paragraph's typed inline-span projection
// (plan 212); the prose values (`eyebrow`, `lead`, `tagline`,
// and the five per-surface descriptions) live under their own
// top-level objects keyed by the section's bind/slug, each
// carrying a `text` field — the projection rule for a paragraph
// under an H2. The mdsmith binary is invoked via
// `go run ./cmd/mdsmith` so the same source tree drives the
// linter and the release tooling. Every documented field must be
// non-empty; a missing or blank field is a hard error.
func LoadMessaging(root string) (*Messaging, error) {
	out, err := messagingExtractor(root)
	if err != nil {
		return nil, err
	}
	var doc messagingDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("decode messaging json: %w", err)
	}
	// An empty headline.inline is left for Validate to surface as
	// the standard missing-field message; a non-empty span list
	// that does not match the canonical shape is a hard headline
	// error that points at the problem directly.
	var pre, em, post string
	if len(doc.Headline.Inline) > 0 {
		p, e, s, perr := splitHeadlineSpans(doc.Headline.Inline)
		if perr != nil {
			return nil, fmt.Errorf("headline: %w", perr)
		}
		pre, em, post = p, e, s
	}
	m := Messaging{
		Title:                       doc.Frontmatter.Title,
		Summary:                     doc.Frontmatter.Summary,
		HeadlinePre:                 pre,
		HeadlineEm:                  em,
		HeadlinePost:                post,
		Eyebrow:                     doc.Eyebrow.Text,
		Lead:                        doc.Lead.Text,
		Tagline:                     doc.Tagline.Text,
		VSCodeDescription:           doc.VSCodeDescription.Text,
		VSCodeOverview:              doc.VSCodeOverview.Text,
		ClaudeCodeLSPDescription:    doc.ClaudeCodeLSPDescription.Text,
		ClaudeCodeSkillsDescription: doc.ClaudeCodeSkillsDescription.Text,
		ClaudeCodeAuditDescription:  doc.ClaudeCodeAuditDescription.Text,
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// headlineSpan is one element of the headline paragraph's `inline`
// projection (plan 212). It is the typed shape `mdsmith extract`
// emits; the release tool reads it directly instead of re-parsing
// Markdown. Leaf spans (text, code, autolink) carry Value; container
// spans (emphasis, strong, link) carry Children.
type headlineSpan struct {
	Span     string         `json:"span"`
	Value    string         `json:"value"`
	Level    int            `json:"level"`
	Children []headlineSpan `json:"children"`
}

// splitHeadlineSpans walks the top-level inline spans of the headline
// paragraph and splits them on the single emphasis span. It returns
// pre, em, post for the website hero template
// `<h1>{pre}<em>{em}</em>{post}</h1>`.
//
// The release tool does no Markdown parsing: the inline projection is
// what mdsmith owns; this function only walks the typed data. It
// errors when the spans have zero or more than one top-level
// `emphasis` span (the hero template renders exactly one <em>), when
// the single span is `strong` rather than `emphasis` (double `**`
// means <strong>, not <em>), when a non-text span sits at the top
// level, or when the emphasis span's children are not all text.
func splitHeadlineSpans(spans []headlineSpan) (pre, em, post string, err error) {
	var preB, emB, postB strings.Builder
	emCount := 0
	for _, s := range spans {
		switch s.Span {
		case "text":
			if emCount == 0 {
				preB.WriteString(s.Value)
			} else {
				postB.WriteString(s.Value)
			}
		case "break":
			// A reflowed headline projects a soft/hard line break
			// between text runs; render it as a space (matching the
			// plain-text extractor) so wrapping the source line does
			// not fail sync-messaging.
			if emCount == 0 {
				preB.WriteByte(' ')
			} else {
				postB.WriteByte(' ')
			}
		case "emphasis":
			emCount++
			if emCount > 1 {
				return "", "", "", errors.New(
					"expected exactly one `*…*` emphasis span, got more")
			}
			flat, ferr := flattenTextChildren(s.Children)
			if ferr != nil {
				return "", "", "", ferr
			}
			emB.WriteString(flat)
		case "strong":
			return "", "", "", errors.New(
				"headline emphasis must use single `*…*` (em), not " +
					"a `strong` span (double `**`)")
		default:
			return "", "", "", fmt.Errorf(
				"unsupported inline span in headline: %q", s.Span)
		}
	}
	if emCount == 0 {
		return "", "", "", errors.New(
			"expected exactly one `*…*` emphasis span, got none")
	}
	if emB.Len() == 0 {
		return "", "", "", errors.New("emphasis span is empty")
	}
	return preB.String(), emB.String(), postB.String(), nil
}

// flattenTextChildren concatenates a span's children, requiring every
// child to be a text leaf. The hero template's <em> renders inline
// text only, so a nested emphasis, code, or link inside the headline
// emphasis is rejected.
func flattenTextChildren(children []headlineSpan) (string, error) {
	var b strings.Builder
	for _, c := range children {
		switch c.Span {
		case "text":
			b.WriteString(c.Value)
		case "break":
			// A break inside the emphasized run (the emphasis wrapped
			// across source lines) renders as a space, same as the
			// top-level handling.
			b.WriteByte(' ')
		default:
			return "", errors.New(
				"headline emphasis must contain plain text only")
		}
	}
	return b.String(), nil
}

// messagingDoc mirrors the shape `mdsmith extract messaging`
// emits. The body-section fields land at the document root
// (not under `frontmatter`); each prose section carries a `text`
// field, while the headline carries an `inline` span list.
type messagingDoc struct {
	Frontmatter struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	} `json:"frontmatter"`
	Headline                    sectionInline `json:"headline"`
	Eyebrow                     sectionText   `json:"eyebrow"`
	Lead                        sectionText   `json:"lead"`
	Tagline                     sectionText   `json:"tagline"`
	VSCodeDescription           sectionText   `json:"vscode-description"`
	VSCodeOverview              sectionText   `json:"vscode-overview"`
	ClaudeCodeLSPDescription    sectionText   `json:"claude-code-lsp-description"`
	ClaudeCodeSkillsDescription sectionText   `json:"claude-code-skills-description"`
	ClaudeCodeAuditDescription  sectionText   `json:"claude-code-audit-description"`
}

type sectionText struct {
	Text string `json:"text"`
}

type sectionInline struct {
	Inline []headlineSpan `json:"inline"`
}

// Validate fails fast if any required field is empty. The
// .mdsmith.yml schema's `nonEmpty` constraint catches the same
// condition under `mdsmith check`, but a defensive check here
// keeps sync-messaging self-contained.
func (m *Messaging) Validate() error {
	pairs := []struct {
		name, value string
	}{
		{"title", m.Title},
		{"summary", m.Summary},
		{"eyebrow", m.Eyebrow},
		{"headline-pre", m.HeadlinePre},
		{"headline-em", m.HeadlineEm},
		{"headline-post", m.HeadlinePost},
		{"lead", m.Lead},
		{"tagline", m.Tagline},
		{"vscode-description", m.VSCodeDescription},
		{"vscode-overview", m.VSCodeOverview},
		{"claude-code-lsp-description", m.ClaudeCodeLSPDescription},
		{"claude-code-skills-description", m.ClaudeCodeSkillsDescription},
		{"claude-code-audit-description", m.ClaudeCodeAuditDescription},
	}
	var missing []string
	for _, p := range pairs {
		if strings.TrimSpace(p.value) == "" {
			missing = append(missing, p.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("messaging: empty field(s): %s",
			strings.Join(missing, ", "))
	}
	return nil
}

// messagingExtractor is a package-level seam so tests stub the
// shell-out without driving the real mdsmith binary. Production
// runs `go run ./cmd/mdsmith extract` with cwd=root and captures
// stdout.
var messagingExtractor = runMdsmithExtract

func runMdsmithExtract(root string) ([]byte, error) {
	cmd := exec.Command("go", "run", "./cmd/mdsmith", //nolint:gosec // CI-only invocation, args constant
		"extract", MessagingKind, MessagingSourceFile,
		"--format", "json")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf(
				"mdsmith extract messaging: %w (stderr: %s)",
				err, ee.Stderr)
		}
		return nil, fmt.Errorf("mdsmith extract messaging: %w", err)
	}
	return out, nil
}
