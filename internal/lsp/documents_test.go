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

// TestFindByPathContinuesAfterMatchedCandidateCloses pins a review
// finding: findByPath snapshots (uri, path) pairs, then evaluates
// match and calls get() afterward, so a document that closes in that
// gap makes get() report a miss for an otherwise-matching candidate.
// The old openURIs()+get() loop would have moved on to the next open
// document in that case; findByPath must do the same instead of
// reporting a miss while another candidate is still open.
//
// The first match invocation (order is unspecified — map iteration)
// deletes its own candidate's document right before findByPath's
// get() call would run, simulating a concurrent close in that exact
// window, then still reports a match. The second candidate is left
// alone and must be the one findByPath returns.
func TestFindByPathContinuesAfterMatchedCandidateCloses(t *testing.T) {
	s := newDocumentStore()
	s.set("file:///a.md", &document{uri: "file:///a.md", path: "/a.md", text: []byte("a")})
	s.set("file:///b.md", &document{uri: "file:///b.md", path: "/b.md", text: []byte("b")})

	first := true
	uri, doc, ok := s.findByPath(func(path string) bool {
		if first {
			first = false
			s.delete("file://" + path)
		}
		return true
	})
	if !ok {
		t.Fatal("findByPath reported a miss even though one candidate was still open")
	}
	if uri == "" || doc == nil {
		t.Fatalf("findByPath returned ok=true with an empty result: uri=%q doc=%v", uri, doc)
	}
}
