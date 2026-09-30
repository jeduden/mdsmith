package lsp

import (
	"slices"
	"strings"
	"sync"
)

// document is one open buffer in the editor.
type document struct {
	uri     string
	path    string
	text    []byte
	version int
}

// documentStore is a goroutine-safe map of open documents keyed by URI.
type documentStore struct {
	mu sync.RWMutex
	m  map[string]*document
}

func newDocumentStore() *documentStore {
	return &documentStore{m: make(map[string]*document)}
}

// get returns a shallow copy of the stored document. The copy
// prevents callers from racing the stored *document pointer (e.g.
// when set() replaces it). Because set() takes ownership of the
// caller's `text` slice via a deep copy, the bytes returned here
// are safe for concurrent readers as long as no one mutates the
// returned slice in place.
func (s *documentStore) get(uri string) (*document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.m[uri]
	if !ok {
		return nil, false
	}
	cp := *d
	return &cp, true
}

// set stores the document under uri, taking ownership of d.text via
// a deep copy. After set returns, the caller may safely reuse or
// mutate its own copy of d.text — the store will not observe the
// change. Without this copy, a caller that retains and later
// mutates the slice could race with concurrent get() readers.
func (s *documentStore) set(uri string, d *document) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *d
	if d.text != nil {
		cp.text = append([]byte(nil), d.text...)
	}
	s.m[uri] = &cp
}

func (s *documentStore) delete(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, uri)
}

func (s *documentStore) openURIs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.m))
	for k := range s.m {
		out = append(out, k)
	}
	return out
}

// findByPath returns (uri, doc, true) for the open document whose path
// satisfies match, or ("", nil, false) if none does. When several open
// URIs satisfy match (one file opened under two URI spellings, say),
// the smallest URI wins, so repeated calls agree on one URI rather
// than following the map's random iteration order.
//
// It snapshots (uri, path) pairs under a single read lock, then runs
// match against that snapshot after releasing the lock, so a
// caller-supplied match callback never holds the store's lock (and so
// can never block a concurrent set()/delete() for its own duration).
// The snapshot costs one slice allocation per call, sized to the
// number of open documents, on a hit or a miss; its strings share
// their bytes with the stored documents. What it avoids is the
// openURIs()+get() pattern's per-document lock and struct copy: get()
// runs only for a candidate whose path matches.
//
// A document can close in the gap between the snapshot and its
// candidate's get() call, turning a real match into a miss for that
// one candidate; the loop then tries the next-smallest matching URI
// instead of reporting an overall miss, matching the old
// openURIs()+get() loop's behavior. Only if every matching candidate
// closes this way does findByPath itself report a miss.
func (s *documentStore) findByPath(match func(path string) bool) (string, *document, bool) {
	type candidate struct{ uri, path string }
	s.mu.RLock()
	candidates := make([]candidate, 0, len(s.m))
	for uri, d := range s.m {
		candidates = append(candidates, candidate{uri, d.path})
	}
	s.mu.RUnlock()

	// Filter the snapshot down to the matches in place, then order them
	// by URI so the winner never depends on map iteration order.
	matches := candidates[:0]
	for _, c := range candidates {
		if match(c.path) {
			matches = append(matches, c)
		}
	}
	slices.SortFunc(matches, func(a, b candidate) int {
		return strings.Compare(a.uri, b.uri)
	})
	for _, c := range matches {
		if d, ok := s.get(c.uri); ok {
			return c.uri, d, true
		}
	}
	return "", nil, false
}
