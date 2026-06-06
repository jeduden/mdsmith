package lsp

import (
	"bytes"
	"encoding/json"
	"path"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
)

// LSP call-hierarchy handlers: prepareCallHierarchy plus incoming /
// outgoing call resolution over the workspace link graph. Split out of
// symbols.go so each LSP dispatch group owns its own file
// (cf. rename.go, completion.go).

// handlePrepareCallHierarchy returns a single call-hierarchy item
// anchored at (file, optional heading). On a directive arg the item
// is the target file; on a heading line, the heading section.
func (s *Server) handlePrepareCallHierarchy(msg *requestMessage) {
	var p textDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid prepareCallHierarchy params")
		return
	}
	source, rel, ok := s.docTextOrFile(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, []callHierarchyItem{})
		return
	}
	idx := s.ensureIndex()
	line := p.Position.Line + 1
	col := lspPositionToByteColumn(source, line, p.Position.Character)
	res := index.Locator{Path: rel}.Locate(source, line, col)

	var item callHierarchyItem
	switch res.Tag {
	case index.TokenHeading:
		fe, _ := idx.File(rel)
		item = callHierarchyItem{
			Name:           res.Name,
			Kind:           symbolKindString,
			Detail:         "#" + res.Anchor,
			URI:            p.TextDocument.URI,
			Range:          headingRangeFromIndex(rel, res.Anchor, fe, source),
			SelectionRange: rangeAt(p.Position.Line+1, 1, source),
			Data:           &callHierarchyData{File: rel, Anchor: res.Anchor},
		}
	case index.TokenDirectiveArg:
		if res.DirectiveTargetFile != "" {
			tgt := linkgraph.ResolveRelTarget(rel, res.DirectiveTargetFile)
			if tgt == "" {
				_ = s.t.writeResponse(msg.ID, []callHierarchyItem{})
				return
			}
			item = callHierarchyItem{
				Name:           tgt,
				Kind:           symbolKindString,
				URI:            s.workspaceURI(tgt),
				Range:          Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
				SelectionRange: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
				Data:           &callHierarchyData{File: tgt},
			}
		}
	case index.TokenFileTop:
		// File-level call hierarchy: only the very top of the
		// document anchors at the file. Plain prose lower in the
		// file does not — see Plan 131's cursor matrix
		// (file root / heading / directive arg). Without this
		// gate the editor would offer a "call hierarchy" entry
		// for arbitrary positions, including paragraphs that
		// have no inbound or outbound references.
		item = callHierarchyItem{
			Name:           rel,
			Kind:           symbolKindString,
			URI:            p.TextDocument.URI,
			Range:          rangeForLines(1, lineCount(source), source),
			SelectionRange: rangeAt(1, 1, source),
			Data:           &callHierarchyData{File: rel},
		}
	}
	if item.URI == "" {
		_ = s.t.writeResponse(msg.ID, []callHierarchyItem{})
		return
	}
	_ = s.t.writeResponse(msg.ID, []callHierarchyItem{item})
}

func headingRangeFromIndex(rel, anchor string, fe *index.FileEntry, source []byte) Range {
	if fe == nil {
		return rangeAt(1, 1, source)
	}
	for _, sym := range fe.Symbols {
		if sym.Kind == index.SymbolHeading && sym.Anchor == anchor {
			return rangeForLines(sym.StartLine, sym.EndLine, source)
		}
	}
	return rangeAt(1, 1, source)
}

func lineCount(source []byte) int {
	if len(source) == 0 {
		return 1
	}
	n := bytes.Count(source, []byte{'\n'})
	if source[len(source)-1] != '\n' {
		n++
	}
	return n
}

