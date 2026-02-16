package corpus

import (
	"path/filepath"
	"strings"
)

// TaxonomyEntry documents category definitions and boundary rules.
type TaxonomyEntry struct {
	Category        Category
	Name            string
	Definition      string
	BoundaryRule    string
	PositiveExample string
	NegativeExample string
}

var taxonomyEntries = []TaxonomyEntry{
	{
		Category:        CategoryAgentControl,
		Name:            "Agent Control Docs",
		Definition:      "Instructions that constrain agent behavior or workflow.",
		BoundaryRule:    "Contains direct agent directives; not end-user product docs.",
		PositiveExample: "AGENTS.md with tool and escalation rules.",
		NegativeExample: "README that explains project setup.",
	},
	{
		Category:        CategoryTutorial,
		Name:            "Tutorial",
		Definition:      "Guided, end-to-end learning material with a clear sequence.",
		BoundaryRule:    "Teaches via ordered steps and outcomes, not quick lookup.",
		PositiveExample: "tutorial/getting-started.md with progressive exercises.",
		NegativeExample: "Reference table of flags.",
	},
	{
		Category:        CategoryHowTo,
		Name:            "How-To Guide",
		Definition:      "Task-focused instructions for a specific operational goal.",
		BoundaryRule:    "Optimized for completion, not conceptual teaching.",
		PositiveExample: "docs/how-to/rotate-keys.md.",
		NegativeExample: "Background explainer on key management theory.",
	},
	{
		Category:        CategoryReference,
		Name:            "Reference",
		Definition:      "Lookup material such as rule catalogs, definitions, and schemas.",
		BoundaryRule:    "Answerable by scanning for facts, not narrative flow.",
		PositiveExample: "rules/MDS001-line-length/README.md.",
		NegativeExample: "Design memo comparing alternatives.",
	},
	{
		Category:        CategoryExplanation,
		Name:            "Explanation / Background",
		Definition:      "Context documents that explain why choices were made.",
		BoundaryRule:    "Emphasizes rationale and mental model over instructions.",
		PositiveExample: "guides/metrics-tradeoffs.md.",
		NegativeExample: "Runbook incident response checklist.",
	},
	{
		Category:        CategoryADR,
		Name:            "Architecture Decision Record",
		Definition:      "Decision record with status, context, and consequences.",
		BoundaryRule:    "Must center on one architectural decision statement.",
		PositiveExample: "docs/adr/0001-use-yaml-config.md.",
		NegativeExample: "General proposal lacking final decision fields.",
	},
	{
		Category:        CategoryRFC,
		Name:            "Request for Comments",
		Definition:      "Pre-decision request soliciting feedback on a proposal.",
		BoundaryRule:    "Includes review-oriented language and open questions.",
		PositiveExample: "RFC-012-token-budget-rule.md.",
		NegativeExample: "Finalized ADR.",
	},
	{
		Category:        CategoryDesignProposal,
		Name:            "Design Proposal / Tradeoff Memo",
		Definition:      "Problem framing with options, tradeoffs, and recommendation.",
		BoundaryRule:    "Evaluates alternatives, even if not formal RFC/ADR.",
		PositiveExample: "plan/62_corpus-acquisition.md.",
		NegativeExample: "Static API parameter reference.",
	},
	{
		Category:        CategoryRunbook,
		Name:            "Runbook / Playbook",
		Definition:      "Operational procedures for repeatable execution.",
		BoundaryRule:    "Action sequence for operators under normal ops conditions.",
		PositiveExample: "ops/runbook/deploy.md.",
		NegativeExample: "Incident retrospective narrative.",
	},
	{
		Category:        CategoryPostmortem,
		Name:            "Incident Postmortem",
		Definition:      "Retrospective incident analysis with root cause and actions.",
		BoundaryRule:    "Focused on one incident timeline and corrective actions.",
		PositiveExample: "postmortems/2026-01-auth-outage.md.",
		NegativeExample: "Forward-looking migration guide.",
	},
	{
		Category:        CategoryChangelog,
		Name:            "Changelog / Release Notes",
		Definition:      "Versioned list of user-facing changes or migrations.",
		BoundaryRule:    "Organized by versions or release dates.",
		PositiveExample: "CHANGELOG.md.",
		NegativeExample: "Troubleshooting FAQ list.",
	},
	{
		Category:        CategoryProjectDocs,
		Name:            "Project / Process Docs",
		Definition:      "Repository-level docs on contribution, security, governance.",
		BoundaryRule:    "Project policy/process and orientation, not deep spec.",
		PositiveExample: "README.md or CONTRIBUTING.md.",
		NegativeExample: "CLI flag reference page.",
	},
	{
		Category:        CategoryAPICLIConfig,
		Name:            "API / CLI / Config / Spec",
		Definition:      "Protocol, CLI, config, or schema specifications.",
		BoundaryRule:    "Defines interfaces, parameters, and compatibility expectations.",
		PositiveExample: "docs/cli/flags.md.",
		NegativeExample: "Tutorial walkthrough.",
	},
	{
		Category:        CategoryTroubleshooting,
		Name:            "Troubleshooting / FAQ / Onboarding / Glossary",
		Definition:      "Diagnostic and quick-help knowledge base content.",
		BoundaryRule:    "Problem-symptom mapping or term/FAQ lookup.",
		PositiveExample: "docs/troubleshooting/common-errors.md.",
		NegativeExample: "Release notes for a specific version.",
	},
}

// Taxonomy returns a copy of the category catalog.
func Taxonomy() []TaxonomyEntry {
	out := make([]TaxonomyEntry, len(taxonomyEntries))
	copy(out, taxonomyEntries)
	return out
}

