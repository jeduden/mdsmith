package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/jeduden/mdsmith/pkg/markdown"
)

// MessagingKind is the kind name registered in .mdsmith.yml that
// drives `mdsmith extract`.
const MessagingKind = "messaging"

// MessagingSourceFile is the canonical brand source path,
// relative to the repo root.
const MessagingSourceFile = "docs/brand/messaging.md"

// Messaging is the flat, in-process Go value every patcher
// consumes. The JSON shape `mdsmith extract messaging`
// produces (frontmatter scalars + a code-block headline
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
// kind's schema: `title`, `summary`, and the headline triple
// live under `frontmatter`; the prose values
// (`eyebrow`, `lead`, `tagline`, and the five per-surface
// descriptions) live under their own top-level objects keyed
// by the section's bind/slug, each carrying a `text` field —
// the projection rule for a paragraph under an H2. The mdsmith
// binary is invoked via `go run ./cmd/mdsmith` so the same
// source tree drives the linter and the release tooling.
// Every documented field must be non-empty; a missing or blank
// field is a hard error.
func LoadMessaging(root string) (*Messaging, error) {
	out, err := messagingExtractor(root)
	if err != nil {
		return nil, err
	}
	var doc messagingDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("decode messaging json: %w", err)
	}
	// An empty headline.code is left for Validate to surface as
	// the standard missing-field message; a non-empty code that
	// does not match the canonical shape is a hard headline
	// error that points at the parse problem directly.
	var pre, em, post string
	if strings.TrimSpace(doc.Headline.Code) != "" {
		p, e, s, perr := parseHeadlineEmphasis(doc.Headline.Code)
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

// parseHeadlineEmphasis walks the headline source as Markdown
// via pkg/markdown.Parse (the same goldmark parser the linter
// uses) and splits it on its single Level-1 emphasis span.
// Returns pre, em, post for the website hero template
// `<h1>{pre}<em>{em}</em>{post}</h1>`. Errors if the source
// has zero or more than one Level-1 emphasis span — the hero
// template can render only one — or if the parse produces
// anything other than a single Paragraph.
//
// The release tool does no Markdown parsing itself: the AST is
// the projection mdsmith owns; this function only walks it.
func parseHeadlineEmphasis(src string) (pre, em, post string, err error) {
	doc := markdown.Parse([]byte(strings.TrimSpace(src)))
	body := doc.Body
	root := doc.AST
	// The headline source is a single line of inline content;
	// the parser wraps it in a single Paragraph.
	first := root.FirstChild()
	if first == nil || first.NextSibling() != nil ||
		first.Kind() != ast.KindParagraph {
		return "", "", "", fmt.Errorf(
			"expected a single paragraph, got %q", src)
	}
	// Walk the paragraph's inline children once. Accumulate text
	// before / inside / after the Emphasis node; reject when the
	// shape doesn't match.
	var preBuf, emBuf, postBuf bytes.Buffer
	emCount := 0
	for child := first.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			seg := n.Segment.Value(body)
			if emCount == 0 {
				preBuf.Write(seg)
			} else {
				postBuf.Write(seg)
			}
		case *ast.Emphasis:
			if n.Level != 1 {
				return "", "", "", fmt.Errorf(
					"headline emphasis must use single `*…*` (em), not double `**`")
			}
			emCount++
			if emCount > 1 {
				return "", "", "", fmt.Errorf(
					"expected exactly one `*…*` emphasis span, got more")
			}
			for t := n.FirstChild(); t != nil; t = t.NextSibling() {
				tn, ok := t.(*ast.Text)
				if !ok {
					return "", "", "", fmt.Errorf(
						"headline emphasis must contain plain text only")
				}
				emBuf.Write(tn.Segment.Value(body))
			}
		default:
			return "", "", "", fmt.Errorf(
				"unsupported inline node in headline: %T", child)
		}
	}
	if emCount == 0 {
		return "", "", "", fmt.Errorf(
			"expected exactly one `*…*` emphasis span, got none")
	}
	if emBuf.Len() == 0 {
		return "", "", "", fmt.Errorf("emphasis span is empty")
	}
	return preBuf.String(), emBuf.String(), postBuf.String(), nil
}

// messagingDoc mirrors the shape `mdsmith extract messaging`
// emits. The body-section fields land at the document root
// (not under `frontmatter`); each carries a `text` field that
// holds the paragraph the H2 section contains.
type messagingDoc struct {
	Frontmatter struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	} `json:"frontmatter"`
	Headline                    sectionCode `json:"headline"`
	Eyebrow                     sectionText `json:"eyebrow"`
	Lead                        sectionText `json:"lead"`
	Tagline                     sectionText `json:"tagline"`
	VSCodeDescription           sectionText `json:"vscode-description"`
	VSCodeOverview              sectionText `json:"vscode-overview"`
	ClaudeCodeLSPDescription    sectionText `json:"claude-code-lsp-description"`
	ClaudeCodeSkillsDescription sectionText `json:"claude-code-skills-description"`
	ClaudeCodeAuditDescription  sectionText `json:"claude-code-audit-description"`
}

type sectionText struct {
	Text string `json:"text"`
}

type sectionCode struct {
	Code string `json:"code"`
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
