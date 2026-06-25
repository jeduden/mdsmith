package integration

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/wordlist"
	"github.com/stretchr/testify/require"
)

// slopPatternsPath is the docs-author catalog that is the source of
// truth for the no-llm-tells convention's curated lists.
const slopPatternsPath = "../../.claude/skills/docs-author/slop-patterns.md"

// TestNoLLMTellsConventionMatchesSlopCatalog asserts that every entry
// in the no-llm-tells convention's forbidden lists still appears in
// the docs-author slop-patterns catalog. It fails CI when an item is
// removed from the catalog without being removed from the convention,
// or vice versa, so the parallel lists cannot silently diverge.
func TestNoLLMTellsConventionMatchesSlopCatalog(t *testing.T) {
	catalog := readSlopCatalog(t)

	// ai-speak holds vocabulary tells followed by phrasal tells.
	aiSpeak, ok := wordlist.Builtin("ai-speak")
	require.True(t, ok, "built-in ai-speak word-list must exist")
	vocabulary := catalog["Vocabulary tells"]
	phrases := catalog["Phrasal tells"]
	for _, item := range aiSpeak.Entries {
		if vocabulary[item] || phrases[item] {
			continue
		}
		t.Errorf(
			"ai-speak entry %q is not in slop-patterns.md "+
				"Vocabulary tells or Phrasal tells", item,
		)
	}

	// ai-openers holds the banned sentence openers.
	aiOpeners, ok := wordlist.Builtin("ai-openers")
	require.True(t, ok, "built-in ai-openers word-list must exist")
	openers := catalog["Sentence openers"]
	for _, item := range aiOpeners.Entries {
		if openers[item] {
			continue
		}
		t.Errorf(
			"ai-openers entry %q is not in "+
				"slop-patterns.md Sentence openers", item,
		)
	}
}

// readSlopCatalog parses slop-patterns.md into a map from section
// heading ("Vocabulary tells", "Phrasal tells", "Sentence openers") to
// a set of normalized catalog items. Vocabulary bullets may list
// several comma-separated words and carry a "(figurative)"-style tag,
// which is stripped. Phrasal bullets are wrapped in double quotes,
// which are stripped. Sentence-opener bullets are taken verbatim
// (including the trailing comma).
func readSlopCatalog(t *testing.T) map[string]map[string]bool {
	t.Helper()
	path, err := filepath.Abs(slopPatternsPath)
	require.NoError(t, err)
	data, err := os.ReadFile(path) //nolint:gosec // fixed in-repo path
	require.NoError(t, err)

	out := map[string]map[string]bool{
		"Vocabulary tells": {},
		"Phrasal tells":    {},
		"Sentence openers": {},
	}
	var section, bullet string
	emit := func() {
		recordCatalogBullet(out, section, bullet)
		bullet = ""
	}
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "## "):
			emit()
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		case strings.HasPrefix(line, "- "):
			emit()
			bullet = line
		case line == "":
			emit()
		case bullet != "":
			bullet += " " + line // continuation of a wrapped bullet
		}
	}
	emit()
	require.NoError(t, sc.Err())
	return out
}

// recordCatalogBullet adds the items in one catalog bullet to the
// section's set. Vocabulary bullets list comma-separated words with an
// optional sense tag; phrasal bullets are quoted; sentence openers are
// taken verbatim. Bullets outside the three tracked sections, and the
// empty bullet, are ignored.
func recordCatalogBullet(out map[string]map[string]bool, section, bullet string) {
	set, ok := out[section]
	if !ok || bullet == "" {
		return
	}
	item := strings.TrimSpace(strings.TrimPrefix(bullet, "- "))
	switch section {
	case "Vocabulary tells":
		for _, word := range strings.Split(item, ",") {
			set[normalizeVocab(word)] = true
		}
	case "Phrasal tells":
		set[strings.Trim(item, `"`)] = true
	default: // Sentence openers
		set[item] = true
	}
}

// normalizeVocab strips a parenthetical sense tag (e.g.
// "landscape (figurative)" -> "landscape") and surrounding whitespace
// from a vocabulary catalog word.
func normalizeVocab(word string) string {
	if i := strings.IndexByte(word, '('); i >= 0 {
		word = word[:i]
	}
	return strings.TrimSpace(word)
}
