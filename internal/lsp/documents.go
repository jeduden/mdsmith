package lsp

import "sync"

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

// findByPath returns a copy of the open document whose path satisfies
// match, or (nil, false) if none does. It snapshots (uri, path) pairs
// — cheap, since a string copy shares the underlying bytes — under a
// single read lock, then evaluates match against that snapshot after
// releasing the lock, so a caller-supplied match callback never holds
// the store's lock (and so can never block a concurrent set()/delete()
// for its own duration, however long it runs). This still avoids the
// openURIs()+get() pattern's O(open-docs) full-document copies on a
// miss: at most one extra locked lookup, for the single matching URI.
func (s *documentStore) findByPath(match func(path string) bool) (string, *document, bool) {
	s.mu.RLock()
	type candidate struct{ uri, path string }
	candidates := make([]candidate, 0, len(s.m))
	for uri, d := range s.m {
		candidates = append(candidates, candidate{uri, d.path})
	}
	s.mu.RUnlock()

	for _, c := range candidates {
		if match(c.path) {
			d, ok := s.get(c.uri)
			return c.uri, d, ok
		}
	}
	return "", nil, false
}
