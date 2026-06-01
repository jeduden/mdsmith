package lsp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
)

// TestPreviewFixRealHandshake exercises the full live flow that VS Code
// performs, rather than assigning s.settings / s.clientCaps directly the
// way newPreviewServer does. It guards the integration seams the
// synthetic-state tests cannot see: parsing the real initialize
// capabilities, reading previewFix off a real workspace/configuration
// response, and producing the annotated edit from those — so a JSON-tag
// typo or a gate regression in handleInitialized would fail here even
// though the unit tests stay green.
func TestPreviewFixRealHandshake(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})

	// 1) The exact capabilities vscode-languageclient@9 advertises.
	initJSON := `{
	  "capabilities": {
	    "workspace": {
	      "configuration": true,
	      "workspaceEdit": {
	        "documentChanges": true,
	        "resourceOperations": ["create","rename","delete"],
	        "failureHandling": "textOnlyTransactional",
	        "normalizesLineEndings": true,
	        "changeAnnotationSupport": { "groupsOnLabel": true }
	      }
	    }
	  }
	}`
	s.handleInitialize(&requestMessage{ID: json.RawMessage(`1`), Params: json.RawMessage(initJSON)})
	assert.True(t, s.clientSupportsAnnotatedEdits(),
		"real initialize capabilities must be parsed as annotated-edit support")

	// 2) A real workspace/configuration response carrying previewFix.
	done := make(chan struct{})
	go func() { defer close(done); s.fetchClientSettings(context.Background()) }()
	deliverPendingResponse(t, s,
		json.RawMessage(`[{"config":"","run":"onSave","path":"","fixOnSave":false,"previewFix":true}]`), nil)
	awaitDone(t, done)
	s.settingsMu.RLock()
	pv := s.settings.PreviewFix
	s.settingsMu.RUnlock()
	assert.True(t, pv, "previewFix:true from the config pull must reach s.settings")

	// 3) The two together must drive the annotated edit path.
	require.True(t, s.useAnnotatedEdits())
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "x.md", text: []byte("# Hi\n\ndirty   \n")}
	actions := s.computeCodeActions(codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///x.md"},
		Context:      codeActionContext{Only: []string{kindSourceFixAll}},
	}, doc, cfg, "")
	require.Len(t, actions, 1)
	edit := actions[0].Edit
	require.NotNil(t, edit)
	assert.Empty(t, edit.Changes, "annotated path must not emit the legacy changes map")
	require.NotEmpty(t, edit.DocumentChanges, "must emit the documentChanges form")
	ann, ok := edit.ChangeAnnotations["mdsmith-fix-all"]
	require.True(t, ok, "changeAnnotations must contain mdsmith-fix-all")
	assert.True(t, ann.NeedsConfirmation, "annotation must be flagged needsConfirmation")
}
