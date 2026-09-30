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
// findByPath tries matches in URI order, so the test closes
// file:///a.md, the smallest URI, from inside the first match call:
// the document goes away after the snapshot and before get() runs,
// simulating a concurrent close in that exact window. get() then
// misses a.md on every run, whatever order the map yields, and
// findByPath must fall through to file:///b.md.
func TestFindByPathContinuesAfterMatchedCandidateCloses(t *testing.T) {
	s := newDocumentStore()
	s.set("file:///a.md", &document{uri: "file:///a.md", path: "/a.md", text: []byte("a")})
	s.set("file:///b.md", &document{uri: "file:///b.md", path: "/b.md", text: []byte("b")})

	closed := false
	uri, doc, ok := s.findByPath(func(string) bool {
		if !closed {
			closed = true
			s.delete("file:///a.md")
		}
		return true
	})
	if !ok {
		t.Fatal("findByPath reported a miss even though file:///b.md was still open")
	}
	if uri != "file:///b.md" || doc == nil || string(doc.text) != "b" {
		t.Fatalf("findByPath = (%q, %v), want file:///b.md after file:///a.md closed mid-scan",
			uri, doc)
	}
}

// TestFindByPathPicksSmallestMatchingURI pins findByPath's winner when
// several open URIs map to one path, for example one file opened under
// two URI spellings on a case-insensitive filesystem. Map iteration
// order is random, so without an explicit rule the winner could change
// from call to call and a rename could split one file's edits across
// two URI keys. The smallest matching URI wins, every call.
func TestFindByPathPicksSmallestMatchingURI(t *testing.T) {
	s := newDocumentStore()
	for _, uri := range []string{"file:///ws/doc.md", "file:///ws/Doc.md", "file:///ws/DOC.md"} {
		s.set(uri, &document{uri: uri, path: "/ws/doc.md", text: []byte(uri)})
	}
	s.set("file:///ws/other.md", &document{uri: "file:///ws/other.md", path: "/ws/other.md"})

	for i := 0; i < 50; i++ {
		uri, doc, ok := s.findByPath(func(path string) bool { return path == "/ws/doc.md" })
		if !ok || uri != "file:///ws/DOC.md" || string(doc.text) != uri {
			t.Fatalf("call %d: findByPath = (%q, %v), want the smallest matching URI file:///ws/DOC.md",
				i, uri, ok)
		}
	}
}
