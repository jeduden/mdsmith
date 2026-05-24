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
	m := Messaging{
		Title:                       doc.Frontmatter.Title,
		Summary:                     doc.Frontmatter.Summary,
		HeadlinePre:                 doc.Frontmatter.HeadlinePre,
		HeadlineEm:                  doc.Frontmatter.HeadlineEm,
		HeadlinePost:                doc.Frontmatter.HeadlinePost,
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

// messagingDoc mirrors the shape `mdsmith extract messaging`
// emits. The body-section fields land at the document root
// (not under `frontmatter`); each carries a `text` field that
// holds the paragraph the H2 section contains.
type messagingDoc struct {
	Frontmatter struct {
		Title        string `json:"title"`
		Summary      string `json:"summary"`
		HeadlinePre  string `json:"headline-pre"`
		HeadlineEm   string `json:"headline-em"`
		HeadlinePost string `json:"headline-post"`
	} `json:"frontmatter"`
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
