package metrics

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestListDocs_SortedByID(t *testing.T) {
	docs, err := ListDocs()
	if err != nil {
		t.Fatalf("ListDocs: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("expected docs")
	}
	for i := 1; i < len(docs); i++ {
		if docs[i].ID < docs[i-1].ID {
			t.Fatalf("docs not sorted: %s after %s", docs[i].ID, docs[i-1].ID)
		}
	}
}

func TestLookupDoc_ByIDAndName(t *testing.T) {
	content, err := LookupDoc("MET001")
	if err != nil {
		t.Fatalf("LookupDoc(MET001): %v", err)
	}
	if !strings.Contains(content, "bytes") {
		t.Fatalf("expected bytes content, got: %s", content)
	}

	content, err = LookupDoc("bytes")
	if err != nil {
		t.Fatalf("LookupDoc(bytes): %v", err)
	}
	if !strings.Contains(content, "MET001") {
		t.Fatalf("expected MET001 content, got: %s", content)
	}
}

func TestLookupDoc_Unknown(t *testing.T) {
	_, err := LookupDoc("MET999")
	if err == nil {
		t.Fatal("expected unknown metric error")
	}
	if !strings.Contains(err.Error(), "unknown metric") {
		t.Fatalf("error = %q, want unknown metric", err.Error())
	}
}

func TestListDocsFromFS_SkipsBadFrontMatter(t *testing.T) {
	fsys := fstest.MapFS{
		"good/README.md": &fstest.MapFile{
			Data: []byte("---\nid: MET999\nname: test\ndescription: Test.\n---\n# MET999\n"),
		},
		"bad/README.md": &fstest.MapFile{
			Data: []byte("# missing front matter\n"),
		},
	}

	docs, err := listDocsFromFS(fsys)
	if err != nil {
		t.Fatalf("listDocsFromFS: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("len = %d, want 1", len(docs))
	}
	if docs[0].ID != "MET999" {
		t.Fatalf("id = %q, want MET999", docs[0].ID)
	}
}
