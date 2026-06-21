package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	_ "github.com/jeduden/mdsmith/internal/rules"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// TestInlineIndexEquivalence_CodeSpans is the Layer 1 counterpart to
// TestLayer0Equivalence_Fixtures: for every parse-skip-eligible Markdown
// file in the repository corpus, the code-span content and literal ranges
// served on the nil-AST path (from the shared run-grouped inline parse,
// InlineBlocks) must be byte-identical to the ones the goldmark AST walk
// produces.
//
// It restricts the comparison to the files the production parse-skip gate
// would actually skip — those with no fenced/indented code block
// (lint.SourceMayHaveCodeBlock) and no `<?` directive marker — so the test
// scope matches the inputs the gate admits. That is the soundness contract
// that lets MDS047 and MDS054 resolve to Layer 0.
func TestInlineIndexEquivalence_CodeSpans(t *testing.T) {
	root := repoRoot(t)
	files := collectMarkdownCorpus(t, root)
	require.NotEmpty(t, files)

	var checked int
	for _, path := range files {
		source, err := os.ReadFile(path)
		require.NoError(t, err)
		_, body := lint.StripFrontMatter(source)

		// Mirror the engine's parse-skip eligibility (runner.layer0SkipEligible).
		if lint.SourceMayHaveCodeBlock(body) || bytes.Contains(body, []byte("<?")) {
			continue
		}
		checked++

		rel, _ := filepath.Rel(root, path)
		t.Run(rel, func(t *testing.T) {
			astFile, err := lint.NewFile(path, body)
			require.NoError(t, err)
			l0File := lint.NewFileLines(path, body)

			assert.Equal(t, astFile.CodeSpanContentRanges(), l0File.CodeSpanContentRanges(),
				"code-span content ranges differ between AST and inline index")
			assert.Equal(t, astFile.CodeSpanLiteralRanges(), l0File.CodeSpanLiteralRanges(),
				"code-span literal ranges differ between AST and inline index")
		})
	}
	require.NotZero(t, checked, "expected at least one parse-skip-eligible corpus file")
}

// parityInlineRuleIDs are the parity inline rules whose diagnostics must be
// byte-identical between the goldmark AST path and the nil-AST inline scan.
// They are the rules the plan's equivalence gate names: bare URLs (MDS012),
// empty alt text (MDS032), and link validity (MDS062).
var parityInlineRuleIDs = []string{"MDS012", "MDS032", "MDS062"}

// TestInlineIndexEquivalence_ParityRules holds the Layer 1 inline scan to
// byte-identity with goldmark for every parity inline rule, across the
// parse-skip-eligible repository corpus. For each eligible file it runs each
// rule once over an AST-backed File and once over a nil-AST File (which reads
// the inline scan via lint.InlineBlocks) and requires the diagnostic slices
// to match exactly. A divergence here means the scanner produced a different
// inline node stream than goldmark — the gate the plan requires the scanner
// to clear before it can ship.
func TestInlineIndexEquivalence_ParityRules(t *testing.T) {
	root := repoRoot(t)
	files := collectMarkdownCorpus(t, root)
	require.NotEmpty(t, files)

	var checked int
	for _, path := range files {
		source, err := os.ReadFile(path)
		require.NoError(t, err)
		_, body := lint.StripFrontMatter(source)

		if lint.SourceMayHaveCodeBlock(body) || bytes.Contains(body, []byte("<?")) {
			continue
		}
		checked++

		rel, _ := filepath.Rel(root, path)
		t.Run(rel, func(t *testing.T) {
			astFile, err := lint.NewFile(path, body)
			require.NoError(t, err)
			l0File := lint.NewFileLines(path, body)

			for _, id := range parityInlineRuleIDs {
				r := rule.ByID(id)
				require.NotNil(t, r, "rule %s not registered", id)
				assert.Equal(t, r.Check(astFile), r.Check(l0File),
					"%s diagnostics differ between AST and inline scan", id)
			}
		})
	}
	require.NotZero(t, checked, "expected at least one parse-skip-eligible corpus file")
}

// inlineNodeRec is a flat, comparable projection of the inline AST fields the
// parity rules read: the kind, a Text node's segment bounds and line-break /
// raw flags, and a link's or image's destination and title. Two trees that
// agree on the ordered slice of these records produce identical diagnostics
// for every parity inline rule, so the slice is the byte-identity oracle.
type inlineNodeRec struct {
	kind            string
	start, stop     int
	dest, title     string
	soft, hard, raw bool
}

// collectInlineNodeRecs walks n in document order and records every Text,
// Link, Image, AutoLink, and CodeSpan node. base maps a Text node's
// run-local segment offsets to document-absolute offsets.
func collectInlineNodeRecs(n ast.Node, base int, out *[]inlineNodeRec) {
	switch x := n.(type) {
	case *ast.Text:
		*out = append(*out, inlineNodeRec{
			kind: "Text", start: base + x.Segment.Start, stop: base + x.Segment.Stop,
			soft: x.SoftLineBreak(), hard: x.HardLineBreak(), raw: x.IsRaw(),
		})
	case *ast.Link:
		*out = append(*out, inlineNodeRec{kind: "Link", dest: string(x.Destination), title: string(x.Title)})
	case *ast.Image:
		*out = append(*out, inlineNodeRec{kind: "Image", dest: string(x.Destination), title: string(x.Title)})
	case *ast.AutoLink:
		*out = append(*out, inlineNodeRec{kind: "AutoLink"})
	case *ast.CodeSpan:
		*out = append(*out, inlineNodeRec{kind: "CodeSpan"})
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectInlineNodeRecs(c, base, out)
	}
}

// TestInlineIndexEquivalence_NodeStream is the deepest equivalence gate: for
// every parse-skip-eligible corpus file it compares the full inline node
// stream produced on the nil-AST path (lint.InlineBlocks — which uses the
// byte scanner, falling back to goldmark per run) against the goldmark
// whole-document parse, node by node. It catches divergences the
// per-rule diagnostic gate cannot see (a Text split or destination that no
// enabled rule happens to observe), so the scanner cannot ship a different
// inline tree than goldmark even on a construct no current rule reads.
func TestInlineIndexEquivalence_NodeStream(t *testing.T) {
	root := repoRoot(t)
	files := collectMarkdownCorpus(t, root)
	require.NotEmpty(t, files)

	var checked int
	for _, path := range files {
		source, err := os.ReadFile(path)
		require.NoError(t, err)
		_, body := lint.StripFrontMatter(source)

		if lint.SourceMayHaveCodeBlock(body) || bytes.Contains(body, []byte("<?")) {
			continue
		}
		checked++

		rel, _ := filepath.Rel(root, path)
		t.Run(rel, func(t *testing.T) {
			astFile, err := lint.NewFile(path, body)
			require.NoError(t, err)
			l0File := lint.NewFileLines(path, body)

			var got, want []inlineNodeRec
			for _, blk := range lint.InlineBlocks(l0File) {
				collectInlineNodeRecs(blk.Node, blk.Offset, &got)
			}
			collectInlineNodeRecs(astFile.AST, 0, &want)
			assert.Equal(t, want, got, "inline node stream differs between AST and scan")
		})
	}
	require.NotZero(t, checked, "expected at least one parse-skip-eligible corpus file")
}
