package extension_test

// Direct-call coverage for the predicate methods (Close,
// CloseBlock, CanInterruptParagraph, CanAcceptIndentedLine) on
// extension parsers that the normal Parse flow does not always
// reach. Also covers the Dump / String pretty-printers on
// extension AST nodes that exist for debugging.

import (
	"io"
	"os"
	"testing"

	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
)

func silence(t *testing.T, fn func()) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		_ = w.Close()
		os.Stdout = old
		_ = r.Close()
	}()
	go io.Copy(io.Discard, r)
	fn()
}

func TestNewTableASTTransformer_Direct(t *testing.T) {
	// NewTableASTTransformer just returns the package-level
	// singleton.  Call it once for coverage.
	if extension.NewTableASTTransformer() == nil {
		t.Error("NewTableASTTransformer returned nil")
	}
}

func TestDefinitionList_Predicates(t *testing.T) {
	// DefinitionList parser and DefinitionDescription parser
	// each have predicates Close, CanInterruptParagraph, and
	// CanAcceptIndentedLine that the normal block-parser
	// dispatcher might not call on every input. Drive them
	// directly.
	listP := extension.NewDefinitionListParser()
	listP.Close(nil, nil, nil) // Close is a no-op in current impl
	if listP.CanAcceptIndentedLine() {
		t.Error("DefinitionListParser.CanAcceptIndentedLine should be false")
	}

	descP := extension.NewDefinitionDescriptionParser()
	if !descP.CanInterruptParagraph() {
		t.Error("DefinitionDescriptionParser.CanInterruptParagraph should be true")
	}
	if descP.CanAcceptIndentedLine() {
		t.Error("DefinitionDescriptionParser.CanAcceptIndentedLine should be false")
	}
}

func TestExtensionAST_TableNodeString(t *testing.T) {
	// Cover the String() / Dump() of TableCellAlignType.
	for _, a := range []extast.Alignment{
		extast.AlignLeft,
		extast.AlignRight,
		extast.AlignCenter,
		extast.AlignNone,
	} {
		_ = a.String()
	}
}

func TestExtensionAST_TableDump(t *testing.T) {
	// Empty Alignments — Table.Dump iterates 0 times.
	table := extast.NewTable()
	silence(t, func() { table.Dump(nil, 0) })

	// Populated Alignments — iterates all rows; the trailing
	// branch (Println on non-last entry) needs at least 2 entries.
	table2 := extast.NewTable()
	table2.Alignments = []extast.Alignment{
		extast.AlignLeft,
		extast.AlignRight,
		extast.AlignCenter,
	}
	silence(t, func() { table2.Dump(nil, 0) })

	header := extast.NewTableHeader(extast.NewTableRow(nil))
	silence(t, func() { header.Dump(nil, 0) })

	row := extast.NewTableRow([]extast.Alignment{extast.AlignLeft, extast.AlignRight})
	silence(t, func() { row.Dump(nil, 0) })

	cell := extast.NewTableCell()
	silence(t, func() { cell.Dump(nil, 0) })
	cell.Alignment = extast.AlignCenter
	silence(t, func() { cell.Dump(nil, 0) })
}
