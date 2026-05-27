package flavor_test

// Contract test for the pkg/markdown/flavor public API. Every symbol
// asserted here is part of the package's stable surface; removing or
// changing one is a public-API break that must be evaluated against
// the pkg/markdown compatibility policy. The asserts use the
// reflect-free trick of taking each symbol's address (or invoking it
// with zero values) — a build failure here is the API-shape gate.

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"

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

// TestContract_DetectSignature pins Detect's signature: it takes a
// *markdown.Document and an accept predicate; it returns []Finding.
// A nil predicate accepts every feature.
// detectSig pins the Detect function's signature at package init
// time. A signature change would break the type assertion below.
var detectSig func(*markdown.Document, func(flavor.Feature) bool) []flavor.Finding = flavor.Detect

func TestContract_DetectSignature(t *testing.T) {
	doc := markdown.Parse([]byte("# h\n"))
	_ = detectSig(doc, nil)
	_ = detectSig(doc, func(f flavor.Feature) bool { return f == flavor.FeatureTables })
}

// TestContract_ParserConstructors pins the four constructor signatures:
//
//   - NewParser() parser.Parser
//   - NewParserWith(...goldmark.Extender) parser.Parser
//   - NewPooledParser() (parser.Parser, func())
//   - NewPooledParserWith(...goldmark.Extender) (parser.Parser, func())
func TestContract_ParserConstructors(t *testing.T) {
	var p parser.Parser
	p = flavor.NewParser()
	_ = p
	p = flavor.NewParserWith()
	_ = p
	var reset func()
	p, reset = flavor.NewPooledParser()
	_ = p
	reset()
	p, reset = flavor.NewPooledParserWith()
	_ = p
	reset()
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
