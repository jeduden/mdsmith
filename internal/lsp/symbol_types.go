package lsp

// LSP types for the symbol-navigation methods (documentSymbol,
// definition, implementation, references, workspace/symbol, and the
// callHierarchy/* trio). Kept in their own file to keep protocol.go
// focused on the lifecycle / diagnostics surface that pre-dates the
// navigation work.

// LSP SymbolKind enum (LSP §3.18). Only the four values mdsmith
// actually emits are declared here — Property for front-matter
// keys, String for headings, Key for link-reference definitions,
// and Event for directives. Editors render each kind with a
// distinct icon, so picking the right one matters even though we
// never read the value back.
type symbolKind int

const (
	symbolKindProperty symbolKind = 7
	symbolKindString   symbolKind = 15
	symbolKindKey      symbolKind = 20
	symbolKindEvent    symbolKind = 24
)

// documentSymbolParams (LSP §3.18.5).
type documentSymbolParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

// documentSymbol is the hierarchical reply form (LSP §3.18.5). We
// always emit the hierarchical form so editors can render headings
// as nested outlines.
type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           symbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []documentSymbol `json:"children,omitempty"`
}

// location is the LSP Location type (file URI + range).
type location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// definitionParams / referencesParams / implementationParams are
// LSP TextDocumentPositionParams; references adds a context.
type textDocumentPositionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type referencesParams struct {
	textDocumentPositionParams
	Context referencesContext `json:"context"`
}

type referencesContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// workspaceSymbolParams (LSP §3.18.7).
type workspaceSymbolParams struct {
	Query string `json:"query"`
}

// symbolInformation (LSP §3.18.7) is the workspace/symbol reply
// shape. Newer servers can return WorkspaceSymbol[] instead, but
// SymbolInformation has the broadest client compatibility.
type symbolInformation struct {
	Name          string     `json:"name"`
	Kind          symbolKind `json:"kind"`
	Location      location   `json:"location"`
	ContainerName string     `json:"containerName,omitempty"`
}

// callHierarchyItem (LSP §3.18.10). Used both as the prepare-call
// reply and as the from/to anchor in the incoming/outgoing call
// payloads.
type callHierarchyItem struct {
	Name           string     `json:"name"`
	Kind           symbolKind `json:"kind"`
	Detail         string     `json:"detail,omitempty"`
	URI            string     `json:"uri"`
	Range          Range      `json:"range"`
	SelectionRange Range      `json:"selectionRange"`
	// Data round-trips arbitrary state through the client back to
	// incomingCalls / outgoingCalls. We use it to anchor the item
	// (file path + optional anchor) without re-parsing.
	Data *callHierarchyData `json:"data,omitempty"`
}

// callHierarchyData carries enough state for a follow-up
// incoming/outgoing call to identify the anchor without depending on
// the client to round-trip the original Range exactly.
type callHierarchyData struct {
	File   string `json:"file"`
	Anchor string `json:"anchor,omitempty"`
}

// callHierarchyIncomingCall / callHierarchyOutgoingCall (LSP §3.18.10).
type callHierarchyIncomingCall struct {
	From       callHierarchyItem `json:"from"`
	FromRanges []Range           `json:"fromRanges"`
}

type callHierarchyOutgoingCall struct {
	To         callHierarchyItem `json:"to"`
	FromRanges []Range           `json:"fromRanges"`
}

type callHierarchyIncomingCallsParams struct {
	Item callHierarchyItem `json:"item"`
}

type callHierarchyOutgoingCallsParams struct {
	Item callHierarchyItem `json:"item"`
}
