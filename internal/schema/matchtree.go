package schema

import (
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/fieldinterp"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"
)

// regexEscape matches a backslash escaping an ASCII punctuation
// byte — the only escapes regexp.QuoteMeta emits. Reversing it
// recovers the literal heading stem from a desugared matcher.
var regexEscape = regexp.MustCompile(`\\([^0-9A-Za-z_])`)

// HeadingStem splits a scope's matcher regex into its literal stem
// (the heading text with `\#(digits)` / `\#(fmvar(name))`
// interpolations removed and regexp escaping reversed) plus the
// `fmvar` field names and whether a `digits` capture is present.
// It works uniformly for proto-sugar scopes and hand-written
// inline `regex:` bodies, so the projector's key seam does not
// need to know which parser produced the scope.
func HeadingStem(sc *Scope) (stem string, fmvars []string, hasDigits bool) {
	if sc == nil || sc.Matcher == nil {
		if sc != nil {
			return sc.Heading, nil, false
		}
		return "", nil, false
	}
	pat := sc.Matcher.Regex
	var lit strings.Builder
	cursor := 0
	_ = scanInterps(pat, func(expr string, start, end int) error {
		lit.WriteString(pat[cursor:start])
		expr = strings.TrimSpace(expr)
		if expr == "digits" {
			hasDigits = true
		} else if name, ok := parseFmvarCall(expr); ok {
			fmvars = append(fmvars, name)
		}
		cursor = end
		return nil
	})
	lit.WriteString(pat[cursor:])
	s := lit.String()
	s = strings.TrimPrefix(s, "^")
	s = strings.TrimSuffix(s, "$")
	s = regexEscape.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s), fmvars, hasDigits
}

// MatchTree is the projection-ready record of how a document's AST
// satisfied a composed Schema. It is produced after a successful
// schema match (extraction is gated on conformance) and consumed by
// internal/extract to build a data tree without re-matching.
//
// Validate keeps its diagnostic-only return; BuildMatchTree is a
// separate walk so MDS020 is unaffected.
type MatchTree struct {
	// Frontmatter is the document's decoded front matter, unchanged.
	Frontmatter map[string]any

	// Root is a synthetic node whose Children are the top-level
	// section matches in document order. Root.Scope is nil.
	Root *ScopeMatch
}

// ScopeMatch records one matched section (or the no-heading
// preamble). A repeating scope produces one ScopeMatch per
// occurrence, all sharing the same Scope pointer so the projector
// can group consecutive occurrences into an array.
type ScopeMatch struct {
	// Scope is the schema scope this match satisfied. Nil only for
	// the synthetic MatchTree.Root.
	Scope *Scope

	// Preamble reports whether this is the `heading: null`
	// no-heading section (content before the first child heading).
	Preamble bool

	// Heading is the document heading that matched. Zero-valued for
	// the preamble and the synthetic root.
	Heading DocHeading

	// Captures holds every placeholder bound by this heading: named
	// regex groups (the `n` digits capture) plus `{field}` fmvar
	// placeholders resolved from front matter. Nil when the scope's
	// heading is a plain literal.
	Captures map[string]string

	// Children are the matched child scopes in document order.
	Children []*ScopeMatch

	// Content are the matched content entries in declared order.
	Content []ContentMatch
}

// ContentMatch pairs a schema ContentEntry with the AST node that
// satisfied it and that node's 1-based source line.
type ContentMatch struct {
	Entry *ContentEntry
	Node  ast.Node
	Line  int
}

// BuildMatchTree walks f against the composed schema and records the
// matched headings, captures, and content nodes. It assumes the
// document already conforms (callers gate on a clean Validate), so
// it uses the validator's in-order run/yield helpers without the
// error-recovery branches.
func BuildMatchTree(f *lint.File, sch *Schema, docFM map[string]any) *MatchTree {
	mt := &MatchTree{Frontmatter: docFM, Root: &ScopeMatch{}}
	if sch == nil || sch.IsEmpty() {
		return mt
	}
	rootLevel := sch.EffectiveRootLevel()
	heads := skipBelow(ExtractDocHeadings(f), rootLevel)

	var blocks []contentBlock
	if anyScopeHasContent(sch.Sections) {
		blocks = topLevelBlocks(f, parseWithTableExt(f.Source))
	}

	claimed := make(map[int]bool)
	buildScopeMatches(
		f, sch.Sections, heads, rootLevel, 1, len(f.Lines)+1,
		claimed, blocks, docFM, mt.Root,
	)
	return mt
}

