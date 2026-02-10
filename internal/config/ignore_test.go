package config

import "testing"

func TestIsIgnored_MatchesGlobPattern(t *testing.T) {
	patterns := []string{"vendor/**"}
	if !IsIgnored(patterns, "vendor/lib.md") {
		t.Error("expected vendor/lib.md to be ignored")
	}
}

func TestIsIgnored_MatchesBasename(t *testing.T) {
	patterns := []string{"CHANGELOG.md"}
	if !IsIgnored(patterns, "/some/path/CHANGELOG.md") {
		t.Error("expected CHANGELOG.md to be ignored by basename")
	}
}

func TestIsIgnored_NoMatch(t *testing.T) {
	patterns := []string{"vendor/**"}
	if IsIgnored(patterns, "src/main.md") {
		t.Error("expected src/main.md not to be ignored")
	}
}

func TestIsIgnored_EmptyPatterns(t *testing.T) {
	if IsIgnored(nil, "test.md") {
		t.Error("expected no match with empty patterns")
	}
}

func TestIsIgnored_InvalidPatternSkipped(t *testing.T) {
	patterns := []string{"[invalid"}
	if IsIgnored(patterns, "test.md") {
		t.Error("expected invalid pattern to be skipped")
	}
}

func TestIsIgnored_CleanedPath(t *testing.T) {
	patterns := []string{"vendor/**"}
	if !IsIgnored(patterns, "vendor/./lib.md") {
		t.Error("expected cleaned path to match")
	}
}
