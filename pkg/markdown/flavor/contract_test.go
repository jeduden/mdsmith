package flavor_test

// Contract test for the pkg/markdown/flavor public API. Every symbol
// asserted here is part of the package's stable surface; removing or
// changing one is a public-API break that must be evaluated against
// the pkg/markdown compatibility policy. The asserts use the
// reflect-free trick of taking each symbol's address (or invoking it
// with zero values) — a build failure here is the API-shape gate.

import (
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gparser "github.com/yuin/goldmark/parser"

	"github.com/jeduden/mdsmith/pkg/markdown"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor/ext"
)

// TestContract_FlavorIdentity covers the Flavor type, its IsValid
// and String methods, and every named constant.
func TestContract_FlavorIdentity(t *testing.T) {
	var fl flavor.Flavor
	_ = fl.IsValid()
	_ = fl.String()
	_ = []flavor.Flavor{
		flavor.FlavorCommonMark,
		flavor.FlavorGFM,
		flavor.FlavorGoldmark,
		flavor.FlavorAny,
		flavor.FlavorPandoc,
		flavor.FlavorPHPExtra,
		flavor.FlavorMultiMarkdown,
		flavor.FlavorMyST,
	}
	_, _ = flavor.ParseFlavor("commonmark")
}

// TestContract_FeatureIdentity covers the Feature type, AllFeatures,
// Feature.Name, and every named constant.
func TestContract_FeatureIdentity(t *testing.T) {
	var feat flavor.Feature
	_ = feat.Name()
	_ = flavor.AllFeatures()
	_ = []flavor.Feature{
		flavor.FeatureTables,
		flavor.FeatureTaskLists,
		flavor.FeatureStrikethrough,
		flavor.FeatureBareURLAutolinks,
		flavor.FeatureFootnotes,
		flavor.FeatureDefinitionLists,
		flavor.FeatureHeadingIDs,
		flavor.FeatureSuperscript,
		flavor.FeatureSubscript,
		flavor.FeatureMathBlock,
		flavor.FeatureMathInline,
		flavor.FeatureAbbreviations,
		flavor.FeatureGitHubAlerts,
	}
	_ = flavor.Supports(flavor.FlavorGFM, flavor.FeatureTables)
}

// TestContract_FindingShape pins Finding and HeadingIDExtra to their
// documented field set. Any field rename, removal, or type change
// breaks the build here.
func TestContract_FindingShape(t *testing.T) {
	fin := flavor.Finding{
		Feature: flavor.FeatureTables,
		Line:    1,
		Column:  1,
		Start:   0,
		End:     0,
		Extra:   flavor.HeadingIDExtra{AttrStart: 0, AttrEnd: 0},
	}
	_ = fin.Feature
	_ = fin.Line
	_ = fin.Column
	_ = fin.Start
	_ = fin.End
	_ = fin.Extra

	hx := flavor.HeadingIDExtra{AttrStart: 1, AttrEnd: 2}
	_ = hx.AttrStart
	_ = hx.AttrEnd
}

// Signature pins at package scope: a constructor signature change
// breaks the build at these assignments. staticcheck's "could omit
// type" quick-fix rule is suppressed for package-level vars where
// the explicit type is the contract.
var (
	_                  func(*markdown.Document, func(flavor.Feature) bool) []flavor.Finding = flavor.Detect
	_                  func() (gparser.Parser, func())                                      = flavor.NewPooledParser
	_                  func(...goldmark.Extender) (gparser.Parser, func())                  = flavor.NewPooledParserWith
	withSharedParserFn func(func(gparser.Parser))                                           = flavor.WithSharedParser

	// Public rewriter helpers: same idea, pinned by signature.
	_ func([]byte, *ast.Heading) (flavor.HeadingIDExtra, bool) = flavor.FindHeadingID
	_ func(*ast.Blockquote, []byte) bool                       = flavor.IsGitHubAlert
	_ func([]byte, int) (int, int)                             = flavor.LineCol
	_ func(ast.Node) ast.Node                                  = flavor.NearestBlockAncestor
)

// TestContract_DetectAndParserPinsAreCallable exercises the
// signature-pinned variables at runtime so a compile-time pin alone
// cannot mask a panic on the happy path.
func TestContract_DetectAndParserPinsAreCallable(t *testing.T) {
	doc := markdown.Parse([]byte("# h\n"))
	_ = flavor.Detect(doc, nil)
	_ = flavor.Detect(doc, func(f flavor.Feature) bool { return f == flavor.FeatureTables })

	p, reset := flavor.NewPooledParser()
	reset()
	_ = p
	p2, reset2 := flavor.NewPooledParserWith()
	reset2()
	_ = p2

	withSharedParserFn(func(p gparser.Parser) { _ = p })
}

// TestContract_Rewriters covers the small surface needed by external
// rewriters: FindHeadingID, IsGitHubAlert, LineCol, NearestBlockAncestor.
func TestContract_Rewriters(t *testing.T) {
	source := []byte("# h\n")
	h := ast.NewHeading(1)
	_, _ = flavor.FindHeadingID(source, h)
	_ = flavor.IsGitHubAlert(ast.NewBlockquote(), source)
	_, _ = flavor.LineCol(source, 0)
	_ = flavor.NearestBlockAncestor(h)
}

// TestContract_ExtensionExtenders pins the five custom extension
// Extender singletons. Each must satisfy goldmark.Extender.
func TestContract_ExtensionExtenders(t *testing.T) {
	exts := []any{
		ext.Superscript,
		ext.Subscript,
		ext.MathBlock,
		ext.MathInline,
		ext.Abbreviation,
	}
	for _, e := range exts {
		if e == nil {
			t.Fatalf("extender is nil: %v", e)
		}
	}
}

// TestContract_ExtensionNodeKinds pins the public NodeKind values
// produced by the five custom extensions, so external code can
// type-switch on them.
func TestContract_ExtensionNodeKinds(t *testing.T) {
	_ = []ast.NodeKind{
		ext.KindSuperscript,
		ext.KindSubscript,
		ext.KindMathBlock,
		ext.KindMathInline,
		ext.KindAbbreviationDefinition,
		ext.KindAbbreviationReference,
	}
}