// buildScopeMatches mirrors walkContentScopes: pair each scope with
// its heading run, recurse into children over the matched section's
// line window, and collect content nodes. Wildcard slots and broad
// `.+` matchers are skipped — the projection is a faithful image of
// the declared schema only.
func buildScopeMatches(
	f *lint.File, scopes []Scope, heads []DocHeading,
	expectedLevel, parentStart, parentEnd int,
	claimed map[int]bool, blocks []contentBlock,
	docFM map[string]any, parent *ScopeMatch,
) {
	for i := range scopes {
		sc := &scopes[i]
		if isSlotMatcher(sc.Matcher) || isBroadMatcher(sc.Matcher) {
			continue
		}
		if sc.Preamble {
			end := firstContentHeadingLine(heads, expectedLevel, parentStart, parentEnd)
			sm := &ScopeMatch{Scope: sc, Preamble: true}
			collectContent(sc, blocks, parentStart, end, sm)
			parent.Children = append(parent.Children, sm)
			continue
		}
		for _, matched := range ScopeRunIndices(
			scopes, i, heads, expectedLevel, parentStart, parentEnd, claimed, docFM,
		) {
			dh := heads[matched]
			claimed[matched] = true
			end := contentScopeEndLine(heads, matched, dh.Level, parentEnd)
			sm := &ScopeMatch{
				Scope:    sc,
				Heading:  dh,
				Captures: scopeCaptures(sc, dh, docFM),
			}
			collectContent(sc, blocks, dh.Line+1, end, sm)
			if len(sc.Sections) > 0 {
				buildScopeMatches(
					f, sc.Sections, heads, expectedLevel+1, dh.Line, end,
					claimed, blocks, docFM, sm,
				)
			}
			parent.Children = append(parent.Children, sm)
		}
	}
}

// scopeCaptures merges the regex named captures with the `{field}`
// fmvar placeholders declared in the scope's heading template. The
// `n` digits group comes straight from the regex; an fmvar field
// has no capture group, so its value is resolved from front matter
// — both the placeholder name and its bound value survive.
func scopeCaptures(sc *Scope, dh DocHeading, docFM map[string]any) map[string]string {
	_, caps := headingCaptures(sc.Matcher, dh, docFM)
	_, fmvars, _ := HeadingStem(sc)
	// The only regex-named capture the schema parser emits is the
	// `n` digits group, and protoTokenRegex routes a `{n}`
	// placeholder to `\#(digits)` rather than fmvar — so an fmvar
	// field name can never collide with an existing capture key.
	// No dedup guard is needed.
	for _, name := range fmvars {
		path := fieldinterp.ParseCUEPath(name)
		if len(path) == 0 {
			continue
		}
		val, err := fieldinterp.ResolvePath(docFM, path)
		if err != nil {
			continue
		}
		if caps == nil {
			caps = make(map[string]string, 1)
		}
		caps[name] = val
	}
	return caps
}

// collectContent pairs each declared content entry with the first
// not-yet-consumed body node of the matching kind, in declared
// order. Conformance is already established, so the in-order scan
// suffices; `unlisted` entries never bind a node.
//
// An optional entry that the document omits must not swallow nodes
// belonging to a later entry: when the current entry does not match
// a node but a later listed entry would, the scan stops and leaves
// the node for that entry. Without this, an absent optional
// paragraph before a required code block would consume the code
// block while searching, so it would never be projected even
// though MDS020 accepts the file. This mirrors the content
// validator's findLaterEntry yield in validate_content.go.
func collectContent(
	sc *Scope, blocks []contentBlock, startLine, endLine int, sm *ScopeMatch,
) {
	if len(sc.Content) == 0 {
		return
	}
	nodes := blocksInRange(blocks, startLine, endLine)
	nodeIdx := 0
	for ei := range sc.Content {
		e := &sc.Content[ei]
		if e.Kind == ContentKindUnlisted {
			continue
		}
		for nodeIdx < len(nodes) {
			n := nodes[nodeIdx]
			if nodeMatchesKind(e.Kind, n.node) {
				sm.Content = append(sm.Content, ContentMatch{
					Entry: e, Node: n.node, Line: n.line,
				})
				nodeIdx++
				break
			}
			if laterContentEntryMatches(sc.Content, ei+1, n.node) {
				// Node belongs to a later listed entry; leave it
				// and move on so this (absent) entry does not
				// consume it.
				break
			}
			nodeIdx++
		}
	}
}

// laterContentEntryMatches reports whether n matches the kind of
// any listed (non-`unlisted`) content entry at or after startIdx.
func laterContentEntryMatches(content []ContentEntry, startIdx int, n ast.Node) bool {
	for j := startIdx; j < len(content); j++ {
		if content[j].Kind == ContentKindUnlisted {
			continue
		}
		if nodeMatchesKind(content[j].Kind, n) {
			return true
		}
	}
	return false
}
