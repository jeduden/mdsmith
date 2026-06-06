package lsp

import (
	"encoding/json"
	"sort"

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
	sort.Slice(out, func(i, j int) bool {
		if out[i].ContainerName != out[j].ContainerName {
			return out[i].ContainerName < out[j].ContainerName
		}
		return out[i].Name < out[j].Name
	})
	_ = s.t.writeResponse(msg.ID, out)
}
