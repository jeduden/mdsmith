package mdtext_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/stretchr/testify/assert"
)

// Segmentation regression test: mdtext.SplitSentences must produce
// the same sentences as upstream Punkt over the canonical
// golden-rules corpus. The file carries no build tag, so it gates
// both builds — default (fastMultiPunctWordAnnotation) and
// `-tags mdtext_punkt_upstream` (upstream
// english.MultiPunctWordAnnotation). If either implementation
// drifts from the corpus, the assertion fires.
//
// The cases are copied verbatim from
// github.com/neurosnap/sentences@v1.1.2/english/golden_rules_test.go
// (plan 191 task 5). The "rule #" name preserves upstream's label
// so a divergence is traceable to the source. Upstream tests raw
// `*Sentence.Text` (with leading whitespace); mdsmith's
// `SplitSentences` trims, so the expected slices here are trimmed
// to match.

type sentenceCase struct {
	name string
	text string
	want []string
}

// goldenRulesCases is the upstream golden-rules corpus (v1.1.2),
// trimmed for SplitSentences post-processing. Held at package scope
// so the test function stays under funlen.
var goldenRulesCases = []sentenceCase{
	{
		name: "21. Parenthetical inside sentence",
		text: "He teaches science (He previously worked for 5 years " +
			"as an engineer.) at the local University.",
		want: []string{
			"He teaches science (He previously worked for 5 years " +
				"as an engineer.) at the local University.",
		},
	},
	{
		name: "24. Single quotations inside sentence",
		text: "She turned to him, 'This is great.' she said.",
		want: []string{"She turned to him, 'This is great.' she said."},
	},
	{
		name: "25. Double quotations inside sentence",
		text: "She turned to him, \"This is great.\" she said.",
		want: []string{"She turned to him, \"This is great.\" she said."},
	},
	{
		name: "26. Double quotations at the end of a sentence",
		text: "She turned to him, \"This is great.\" She held the book " +
			"out to show him.",
		want: []string{
			"She turned to him, \"This is great.\"",
			"She held the book out to show him.",
		},
	},
	{
		name: "32. List (period followed by parens and period to end item)",
		text: "1.) The first item. 2.) The second item.",
		want: []string{"1.) The first item.", "2.) The second item."},
	},
	{
		name: "34. List (parens and period to end item)",
		text: "1) The first item. 2) The second item.",
		want: []string{"1) The first item.", "2) The second item."},
	},
	{
		name: "36. List (period to mark list and period to end item)",
		text: "1. The first item. 2. The second item.",
		want: []string{"1. The first item.", "2. The second item."},
	},
	{
		name: "43. Geo Coordinates",
		text: "You can find it at N°. 1026.253.553. That is where the " +
			"treasure is.",
		want: []string{
			"You can find it at N°. 1026.253.553.",
			"That is where the treasure is.",
		},
	},
	{
		name: "46. Ellipsis at end of quotation",
		text: "Thoreau argues that by simplifying one’s life, " +
			"“the laws of the universe will appear less complex. . . .”",
		want: []string{
			"Thoreau argues that by simplifying one’s life, " +
				"“the laws of the universe will appear less complex. . . .”",
		},
	},
	{
		name: "47. Ellipsis with square brackets",
		text: "\"Bohr [...] used the analogy of parallel stairways " +
			"[...]\" (Smith 55).",
		want: []string{
			"\"Bohr [...] used the analogy of parallel stairways " +
				"[...]\" (Smith 55).",
		},
	},
	{
		name: "48. Ellipsis as sentence boundary (standard ellipsis rules)",
		text: "If words are left off at the end of a sentence, and that " +
			"is all that is omitted, indicate the omission with ellipsis " +
			"marks (preceded and followed by a space) and then indicate " +
			"the end of the sentence with a period ... . Next sentence.",
		want: []string{
			"If words are left off at the end of a sentence, and that " +
				"is all that is omitted, indicate the omission with " +
				"ellipsis marks (preceded and followed by a space) and " +
				"then indicate the end of the sentence with a period ... .",
			"Next sentence.",
		},
	},
	{
		name: "49. Ellipsis as sentence boundary (non-standard ellipsis rules)",
		text: "I never meant that.... She left the store.",
		want: []string{"I never meant that....", "She left the store."},
	},
	{
		name: "51. 4-dot ellipsis",
		text: "One further habit which was somewhat weakened . . . was " +
			"that of combining words into self-interpreting compounds. " +
			". . . The practice was not abandoned. . . .",
		want: []string{
			"One further habit which was somewhat weakened . . . was " +
				"that of combining words into self-interpreting " +
				"compounds. . . .",
			"The practice was not abandoned. . . .",
		},
	},
}

func TestSplitSentences_GoldenRules(t *testing.T) {
	for _, tc := range goldenRulesCases {
		t.Run(tc.name, func(t *testing.T) {
			got := mdtext.SplitSentences(tc.text)
			assert.Equalf(t, tc.want, got,
				"upstream golden rule %q must segment identically "+
					"to upstream Punkt",
				tc.name)
		})
	}
}

// englishMainCases mirrors the high-signal cases in
// english/main_test.go from upstream (smart quotes, custom and
// supervised abbreviations, semicolons). They are particularly tied
// to the abbreviation classifier this plan optimizes — if the DFA
// disagrees with the regex on F.B.I., J.G., Sgt., Gov., or No., the
// assertion below catches it.
var englishMainCases = []sentenceCase{
	{
		name: "smart quotes",
		text: "Here is a quote, ”a smart one.” Will this break properly?",
		want: []string{
			"Here is a quote, ”a smart one.”",
			"Will this break properly?",
		},
	},
	{
		name: "custom abbrev F.B.I.",
		text: "One custom abbreviation is F.B.I.  The abbreviation, " +
			"F.B.I. should properly break.",
		want: []string{
			"One custom abbreviation is F.B.I.",
			"The abbreviation, F.B.I. should properly break.",
		},
	},
	{
		name: "custom abbrev G.D./J.G.",
		text: "An abbreviation near the end of a G.D. sentence.  J.G. " +
			"Wentworth was cool.",
		want: []string{
			"An abbreviation near the end of a G.D. sentence.",
			"J.G. Wentworth was cool.",
		},
	},
	{
		name: "supervised abbrevs Sgt./No./Gov.",
		text: "I am a Sgt. in the army.  I am a No. 1 student.  The " +
			"Gov. of Michigan is a dick.",
		want: []string{
			"I am a Sgt. in the army.",
			"I am a No. 1 student.",
			"The Gov. of Michigan is a dick.",
		},
	},
	{
		name: "semicolon",
		text: "I am here; you are over there.  Will the tokenizer " +
			"output two complete sentences?",
		want: []string{
			"I am here; you are over there.",
			"Will the tokenizer output two complete sentences?",
		},
	},
}

func TestSplitSentences_EnglishMainCases(t *testing.T) {
	for _, tc := range englishMainCases {
		t.Run(tc.name, func(t *testing.T) {
			got := mdtext.SplitSentences(tc.text)
			assert.Equalf(t, tc.want, got,
				"upstream main_test case %q must segment identically "+
					"to upstream Punkt",
				tc.name)
		})
	}
}
