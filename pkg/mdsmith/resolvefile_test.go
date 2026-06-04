package mdsmith

import "testing"

// TestResolveFileReturnsRawResolution covers the native ResolveFile
// entry the CLI's `kinds resolve` / `kinds why` route through: given a
// uri plus its already-parsed front-matter kinds and fields, it returns
// the raw *config.FileResolution (the per-rule merge chain the `why`
// subcommand walks), resolved against the session's compiled config.
func TestResolveFileReturnsRawResolution(t *testing.T) {
	cfg := "kinds:\n  doc:\n    path-pattern: \"docs/**/*.md\"\n" +
		"kind-assignment:\n  - glob: [\"docs/**/*.md\"]\n    kinds: [doc]\n"
	s := newTestSession(t, cfg, nil)

	res := s.ResolveFile("docs/guide.md", nil, nil)
	if res == nil {
		t.Fatal("ResolveFile returned nil")
	}
	if res.File != "docs/guide.md" {
		t.Fatalf("res.File = %q, want docs/guide.md", res.File)
	}
	var hasDoc bool
	for _, k := range res.Kinds {
		if k.Name == "doc" {
			hasDoc = true
		}
	}
	if !hasDoc {
		t.Fatalf("ResolveFile: expected kind 'doc' for docs/guide.md, got %+v", res.Kinds)
	}
	// The per-rule chain the `why` subcommand needs must be present.
	if len(res.Rules) == 0 {
		t.Fatal("ResolveFile: expected a non-empty per-rule resolution map")
	}
}

// TestResolveFileHonorsFrontMatterInputs verifies the front-matter
// kinds/fields the caller passes feed kind assignment — the CLI parses
// and validates front matter, then hands the parsed inputs here.
func TestResolveFileHonorsFrontMatterInputs(t *testing.T) {
	cfg := "kinds:\n  titled: {}\n" +
		"kind-assignment:\n  - glob: [\"*.md\"]\n    kinds: [titled]\n" +
		"    fields-present: [title]\n"
	s := newTestSession(t, cfg, nil)

	res := s.ResolveFile("a.md", nil, map[string]any{"title": "Hello"})
	var found bool
	for _, k := range res.Kinds {
		if k.Name == "titled" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ResolveFile: expected 'titled' via fields-present, got %+v", res.Kinds)
	}
}