// handleIncomingCalls returns every workspace edge into the item.
// Edges from the same source file are coalesced into one entry with
// multiple `fromRanges`; LSP clients render each fromRange as a
// click target under the same caller, so emitting one item per edge
// would show the same caller N times in the call-hierarchy view.
func (s *Server) handleIncomingCalls(msg *requestMessage) {
	var p callHierarchyIncomingCallsParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid incomingCalls params")
		return
	}
	if p.Item.Data == nil {
		_ = s.t.writeResponse(msg.ID, []callHierarchyIncomingCall{})
		return
	}
	idx := s.ensureIndex()
	edges := idx.IncomingEdges(p.Item.Data.File, p.Item.Data.Anchor)

	type bucket struct {
		item   callHierarchyItem
		ranges []Range
	}
	order := make([]string, 0, len(edges))
	groups := make(map[string]*bucket, len(edges))
	for _, e := range edges {
		// Call hierarchy is a cross-file dependency view: keep only
		// the edge kinds that represent inter-document flow. Anchor
		// and reference-style links are intra-document, and an
		// edge whose SourceFile equals the item's File is a
		// self-reference (e.g. `[a](#sec)` to a heading whose
		// anchor matches `Anchor`); both would clutter the result.
		if e.Kind == index.EdgeAnchorLink || e.Kind == index.EdgeRefLink {
			continue
		}
		if e.SourceFile == p.Item.Data.File {
			continue
		}
		r := rangeAt(e.SourceLine, e.SourceCol, nil)
		if g, ok := groups[e.SourceFile]; ok {
			g.ranges = append(g.ranges, r)
			continue
		}
		groups[e.SourceFile] = &bucket{
			item: callHierarchyItem{
				Name:           e.SourceFile,
				Kind:           symbolKindString,
				URI:            s.workspaceURI(e.SourceFile),
				Range:          r,
				SelectionRange: r,
				Data:           &callHierarchyData{File: e.SourceFile},
			},
			ranges: []Range{r},
		}
		order = append(order, e.SourceFile)
	}
	out := make([]callHierarchyIncomingCall, 0, len(order))
	for _, k := range order {
		g := groups[k]
		out = append(out, callHierarchyIncomingCall{From: g.item, FromRanges: g.ranges})
	}
	_ = s.t.writeResponse(msg.ID, out)
}

// handleOutgoingCalls returns every edge out of the item, scoped
// to the section when the item carries an anchor (heading-level
// call hierarchy). Edges to the same target file are coalesced into
// one entry with multiple `fromRanges`, matching the LSP grouping
// contract.
func (s *Server) handleOutgoingCalls(msg *requestMessage) {
	var p callHierarchyOutgoingCallsParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid outgoingCalls params")
		return
	}
	if p.Item.Data == nil {
		_ = s.t.writeResponse(msg.ID, []callHierarchyOutgoingCall{})
		return
	}
	idx := s.ensureIndex()
	edges := idx.OutgoingEdges(p.Item.Data.File)
	startLine, endLine := outgoingScope(idx, p.Item.Data)

	type bucket struct {
		item   callHierarchyItem
		ranges []Range
	}
	order := make([]string, 0, len(edges))
	groups := make(map[string]*bucket, len(edges))
	for _, e := range edges {
		// Same-file anchor / ref-style links are intra-document and
		// don't fit the cross-file call-graph view.
		if e.Kind == index.EdgeAnchorLink || e.Kind == index.EdgeRefLink {
			continue
		}
		// Heading-scoped item: skip edges outside the section's
		// source range so a heading with no outbound links doesn't
		// inherit calls from sibling sections.
		if endLine > 0 && (e.SourceLine < startLine || e.SourceLine > endLine) {
			continue
		}
		toFile := e.TargetFile
		if toFile == "" {
			// Catalog without expansion: point at the host file's
			// directory as a placeholder. Plan 131 documents this
			// fallback explicitly under "Open Questions".
			toFile = path.Dir(p.Item.Data.File)
		}
		r := rangeAt(e.SourceLine, e.SourceCol, nil)
		if g, ok := groups[toFile]; ok {
			g.ranges = append(g.ranges, r)
			continue
		}
		// Coalesce by target file. The bucket represents the
		// callee file as a whole, so Data.Anchor must stay empty
		// — different edges from the source can target different
		// headings inside the same file, and a follow-up
		// incomingCalls on this item would otherwise be filtered
		// to whichever anchor happened to land in the bucket
		// first. To navigate to a specific heading, the user can
		// open the callee and re-issue prepareCallHierarchy
		// there.
		groups[toFile] = &bucket{
			item: callHierarchyItem{
				Name:           toFile,
				Kind:           symbolKindString,
				URI:            s.workspaceURI(toFile),
				Range:          Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
				SelectionRange: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
				Data:           &callHierarchyData{File: toFile},
			},
			ranges: []Range{r},
		}
		order = append(order, toFile)
	}
	out := make([]callHierarchyOutgoingCall, 0, len(order))
	for _, k := range order {
		g := groups[k]
		out = append(out, callHierarchyOutgoingCall{To: g.item, FromRanges: g.ranges})
	}
	_ = s.t.writeResponse(msg.ID, out)
}

// outgoingScope returns the [startLine, endLine] bound for outgoing
// edges when the call-hierarchy item is heading-scoped. Returns
// (1, 0) — i.e. an open-ended range — for file-level items so the
// caller treats every edge as in scope.
func outgoingScope(idx *index.Index, data *callHierarchyData) (int, int) {
	if data == nil || data.Anchor == "" {
		return 1, 0
	}
	fe, ok := idx.File(data.File)
	if !ok {
		return 1, 0
	}
	for _, sym := range fe.Symbols {
		if sym.Kind == index.SymbolHeading && sym.Anchor == data.Anchor {
			return sym.StartLine, sym.EndLine
		}
	}
	return 1, 0
}
