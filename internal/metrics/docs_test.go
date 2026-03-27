package metrics

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestListDocs_SortedByID(t *testing.T) {
	docs, err := ListDocs()
	require.NoError(t, err, "ListDocs: %v", err)
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
	require.NoError(t, err, "LookupDoc(MET001): %v", err)
	require.Contains(t, content, "bytes", "expected bytes content, got: %s", content)

	content, err = LookupDoc("bytes")
	require.NoError(t, err, "LookupDoc(bytes): %v", err)
	require.Contains(t, content, "MET001", "expected MET001 content, got: %s", content)
}

func TestLookupDoc_Unknown(t *testing.T) {
	_, err := LookupDoc("MET999")
	require.Error(t, err, "expected unknown metric error")
	require.Contains(t, err.Error(), "unknown metric", "error = %q, want unknown metric", err.Error())
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
	require.NoError(t, err, "listDocsFromFS: %v", err)
	require.Len(t, docs, 1, "len = %d, want 1", len(docs))
	if docs[0].ID != "MET999" {
		t.Fatalf("id = %q, want MET999", docs[0].ID)
	}
}
