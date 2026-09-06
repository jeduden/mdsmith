package lsp

import (
	"cmp"
	"encoding/json"
	"slices"

	"github.com/jeduden/mdsmith/internal/index"
)

// LSP workspace-symbol handler: workspace/symbol substring search over
// the workspace index. Split out of symbols.go so each LSP dispatch
// group owns its own file (cf. rename.go, completion.go).

// handleWorkspaceSymbol returns SymbolInformation entries for every
// substring match in the workspace index.
func (s *Server) handleWorkspaceSymbol(msg *requestMessage) {
	var p workspaceSymbolParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid workspace/symbol params")
		return
	}
	idx := s.ensureIndex()
	hits := idx.SearchSymbols(p.Query, 1024)
	out := make([]symbolInformation, 0, len(hits))
	for _, h := range hits {
		kind := symbolKindString
		switch h.Symbol.Kind {
		case index.SymbolFrontMatter:
			kind = symbolKindProperty
		case index.SymbolLinkRef:
			kind = symbolKindKey
		case index.SymbolDirective:
			kind = symbolKindEvent
		}
		out = append(out, symbolInformation{
			Name: h.Symbol.Name,
			Kind: kind,
			Location: location{
				URI:   s.workspaceURI(h.File),
				Range: rangeAt(h.Symbol.SelectionLine, h.Symbol.SelectionCol, nil),
			},
			ContainerName: h.File,
		})
	}
	sortSymbolInformation(out)
	_ = s.t.writeResponse(msg.ID, out)
}

// sortSymbolInformation orders out by (ContainerName, Name), the order
// workspace/symbol results are returned in. slices.SortFunc compares
// the concrete symbolInformation values directly, unlike sort.Slice,
// which drives reflect.Swapper internally (see
// docs/development/high-performance-go.md, "reflect in hot paths").
// This runs on every workspace/symbol query the editor's symbol picker
// issues as the user types.
func sortSymbolInformation(out []symbolInformation) {
	slices.SortFunc(out, func(a, b symbolInformation) int {
		return cmp.Or(
			cmp.Compare(a.ContainerName, b.ContainerName),
			cmp.Compare(a.Name, b.Name),
		)
	})
}
