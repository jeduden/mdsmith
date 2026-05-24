package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// MessagingKind is the kind name registered in .mdsmith.yml that
// drives `mdsmith extract`.
const MessagingKind = "messaging"

// MessagingSourceFile is the canonical brand source path,
// relative to the repo root.
const MessagingSourceFile = "docs/brand/messaging.md"

// Messaging holds every field projected from the messaging kind.
// Field names use Go conventions; the JSON tags pin the
// dash-cased keys produced by `mdsmith extract`. Adding a field
// here is a coordinated edit with .mdsmith.yml's messaging kind,
// docs/brand/messaging.md, and the e2e_extract_messaging_test.go
// expected-fields list.
type Messaging struct {
	Title                       string `json:"title"`
	Summary                     string `json:"summary"`
	Eyebrow                     string `json:"eyebrow"`
	HeadlinePre                 string `json:"headline-pre"`
	HeadlineEm                  string `json:"headline-em"`
	HeadlinePost                string `json:"headline-post"`
	Lead                        string `json:"lead"`
	Tagline                     string `json:"tagline"`
	VSCodeDescription           string `json:"vscode-description"`
	ClaudeCodeLSPDescription    string `json:"claude-code-lsp-description"`
	ClaudeCodeSkillsDescription string `json:"claude-code-skills-description"`
	ClaudeCodeAuditDescription  string `json:"claude-code-audit-description"`
}

// LoadMessaging projects MessagingSourceFile through
// `mdsmith extract <MessagingKind> --format json` and decodes
// it into a typed Messaging value. The JSON shape mirrors the
// kind's schema: `title`, `summary`, and the headline triple
// live under `frontmatter`; the prose values
// (`eyebrow`, `lead`, `tagline`, and the four per-surface
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
		ClaudeCodeLSPDescription:    doc.ClaudeCodeLSPDescription.Text,
		ClaudeCodeSkillsDescription: doc.ClaudeCodeSkillsDescription.Text,
		ClaudeCodeAuditDescription:  doc.ClaudeCodeAuditDescription.Text,
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// headlineEmphasisRE matches the canonical headline shape: any
// leading text, exactly one single-asterisk emphasis span, then
// any trailing text. The span uses `*` (the Markdown `<em>`
// convention) so the rendered HTML matches the hero template's
// `<em>` wrapping.
var headlineEmphasisRE = regexp.MustCompile(`^([^*]*)\*([^*]+)\*([^*]*)$`)

// parseHeadlineEmphasis splits the headline source on its single
// emphasis span. Returns pre, em, post for the website hero
// template (`<h1>{pre}<em>{em}</em>{post}</h1>`). Errors if the
// source has zero or more than one emphasis span — the hero
// template can only render one.
func parseHeadlineEmphasis(src string) (pre, em, post string, err error) {
	src = strings.TrimSpace(src)
	m := headlineEmphasisRE.FindStringSubmatch(src)
	if m == nil {
		return "", "", "", fmt.Errorf(
			"expected exactly one `*…*` emphasis span, got %q", src)
	}
	return m[1], m[2], m[3], nil
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
