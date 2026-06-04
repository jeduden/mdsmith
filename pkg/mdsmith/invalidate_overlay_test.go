package mdsmith

import (
	"sync"
	"testing"
)

// recordingWorkspace is a Workspace that also implements Set/Delete so
// the footgun-3 test can prove Invalidate routes open-document content
// through the Workspace interface, not a hardcoded *MemWorkspace type
// assertion. It delegates reads to an embedded MemWorkspace.
type recordingWorkspace struct {
	*MemWorkspace
	mu      sync.Mutex
	sets    map[string][]byte
	deletes []string
}

func newRecordingWorkspace(files map[string][]byte) *recordingWorkspace {
	return &recordingWorkspace{MemWorkspace: NewMemWorkspace(files), sets: map[string][]byte{}}
}

func (w *recordingWorkspace) Set(p string, data []byte) {
	w.mu.Lock()
	w.sets[p] = append([]byte(nil), data...)
	w.mu.Unlock()
	w.MemWorkspace.Set(p, data)
}

func (w *recordingWorkspace) Delete(p string) {
	w.mu.Lock()
	w.deletes = append(w.deletes, p)
	w.mu.Unlock()
	w.MemWorkspace.Delete(p)
}

// TestInvalidateRoutesContentThroughInterface proves footgun 3 is
// resolved generically: a Workspace implementing Set/Delete (not the
// concrete *MemWorkspace) receives the open-document content on
// Invalidate, so an overlay workspace's buffer bytes reach cross-file
// rules.
func TestInvalidateRoutesContentThroughInterface(t *testing.T) {
	ws := newRecordingWorkspace(nil)
	s, err := NewSession(SessionOptions{Workspace: ws, Config: ConfigYAML("")})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	s.Invalidate("docs/a.md", []byte("buffer bytes"))
	ws.mu.Lock()
	got, ok := ws.sets["docs/a.md"]
	ws.mu.Unlock()
	if !ok {
		t.Fatal("Invalidate did not call Set on the Workspace interface")
	}
	if string(got) != "buffer bytes" {
		t.Fatalf("Set received %q, want %q", got, "buffer bytes")
	}

	s.Invalidate("docs/a.md")
	ws.mu.Lock()
	deleted := len(ws.deletes) == 1 && ws.deletes[0] == "docs/a.md"
	ws.mu.Unlock()
	if !deleted {
		t.Fatalf("no-content Invalidate did not call Delete: deletes=%v", ws.deletes)
	}
}

// TestInvalidateDropsVersionParseCache verifies Invalidate drops the
// version-keyed parse cache for the changed path, so a CheckVersion at
// the same version after an external change re-parses instead of serving
// a stale *lint.File.
func TestInvalidateDropsVersionParseCache(t *testing.T) {
	s := newTestSession(t, "", nil)
	src := []byte("# T\n\nBody paragraph here.\n")

	s.CheckVersion("a.md", src, 1)
	hits := s.parseCacheHits()
	s.CheckVersion("a.md", src, 1)
	if s.parseCacheHits() <= hits {
		t.Fatal("expected a parse-cache hit before invalidation")
	}

	s.Invalidate("a.md", []byte("# T\n\nDifferent body now.\n"))

	hitsAfter := s.parseCacheHits()
	s.CheckVersion("a.md", src, 1)
	if s.parseCacheHits() != hitsAfter {
		t.Fatal("CheckVersion served the version parse cache after Invalidate dropped it")
	}
}
