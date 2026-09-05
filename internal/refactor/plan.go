package refactor

// Plan is the neutral result of a refactor operation. Edits groups the
// per-file text edits by output target — a CLI file path or an LSP
// document URI, the same key vocabulary the Workspace seam's Resolve
// returns — so a host applies each key's edits to that one target.
// FileOp describes a file relocation the host performs after applying
// the edits; it is nil for a heading or label rename and non-nil only
// for a move.
//
// The engine builds a Plan and never touches the filesystem: applying
// the edits and running any FileOp belong to the host. The same Plan
// then drives the CLI (a git mv or an os.Rename), the LSP (the editor
// renames the file), and the WASM host (the vault API) without the
// engine shelling out — which is impossible under GOOS=js GOARCH=wasm
// anyway.
type Plan struct {
	// Edits maps an output-target key to the edits applied to it. It is
	// non-nil for a successful plan and may be empty on a no-op.
	Edits map[string][]Edit
	// FileOp is the file relocation the host runs, or nil when the
	// operation changes only in-file symbols (a heading or label rename).
	FileOp *FileOp
}

// FileOp is a described file relocation: move the file at From to To,
// both workspace-relative. The host decides how — a git mv when the
// file is tracked, a plain filesystem rename otherwise — because the
// engine cannot run a subprocess under wasm.
type FileOp struct {
	From string
	To   string
}
