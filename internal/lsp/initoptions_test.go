package lsp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitializeParamsSingletonScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		params string
		want   string
	}{
		{"present", `{"initializationOptions":{"mdsmith":{"singletonScope":"abc-123"}}}`, "abc-123"},
		{"absent", `{}`, ""},
		{"null options", `{"initializationOptions":null}`, ""},
		{"non-object options", `{"initializationOptions":"vscode"}`, ""},
		{"array options", `{"initializationOptions":[1,2]}`, ""},
		{"no mdsmith namespace", `{"initializationOptions":{"other":{"singletonScope":"x"}}}`, ""},
		{"null mdsmith namespace", `{"initializationOptions":{"mdsmith":null}}`, ""},
		{"non-object mdsmith namespace", `{"initializationOptions":{"mdsmith":7}}`, ""},
		{"null scope", `{"initializationOptions":{"mdsmith":{"singletonScope":null}}}`, ""},
		{"non-string scope", `{"initializationOptions":{"mdsmith":{"singletonScope":42}}}`, ""},
		{"top-level scope is ignored", `{"initializationOptions":{"singletonScope":"x"}}`, ""},
		{"namespace key is case-sensitive", `{"initializationOptions":{"MDSMITH":{"singletonScope":"x"}}}`, ""},
		{"scope key is case-sensitive", `{"initializationOptions":{"mdsmith":{"SingletonScope":"x"}}}`, ""},
		{"case-variant namespace sibling is ignored",
			`{"initializationOptions":{"mdsmith":{"singletonScope":"x"},"Mdsmith":7}}`, "x"},
		{"case-variant scope sibling is ignored",
			`{"initializationOptions":{"mdsmith":{"singletonScope":"x","singletonscope":7}}}`, "x"},
		{"scope with NUL byte opts out", `{"initializationOptions":{"mdsmith":{"singletonScope":"a\u0000b"}}}`, ""},
		// encoding/json folds case for struct fields, so the top-level
		// key needs the same exact match as the nested ones.
		{"options key is case-sensitive",
			`{"InitializationOptions":{"mdsmith":{"singletonScope":"x"}}}`, ""},
		{"case-variant options sibling is ignored",
			`{"initializationOptions":{"mdsmith":{"singletonScope":"x"}},"INITIALIZATIONOPTIONS":7}`, "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var p initializeParams
			require.NoError(t, json.Unmarshal([]byte(tc.params), &p),
				"a malformed initializationOptions must not fail the initialize decode")
			assert.Equal(t, tc.want, p.singletonScope())
		})
	}
}

// The exact-key re-read must leave the rest of the decode intact: the
// other fields still fill in, and a field of the wrong type still fails
// the whole initialize decode as it did before UnmarshalJSON existed.
func TestInitializeParamsUnmarshalJSON(t *testing.T) {
	t.Parallel()
	var p initializeParams
	require.NoError(t, json.Unmarshal([]byte(`{"processId":7,"rootUri":"file:///r",`+
		`"workspaceFolders":[{"uri":"file:///w","name":"w"}],`+
		`"initializationOptions":{"k":1}}`), &p))
	require.NotNil(t, p.ProcessID)
	assert.Equal(t, 7, *p.ProcessID)
	require.NotNil(t, p.RootURI)
	assert.Equal(t, "file:///r", *p.RootURI)
	require.Len(t, p.WorkspaceFolders, 1)
	assert.Equal(t, "file:///w", p.WorkspaceFolders[0].URI)
	assert.JSONEq(t, `{"k":1}`, string(p.InitializationOptions))

	var bad initializeParams
	assert.Error(t, json.Unmarshal([]byte(`{"processId":"seven"}`), &bad),
		"a wrongly typed field must still fail the decode")

	var null initializeParams
	require.NoError(t, json.Unmarshal([]byte(`null`), &null))
	assert.Nil(t, null.InitializationOptions)

	var arr initializeParams
	assert.Error(t, json.Unmarshal([]byte(`[]`), &arr),
		"a non-object params value must still fail the decode")
}

// The decode is one exact-key pass over the members, so every
// top-level key matches as the LSP spec spells it: a case variant is an
// unknown member and is skipped, as initializationOptions already was.
// Unknown members of any shape are skipped without failing the decode.
func TestInitializeParamsUnmarshalJSONExactKeys(t *testing.T) {
	t.Parallel()
	var p initializeParams
	require.NoError(t, json.Unmarshal([]byte(`{"ProcessId":7,"ROOTURI":"file:///r",`+
		`"extra":{"nested":[1,{"a":null}]},"trace":"off",`+
		`"capabilities":{"workspace":{"workspaceEdit":{"documentChanges":true}}}}`), &p))
	assert.Nil(t, p.ProcessID, "a case-variant processId is an unknown member")
	assert.Nil(t, p.RootURI, "a case-variant rootUri is an unknown member")
	require.NotNil(t, p.Capabilities.Workspace)
	require.NotNil(t, p.Capabilities.Workspace.WorkspaceEdit)
	assert.True(t, p.Capabilities.Workspace.WorkspaceEdit.DocumentChanges)
}
