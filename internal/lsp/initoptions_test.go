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
