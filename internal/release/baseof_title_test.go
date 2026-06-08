package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBaseofHomeTitleDoesNotDuplicateDescription parses
// `website/layouts/_default/baseof.html` and asserts the home-page
// branch of the `<title>` tag does not reference
// `.Site.Params.description`. That field carries the long Tagline
// which is also what the `<meta name="description">` emits, so a
// title that pulls from it duplicates the description in search-
// engine snippets — the same sentence printed twice. The home title
// uses the short eyebrow tagline instead.
func TestBaseofHomeTitleDoesNotDuplicateDescription(t *testing.T) {
	path := filepath.Join(repoRoot(t), "website", "layouts", "_default", "baseof.html")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read baseof.html")

	tree := parse.New(path)
	tree.Mode = parse.SkipFuncCheck
	treeSet := map[string]*parse.Tree{}
	_, err = tree.Parse(string(data), "{{", "}}", treeSet)
	require.NoError(t, err, "parse baseof.html")

	branch := findIsHomeTitleBranch(tree.Root)
	require.NotEmpty(t, branch, "did not find an IsHome title branch in baseof.html")
	for _, s := range branch {
		assert.NotContains(t, s, "Site.Params.description",
			"home-page <title> branch references Site.Params.description "+
				"(the long Tagline). The <meta name=\"description\"> already "+
				"emits that value; duplicating it in the title shows the same "+
				"sentence twice in search snippets. Use site.Home.Params.hero.eyebrow.")
	}
}

// findIsHomeTitleBranch scans the parse tree for `{{ if .IsHome }}`
// actions and returns the rendered string of each true-branch node
// that sits inside the document's `<title>` element. Returning the
// raw branch text lets the test assert on its contents without
// re-implementing Hugo's evaluation.
func findIsHomeTitleBranch(node parse.Node) []string {
	var found []string
	walkNode(node, func(n parse.Node) {
		ifn, ok := n.(*parse.IfNode)
		if !ok || ifn.List == nil {
			return
		}
		if !pipeReferencesField(ifn.Pipe, "IsHome") {
			return
		}
		found = append(found, ifn.List.String())
	})
	return found
}

func walkNode(n parse.Node, visit func(parse.Node)) {
	if n == nil {
		return
	}
	visit(n)
	switch x := n.(type) {
	case *parse.ListNode:
		for _, c := range x.Nodes {
			walkNode(c, visit)
		}
	case *parse.IfNode:
		walkNode(x.List, visit)
		walkNode(x.ElseList, visit)
	case *parse.RangeNode:
		walkNode(x.List, visit)
		walkNode(x.ElseList, visit)
	case *parse.WithNode:
		walkNode(x.List, visit)
		walkNode(x.ElseList, visit)
	}
}

func pipeReferencesField(pipe *parse.PipeNode, field string) bool {
	if pipe == nil {
		return false
	}
	for _, cmd := range pipe.Cmds {
		for _, arg := range cmd.Args {
			fn, ok := arg.(*parse.FieldNode)
			if !ok {
				continue
			}
			for _, ident := range fn.Ident {
				if strings.EqualFold(ident, field) {
					return true
				}
			}
		}
	}
	return false
}
