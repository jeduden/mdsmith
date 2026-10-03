package refactor

import "github.com/jeduden/mdsmith/internal/index"

// edgeLines reads the row an incoming edge points at, resolving and
// splitting each source file once while consecutive edges share it.
// The index returns edges sorted by SourceFile, so a file with many
// links is read once rather than once per link.
type edgeLines struct {
	ws    Workspace
	file  string
	key   string
	lines [][]byte
	ok    bool
	seen  bool
}

// row returns the key e's file groups its edits under and the line e
// points at. ok is false when the file is unreadable or the line is
// out of range: the index can hold stale entries after a closed-buffer
// edit or an unprocessed watcher event, and indexing past EOF would
// panic.
func (r *edgeLines) row(e index.Edge) (key string, row []byte, ok bool) {
	if !r.seen || r.file != e.SourceFile {
		var source []byte
		r.key, source, r.ok = r.ws.Resolve(e.SourceFile)
		r.lines = nil
		if r.ok {
			r.lines = splitLines(source)
		}
		r.file, r.seen = e.SourceFile, true
	}
	if !r.ok || e.SourceLine < 1 || e.SourceLine > len(r.lines) {
		return "", nil, false
	}
	return r.key, r.lines[e.SourceLine-1], true
}
