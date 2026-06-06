package lsp

import (
	"encoding/json"
	"strings"

	"github.com/jeduden/mdsmith/internal/index"
)

// LSP document-symbol outline: textDocument/documentSymbol and the
// heading / front-matter / directive tree builders it renders. Split
// out of symbols.go so each LSP dispatch group owns its own file
// (cf. rename.go, completion.go).

// handleDocumentSymbol returns a hierarchical outline of the buffer.
// Front-matter keys hang off a synthetic top-of-file symbol;
// directives become children of their enclosing heading.
func (s *Server) handleDocumentSymbol(msg *requestMessage) {
	var p documentSymbolParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid documentSymbol params")
		return
	}
	source, _, ok := s.docTextOrFile(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, []documentSymbol{})
		return
	}
	// Outline is built from the live source so unsaved edits are
	// reflected even if the index hasn't been refreshed yet.
	out := buildOutline(source)
	_ = s.t.writeResponse(msg.ID, out)
}

// buildOutline turns a freshly parsed FileEntry into an LSP
// hierarchical outline. Headings stack by level; front-matter keys
// gather under a synthetic top-of-file node; directives attach to
// their enclosing heading or to the file root.
func buildOutline(source []byte) []documentSymbol {
	idx := index.New("")
	idx.Update("buffer", source)
	fe, ok := idx.File("buffer")
	if !ok {
		return nil
	}

	var fmKids []documentSymbol
	var dirRoot []documentSymbol
	var headings []index.Symbol
	for _, sym := range fe.Symbols {
		switch sym.Kind {
		case index.SymbolFrontMatter:
			fmKids = append(fmKids, leafSymbol(sym, source))
		case index.SymbolDirective:
			dirRoot = append(dirRoot, leafSymbol(sym, source))
		case index.SymbolHeading:
			headings = append(headings, sym)
		case index.SymbolLinkRef:
			dirRoot = append(dirRoot, leafSymbol(sym, source))
		}
	}

	var roots []documentSymbol
	if len(fmKids) > 0 {
		// Synthetic "front matter" parent at line 1.
		roots = append(roots, documentSymbol{
			Name:           "front matter",
			Kind:           symbolKindProperty,
			Range:          rangeForLines(1, 1, source),
			SelectionRange: rangeForLines(1, 1, source),
			Children:       fmKids,
		})
	}

	hroots := buildHeadingTree(headings, source)
	// Attach directives + link-refs whose line falls under a heading
	// span; everything else hoists to the file root.
	hroots, unattached := attachDirectives(hroots, dirRoot)
	roots = append(roots, hroots...)
	roots = append(roots, unattached...)
	return roots
}

// buildHeadingTree turns a flat heading list into a nested
// documentSymbol tree using a level-aware stack walk.
func buildHeadingTree(headings []index.Symbol, source []byte) []documentSymbol {
	var roots []documentSymbol
	type stackEntry struct {
		level int
		node  *documentSymbol
	}
	var stack []stackEntry
	for _, h := range headings {
		ds := documentSymbol{
			Name:           headingDisplay(h),
			Detail:         headingDetail(h),
			Kind:           symbolKindString,
			Range:          rangeForLines(h.StartLine, h.EndLine, source),
			SelectionRange: rangeForLines(h.SelectionLine, h.SelectionLine, source),
		}
		// Pop until we find a parent with a lower level.
		for len(stack) > 0 && stack[len(stack)-1].level >= h.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, ds)
			stack = append(stack, stackEntry{
				level: h.Level,
				node:  &roots[len(roots)-1],
			})
		} else {
			parent := stack[len(stack)-1].node
			parent.Children = append(parent.Children, ds)
			stack = append(stack, stackEntry{
				level: h.Level,
				node:  &parent.Children[len(parent.Children)-1],
			})
		}
	}
	return roots
}

// attachDirectives walks the heading tree and reparents each
// directive/leaf into the deepest heading whose range covers its
// start line. Leaves that don't fall under any heading return as
// the second value so the caller can hoist them to the file root.
func attachDirectives(headings []documentSymbol, leaves []documentSymbol) ([]documentSymbol, []documentSymbol) {
	var unattached []documentSymbol
	for _, leaf := range leaves {
		startLine := leaf.SelectionRange.Start.Line + 1 // back to 1-based
		if !attachInto(headings, leaf, startLine) {
			unattached = append(unattached, leaf)
		}
	}
	return headings, unattached
}

func attachInto(nodes []documentSymbol, leaf documentSymbol, startLine int) bool {
	for i := range nodes {
		// LSP ranges are [start, end) in 0-based form. The leaf's
		// start line lives inside the node when it falls between
		// the node's Range start and end (inclusive).
		nodeStart := nodes[i].Range.Start.Line + 1
		nodeEnd := nodes[i].Range.End.Line + 1
		if startLine >= nodeStart && startLine <= nodeEnd {
			// Try to attach into a deeper child first.
			if attachInto(nodes[i].Children, leaf, startLine) {
				return true
			}
			nodes[i].Children = append(nodes[i].Children, leaf)
			return true
		}
	}
	return false
}

func leafSymbol(sym index.Symbol, source []byte) documentSymbol {
	kind := symbolKindKey
	switch sym.Kind {
	case index.SymbolFrontMatter:
		kind = symbolKindProperty
	case index.SymbolDirective:
		kind = symbolKindEvent
	case index.SymbolLinkRef:
		kind = symbolKindKey
	}
	return documentSymbol{
		Name:           sym.Name,
		Detail:         leafDetail(sym),
		Kind:           kind,
		Range:          rangeForLines(sym.StartLine, sym.EndLine, source),
		SelectionRange: rangeForLines(sym.SelectionLine, sym.SelectionLine, source),
	}
}

func headingDisplay(h index.Symbol) string {
	if h.Name == "" {
		return strings.Repeat("#", h.Level)
	}
	return h.Name
}

func headingDetail(h index.Symbol) string {
	if h.Anchor == "" {
		return ""
	}
	return "#" + h.Anchor
}

func leafDetail(sym index.Symbol) string {
	switch sym.Kind {
	case index.SymbolDirective:
		return "<?" + sym.Name + "?>"
	case index.SymbolLinkRef:
		return "[" + sym.Name + "]:"
	}
	return ""
}
