package flavor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jeduden/mdsmith/pkg/markdown"
)

// pinCase is one row in the byte-identical Detect corpus.
type pinCase struct {
	name string
	src  string
	want []findingShape
}

// pinCorpus is the representative input corpus mapped to its
// byte-exact Finding stream (feature, line, column for every emit,
// in document order). Any code path that subtly reorders, drops,
// duplicates, or shifts a finding will break TestDetectByteIdenticalPin
// — that is the plan-185 byte-stable guarantee against MDS034
// diagnostic drift. Start and End byte anchors are not pinned here:
// they are exercised individually by the per-feature TestDetect*
// tests and have less stable cross-feature semantics (some are
// line-start anchors, some are zero-length).
var pinCorpus = []pinCase{
	{
		name: "plain commonmark emits nothing",
		src:  "# Heading\n\nA paragraph.\n\n- one\n- two\n",
		want: nil,
	},
	{
		name: "tables and bare URLs sort by start",
		src: "See https://example.com.\n\n" +
			"| a | b |\n| - | - |\n| 1 | 2 |\n",
		want: []findingShape{
			{FeatureBareURLAutolinks, 1, 5},
			{FeatureTables, 3, 1},
		},
	},
	{
		name: "every feature in one document",
		src: "# Title {#top}\n\n" +
			"- [ ] task\n\n" +
			"| a | b |\n| - | - |\n| 1 | 2 |\n\n" +
			"~~old~~ https://example.com\n\n" +
			"text^sup^ and H~2~O\n\n" +
			"$x+1$ inline\n\n" +
			"$$\nblock\n$$\n\n" +
			"*[API]: Application Programming Interface\n\n" +
			"Use API here.[^1]\n\n" +
			"[^1]: footnote body\n\n" +
			"term\n:   definition\n\n" +
			"> [!NOTE]\n> Something.\n",
		want: []findingShape{
			{FeatureHeadingIDs, 1, 9},
			{FeatureTaskLists, 3, 3},
			{FeatureTables, 5, 1},
			{FeatureStrikethrough, 9, 1},
			{FeatureBareURLAutolinks, 9, 9},
			{FeatureSuperscript, 11, 5},
			{FeatureSubscript, 11, 16},
			{FeatureMathInline, 13, 1},
			{FeatureMathBlock, 15, 1},
			{FeatureAbbreviations, 19, 1},
			{FeatureFootnotes, 21, 1},
			{FeatureAbbreviations, 21, 5},
			{FeatureFootnotes, 23, 1},
			{FeatureDefinitionLists, 25, 1},
			{FeatureGitHubAlerts, 28, 1},
		},
	},
	{
		name: "github alert variants in order",
		src: "> [!NOTE]\n> n.\n\n" +
			"> [!TIP]\n> t.\n\n" +
			"> [!WARNING]\n> w.\n",
		want: []findingShape{
			{FeatureGitHubAlerts, 1, 1},
			{FeatureGitHubAlerts, 4, 1},
			{FeatureGitHubAlerts, 7, 1},
		},
	},
}

// TestDetectByteIdenticalPin runs Detect over pinCorpus and asserts
// the exact Finding stream for every entry.
func TestDetectByteIdenticalPin(t *testing.T) {
	for _, tc := range pinCorpus {
		t.Run(tc.name, func(t *testing.T) {
			doc := markdown.Parse([]byte(tc.src))
			got := Detect(doc, nil)
			var gotShapes []findingShape //nolint:prealloc
			for _, f := range got {
				gotShapes = append(gotShapes,
					findingShape{f.Feature, f.Line, f.Column})
			}
			assert.Equalf(t, tc.want, gotShapes,
				"finding stream drift; want vs got:\n%s\n",
				diffShapes(tc.want, gotShapes))
		})
	}
}

// findingShape is the byte-identical pin tuple: the three Finding
// fields whose stability is the documented MDS034 contract.
type findingShape struct {
	Feature Feature
	Line    int
	Column  int
}

// diffShapes renders a side-by-side report of expected vs actual
// findings to make a drift-test failure self-diagnostic. assert's
// default diff prints opaque struct dumps; this format mirrors the
// stats-line style the rest of the test suite uses.
func diffShapes(want, got []findingShape) string {
	var sb strings.Builder
	max := len(want)
	if len(got) > max {
		max = len(got)
	}
	sb.WriteString("idx | want feature line:col | got feature line:col\n")
	for i := 0; i < max; i++ {
		var w, g string
		if i < len(want) {
			w = fmt.Sprintf("%s %d:%d", want[i].Feature.Name(), want[i].Line, want[i].Column)
		} else {
			w = "(none)"
		}
		if i < len(got) {
			g = fmt.Sprintf("%s %d:%d", got[i].Feature.Name(), got[i].Line, got[i].Column)
		} else {
			g = "(none)"
		}
		marker := " "
		if w != g {
			marker = "*"
		}
		fmt.Fprintf(&sb, "%s%3d | %-30s | %s\n", marker, i, w, g)
	}
	return sb.String()
}
