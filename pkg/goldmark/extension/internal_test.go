package extension

// Internal unit tests for unexported helpers in the extension
// package: isTableDelim, applyFootnoteTemplate, and related
// internals.

import (
	"testing"
)

func TestApplyFootnoteTemplate_AllBranches(t *testing.T) {
	// Drive all branches:
	//   - fast path (no placeholders) -> return template as-is.
	//   - ^^ found -> substitute index.
	//   - %% found -> substitute refCount.
	cases := []struct {
		name     string
		tmpl     string
		index    int
		refCount int
		want     string
	}{
		{"fast-path", "no placeholders here", 5, 3, "no placeholders here"},
		{"only-index", "idx=^^ end", 7, 0, "idx=7 end"},
		{"only-refs", "refs=%% end", 0, 4, "refs=4 end"},
		{"both", "i=^^ r=%%", 10, 2, "i=10 r=2"},
		{"empty", "", 0, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(applyFootnoteTemplate([]byte(c.tmpl), c.index, c.refCount))
			if got != c.want {
				t.Errorf("applyFootnoteTemplate(%q) = %q, want %q", c.tmpl, got, c.want)
			}
		})
	}
}

func TestIsTableDelim_AllBranches(t *testing.T) {
	// Drive each branch:
	//   - IndentWidth > 3 -> false
	//   - allSep (only dashes) -> false
	//   - invalid char -> false
	//   - valid -> true
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid-simple", "---|---", true},
		{"valid-with-colons", ":---|---:|:---:", true},
		{"valid-with-spaces", " --- | --- ", true},
		{"only-dashes-no-pipe", "------", false},  // allSep -> false
		{"invalid-char", "---|--x", false},        // x is not allowed
		{"too-indented", "    ---|---", false},    // IndentWidth > 3
		{"empty", "", false},                      // allSep stays true on empty -> false
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isTableDelim([]byte(c.in)); got != c.want {
				t.Errorf("isTableDelim(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
