package lsp

import "testing"

// TestFindByPathMatchRunsOutsideLock pins a review finding on findByPath:
// the match callback is caller-supplied and arbitrary, so running it
// while holding the store's RLock would block a concurrent set()/delete()
// for however long match takes. findByPath snapshots (uri, path) pairs
// under the lock and evaluates match only after releasing it. sync.Mutex
// (embedded in sync.RWMutex) is not reentrant: TryLock from the same
// goroutine that still holds an RLock fails, so this detects the lock
// being held during match without any concurrency or timing dependence.
func TestFindByPathMatchRunsOutsideLock(t *testing.T) {
	s := newDocumentStore()
	s.set("file:///a.md", &document{uri: "file:///a.md", path: "/a.md", text: []byte("x")})

	var lockWasFree bool
	_, _, _ = s.findByPath(func(path string) bool {
		if ok := s.mu.TryLock(); ok {
			s.mu.Unlock()
			lockWasFree = true
		}
		return false
	})
	if !lockWasFree {
		t.Fatal("match ran while findByPath held the store's lock")
	}
}

// TestFindByPathMatch covers the hit path: a matching path returns the
// document's URI and current text.
func TestFindByPathMatch(t *testing.T) {
	s := newDocumentStore()
	s.set("file:///a.md", &document{uri: "file:///a.md", path: "/a.md", text: []byte("hello")})
	s.set("file:///b.md", &document{uri: "file:///b.md", path: "/b.md", text: []byte("world")})

	uri, doc, ok := s.findByPath(func(path string) bool { return path == "/b.md" })
	if !ok || uri != "file:///b.md" || string(doc.text) != "world" {
		t.Fatalf("findByPath match = (%q, %v, %v), want (file:///b.md, world, true)", uri, doc, ok)
	}
}

// TestFindByPathNoMatch covers the miss path: no open document's path
// satisfies match.
func TestFindByPathNoMatch(t *testing.T) {
	s := newDocumentStore()
	s.set("file:///a.md", &document{uri: "file:///a.md", path: "/a.md", text: []byte("x")})

	_, _, ok := s.findByPath(func(path string) bool { return false })
	if ok {
		t.Fatal("findByPath matched with an always-false predicate")
	}
}
