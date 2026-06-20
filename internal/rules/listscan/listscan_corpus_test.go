package listscan

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// TestListscanMatchesASTCorpus is the corpus-level guard for listscan's
// contract: over every tracked Markdown file's front-matter-stripped body,
// the lists listscan derives from f.Lines must carry the same facts the
// goldmark AST does. The parse-skip gate (engine) feeds list/blockquote
// files to listscan only when this equivalence holds, so a regression here
// is a shipped-diagnostic regression on the Layer-0 path.
//
// Two file classes are out of listscan's contract and skipped, because the
// parse-skip gate excludes them anyway:
//
//   - Files carrying a `<?…?>` directive: the gate always parses these
//     (generated-section suppression needs the AST).
//   - Files whose body contains an HTML block: listscan does not model the
//     raw, opaque interior of an HTML block, so bullet- or number-shaped
//     lines inside one (e.g. a list written inside an HTML comment, or a
//     comment that splits a list item into two blocks) would diverge. The
//     gate's HTML-block guard keeps these off the skip path. Lifting this
//     skip means teaching listscan goldmark's seven HTML-block types.
func TestListscanMatchesASTCorpus(t *testing.T) {
	if testing.Short() {
		t.Skip("corpus walk skipped in -short mode")
	}
	root := corpusRoot(t)
	files := markdownFiles(t, root)
	if len(files) == 0 {
		t.Fatal("no corpus files found")
	}

	checked := 0
	for _, fp := range files {
		src, err := os.ReadFile(fp)
		if err != nil {
			t.Fatalf("read %s: %v", fp, err)
		}
		if bytes.Contains(src, []byte("<?")) {
			continue // directive files never take the parse-skip path
		}
		_, body := lint.StripFrontMatter(src)
		f, err := lint.NewFile(fp, body)
		if err != nil || f.AST == nil {
			continue
		}
		if astHasHTMLBlock(f.AST) {
			continue // outside listscan's contract (see doc comment)
		}
		want := corpusASTFacts(f)
		got, _ := Parse(f.Lines)
		if msg := corpusDiff(want, got); msg != "" {
			rel := strings.TrimPrefix(fp, root)
			t.Errorf("listscan diverges from goldmark on %s:\n%s", rel, msg)
		}
		checked++
	}
	t.Logf("listscan/AST equivalence verified on %d corpus files", checked)
}

func astHasHTMLBlock(root ast.Node) bool {
	found := false
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindHTMLBlock {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

// corpusASTFacts mirrors astFacts (listscan_ast_test.go) but over an
// already-built File so the comparison shares its FM-stripped Lines.
func corpusASTFacts(f *lint.File) []astList {
	var out []astList
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		list, ok := n.(*ast.List)
		if !ok {
			return ast.WalkContinue, nil
		}
		al := astList{
			ordered:   list.IsOrdered(),
			start:     list.Start,
			depth:     computeListDepth(list),
			firstLine: lineOfNode(f, list),
			lastLine:  lastLineOfNode(f, list),
			topLevel:  !parentIsListItem(list),
		}
		for c := list.FirstChild(); c != nil; c = c.NextSibling() {
			item := c.(*ast.ListItem)
			line := firstLineOfListItem(f, item)
			al.items = append(al.items, astItem{
				line:       line,
				level:      nestingLevel(item),
				ordered:    list.IsOrdered(),
				number:     itemNumber(f, line, list.IsOrdered()),
				multiBlock: isMultiItem(item),
			})
		}
		out = append(out, al)
		return ast.WalkContinue, nil
	})
	return out
}

// corpusDiff returns a human-readable description of the first structural
// difference between the AST-derived and listscan-derived lists, or "" when
// they match. It compares the same facts compareLists asserts.
func corpusDiff(want []astList, got []List) string {
	if len(want) != len(got) {
		return fmt.Sprintf("  list count: AST=%d listscan=%d\n  AST:%s\n  listscan:%s",
			len(want), len(got), fmtAST(want), fmtScan(got))
	}
	for i := range want {
		w, g := want[i], got[i]
		if w.ordered != g.Ordered || w.start != g.Start || w.depth != g.Depth ||
			w.firstLine != g.FirstLine || w.lastLine != g.LastLine || w.topLevel != g.TopLevel {
			return fmt.Sprintf("  list %d facts differ%s\n  listscan:%s",
				i, fmtAST(want[i:i+1]), fmtScan(got[i:i+1]))
		}
		if len(w.items) != len(g.Items) {
			return fmt.Sprintf("  list %d item count: AST=%d listscan=%d", i, len(w.items), len(g.Items))
		}
		for j := range w.items {
			wi, gi := w.items[j], g.Items[j]
			if wi.line != gi.Line || wi.level != gi.Level ||
				wi.number != gi.Number || wi.multiBlock != gi.MultiBlock {
				return fmt.Sprintf("  list %d item %d:\n"+
					"    AST(line=%d level=%d num=%d multi=%v)\n"+
					"    listscan(line=%d level=%d num=%d multi=%v)",
					i, j, wi.line, wi.level, wi.number, wi.multiBlock,
					gi.Line, gi.Level, gi.Number, gi.MultiBlock)
			}
		}
	}
	return ""
}

func markdownFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func corpusRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		p := filepath.Dir(dir)
		if p == dir {
			t.Fatal("go.mod not found")
		}
		dir = p
	}
}
