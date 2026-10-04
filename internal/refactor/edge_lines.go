package refactor

import "github.com/jeduden/mdsmith/internal/index"

// edgeLines reads the row an incoming edge points at, resolving and
// splitting each source file once while consecutive edges share it.
// The index returns each file's edges together (the IncomingWikilink*
// lookups sort by SourceFile; IncomingEdges groups by file without
// sorting),
// so a file with many links is read once rather than once per link.
// With memo set, every file read is kept there too, so passes that
// share one reader (the wikilink passes of a MoveAll batch) read a
// file once between them.
type edgeLines struct {
	ws   Workspace
	memo map[string]edgeFile // every file read, when non-nil
	file string
	cur  edgeFile
	seen bool
}

// edgeFile is one resolved source file: its edit key, its lines, and
// whether it was readable.
type edgeFile struct {
	key   string
	lines [][]byte
	ok    bool
}

// row returns the key e's file groups its edits under and the line e
// points at. ok is false when the file is unreadable or the line is
// out of range: the index can hold stale entries after a closed-buffer
// edit or an unprocessed watcher event, and indexing past EOF would
// panic.
func (r *edgeLines) row(e index.Edge) (key string, row []byte, ok bool) {
	if !r.seen || r.file != e.SourceFile {
		f, hit := r.memo[e.SourceFile]
		if !hit {
			var source []byte
			f.key, source, f.ok = r.ws.Resolve(e.SourceFile)
			if f.ok {
				f.lines = splitLines(source)
			}
			if r.memo != nil {
				r.memo[e.SourceFile] = f
			}
		}
		r.cur, r.file, r.seen = f, e.SourceFile, true
	}
	if !r.cur.ok || e.SourceLine < 1 || e.SourceLine > len(r.cur.lines) {
		return "", nil, false
	}
	return r.cur.key, r.cur.lines[e.SourceLine-1], true
}
