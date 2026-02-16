package corpus

import "testing"

func TestTaxonomy_IncludesAllCategories(t *testing.T) {
	t.Parallel()

	entries := Taxonomy()
	if len(entries) != len(AllCategories()) {
		t.Fatalf("taxonomy entries = %d, want %d", len(entries), len(AllCategories()))
	}

	seen := make(map[Category]bool, len(entries))
	for _, entry := range entries {
		seen[entry.Category] = true
		if entry.Definition == "" || entry.BoundaryRule == "" {
			t.Fatalf("entry %s missing definition or boundary rule", entry.Category)
		}
		if entry.PositiveExample == "" || entry.NegativeExample == "" {
			t.Fatalf("entry %s missing examples", entry.Category)
		}
	}
	for _, category := range AllCategories() {
		if !seen[category] {
			t.Fatalf("missing category in taxonomy: %s", category)
		}
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		body string
		want Category
	}{
		{name: "agent", path: "AGENTS.md", body: "# Agent Rules", want: CategoryAgentControl},
		{name: "tutorial", path: "docs/tutorial/setup.md", body: "# Tutorial", want: CategoryTutorial},
		{name: "how-to", path: "docs/how-to/deploy.md", body: "# Deploy", want: CategoryHowTo},
		{name: "adr", path: "docs/adr/0001.md", body: "# Decision", want: CategoryADR},
		{name: "rfc", path: "RFC-001.md", body: "# RFC 1", want: CategoryRFC},
		{name: "project", path: "README.md", body: "# Project", want: CategoryProjectDocs},
		{name: "design", path: "plan/62_corpus-acquisition.md", body: "# Corpus", want: CategoryDesignProposal},
		{name: "troubleshooting", path: "docs/faq.md", body: "# FAQ", want: CategoryTroubleshooting},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Classify(tt.path, tt.body)
			if got != tt.want {
				t.Fatalf("Classify(%q) = %s, want %s", tt.path, got, tt.want)
			}
		})
	}
}
