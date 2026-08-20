package forbiddentext

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// benchNeedles mirrors the shape and size of the no-llm-tells list the
// built-in convention configures (and that this repository itself pins):
// ~70 entries mixing single words with multi-word phrases. It is
// inlined rather than imported so the benchmark stays independent of
// the convention package's contents.
func benchNeedles() []string {
	return []string{
		"delve", "dive into", "dive deep", "deep dive", "tapestry",
		"realm", "testament", "vibrant", "pivotal", "robust",
		"seamless", "leverage", "unlock", "unleash", "embark",
		"foster", "showcase", "underscore", "myriad", "plethora",
		"crucial", "vital", "essential", "paramount", "profound",
		"intricate", "nuanced", "holistic", "innovative", "cutting-edge",
		"state-of-the-art", "game-changer", "revolutionize", "transform",
		"empower", "elevate", "streamline", "optimize", "facilitate",
		"utilize", "endeavor", "commence", "ascertain", "utilization",
		"furthermore", "moreover", "additionally", "consequently",
		"nevertheless", "notwithstanding", "albeit", "whilst", "amongst",
		"it's important to note that", "it's worth mentioning that",
		"in today's fast-paced world", "in the digital age",
		"in the realm of", "in the world of", "at its core",
		"plays a crucial role", "stands as a testament to",
		"a deep dive into", "as we navigate", "harness the power of",
		"unlock the potential of", "embark on a journey",
		"navigating the complexities of",
	}
}

// benchParagraph is a representative compliant paragraph: prose that
// matches no needle, which is the overwhelmingly common case and
// therefore the one whose cost dominates a workspace scan.
const benchParagraph = "The parser walks each block node once and " +
	"records the byte offsets it needs later, so a second pass over " +
	"the same source is never required. Callers that only want the " +
	"line numbers can stop there; the rest of the pipeline reads the " +
	"cached offsets directly from the file."

func benchFile(tb testing.TB, paragraphs int) *lint.File {
	tb.Helper()
	var b strings.Builder
	b.WriteString("# Document\n\n")
	for i := 0; i < paragraphs; i++ {
		b.WriteString(benchParagraph)
		b.WriteString("\n\n")
	}
	f, err := lint.NewFile("doc.md", []byte(b.String()))
	require.NoError(tb, err)
	return f
}

// BenchmarkCheck_NoLLMTells measures MDS056 against the needle list a
// real project configures. The rule scanned every paragraph once per
// needle — O(paragraphs x needles) end-to-end string searches — which
// the merged CPU profile of `mdsmith check .` attributed 9.4% of all
// CPU to, nearly all of it inside strings.Contains.
//
// The budget below fails loudly if the single-pass matcher is ever
// lost, but it only runs under -bench, and CI does not pass -bench for
// internal/rules. Nor can internal/integration's per-opt-in-rule gate
// see this: that harness measures each rule at DEFAULT settings, and
// MDS056's default `contains` is empty, so its pinned row exercises a
// rule that does no work. The gate CI actually relies on is
// TestRule_CheckNodeConsultsMatcher in matcher_test.go, which pins the
// wiring deterministically and without timing. This benchmark is for
// measuring the win, not for guarding it.
func BenchmarkCheck_NoLLMTells(b *testing.B) {
	needles := benchNeedles()
	f := benchFile(b, 40)

	r := &Rule{}
	require.NoError(b, r.ApplySettings(map[string]any{"contains": needles}))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if got := r.Check(f); got != nil {
			b.Fatalf("compliant document produced %d diagnostics", len(got))
		}
	}
	b.StopTimer()

	nsPerParagraph := float64(b.Elapsed().Nanoseconds()) / float64(b.N) / 40
	const budgetNs = 3000
	if nsPerParagraph > budgetNs {
		b.Fatalf(
			"MDS056 costs %.0f ns/paragraph against %d needles, over the "+
				"%d ns budget: the single-pass matcher has regressed to a "+
				"per-needle rescan (see docs/development/high-performance-go.md)",
			nsPerParagraph, len(needles), budgetNs,
		)
	}
}