// Classify maps a markdown file to a taxonomy category.
func Classify(path string, content string) Category {
	rel := normalizePath(path)
	base := strings.ToLower(filepath.Base(rel))
	title := strings.ToLower(firstHeading(content))

	switch {
	case isAgentControl(rel, base):
		return CategoryAgentControl
	case isADR(rel, base, title):
		return CategoryADR
	case isRFC(rel, base, title):
		return CategoryRFC
	case isPostmortem(rel, base, title):
		return CategoryPostmortem
	case isChangelog(base, title):
		return CategoryChangelog
	case isRunbook(rel, base, title):
		return CategoryRunbook
	case isTutorial(rel, base, title):
		return CategoryTutorial
	case isHowTo(rel, base, title):
		return CategoryHowTo
	case isTroubleshooting(rel, base, title):
		return CategoryTroubleshooting
	case isProjectDoc(rel, base):
		return CategoryProjectDocs
	case isDesignProposal(rel, base, title):
		return CategoryDesignProposal
	case isAPICLIConfig(rel, base, title):
		return CategoryAPICLIConfig
	case isExplanation(rel, base, title):
		return CategoryExplanation
	default:
		return CategoryReference
	}
}

func normalizePath(path string) string {
	return strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
}

func isAgentControl(rel string, base string) bool {
	if base == "agents.md" || base == "claude.md" {
		return true
	}
	if strings.Contains(rel, "/skills/") {
		return true
	}
	return strings.Contains(rel, "prompt") && strings.Contains(rel, "agent")
}

func isADR(rel string, base string, title string) bool {
	if strings.Contains(rel, "/adr/") {
		return true
	}
	if strings.HasPrefix(base, "adr-") || strings.HasPrefix(base, "adr_") {
		return true
	}
	return strings.Contains(title, "architecture decision")
}

func isRFC(rel string, base string, title string) bool {
	if strings.HasPrefix(base, "rfc") || strings.Contains(base, "-rfc-") {
		return true
	}
	if strings.Contains(rel, "/rfc/") {
		return true
	}
	return strings.HasPrefix(title, "rfc") || strings.Contains(title, "request for comments")
}

func isPostmortem(rel string, base string, title string) bool {
	if strings.Contains(rel, "postmortem") || strings.Contains(base, "postmortem") {
		return true
	}
	return strings.Contains(title, "incident") && strings.Contains(title, "postmortem")
}

func isChangelog(base string, title string) bool {
	if base == "changelog.md" || strings.Contains(base, "release-notes") {
		return true
	}
	if strings.Contains(base, "migration") {
		return true
	}
	return strings.Contains(title, "release notes") || strings.Contains(title, "changelog")
}

func isRunbook(rel string, base string, title string) bool {
	if strings.Contains(rel, "runbook") || strings.Contains(rel, "playbook") {
		return true
	}
	if strings.Contains(base, "runbook") || strings.Contains(base, "playbook") {
		return true
	}
	return strings.Contains(title, "runbook") || strings.Contains(title, "playbook")
}

func isTutorial(rel string, base string, title string) bool {
	if strings.Contains(rel, "tutorial") || strings.Contains(base, "tutorial") {
		return true
	}
	return strings.Contains(title, "tutorial")
}

func isHowTo(rel string, base string, title string) bool {
	if strings.Contains(rel, "how-to") || strings.Contains(rel, "howto") {
		return true
	}
	if strings.HasPrefix(base, "how-to") || strings.HasPrefix(base, "howto") {
		return true
	}
	return strings.Contains(title, "how to")
}

func isTroubleshooting(rel string, base string, title string) bool {
	if strings.Contains(rel, "troubleshooting") || strings.Contains(base, "troubleshooting") {
		return true
	}
	for _, token := range []string{"faq", "onboarding", "glossary"} {
		if strings.Contains(rel, token) || strings.Contains(base, token) || strings.Contains(title, token) {
			return true
		}
	}
	return false
}

func isProjectDoc(rel string, base string) bool {
	switch base {
	case "contributing.md", "security.md", "code_of_conduct.md":
		return true
	}
	if base == "readme.md" {
		if strings.Contains(rel, "/rules/") || strings.HasPrefix(rel, "internal/rules/") {
			return false
		}
		return rel == "readme.md" || strings.HasPrefix(rel, "docs/")
	}
	return strings.Contains(rel, "governance")
}

func isDesignProposal(rel string, base string, title string) bool {
	if strings.HasPrefix(rel, "guides/") || strings.Contains(rel, "/guides/") {
		return false
	}
	if strings.HasPrefix(rel, "plan/") || strings.Contains(rel, "/plan/") {
		return true
	}
	if strings.Contains(base, "design") || strings.Contains(base, "tradeoff") {
		return true
	}
	return strings.Contains(title, "design") || strings.Contains(title, "tradeoff")
}

func isAPICLIConfig(rel string, base string, title string) bool {
	if strings.Contains(rel, "/api/") || strings.Contains(rel, "/cli/") {
		return true
	}
	if strings.Contains(rel, "/spec") || strings.Contains(base, "spec") {
		return true
	}
	if strings.Contains(base, "config") {
		return true
	}
	if strings.Contains(base, "schema") {
		return true
	}
	return strings.Contains(title, "api") || strings.Contains(title, "cli")
}

func isExplanation(rel string, base string, title string) bool {
	if strings.HasPrefix(rel, "guides/") || strings.Contains(rel, "/guides/") {
		return true
	}
	if strings.Contains(base, "overview") || strings.Contains(base, "background") {
		return true
	}
	if strings.Contains(title, "why") {
		return true
	}
	return strings.Contains(title, "background") || strings.Contains(title, "explanation")
}

func firstHeading(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return ""
}
