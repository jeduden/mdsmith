package lsp

import (
	"encoding/json"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
)

// textDocument/codeAction handling: the request entry point, the
// quick-fix and source.fixAll.mdsmith builders, the previewFix
// capability gate, and the WorkspaceEdit construction helpers (full-file
// and annotated-hunk forms). Split out of server.go so the code-action
// dispatch group owns its own file.

func (s *Server) handleCodeAction(msg *requestMessage) {
	var p codeActionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid codeAction params")
		return
	}
	doc, ok := s.docs.get(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, []codeAction{})
		return
	}
	cfg, _, root := s.snapshotConfig()
	if cfg == nil {
		cfg = config.Merge(config.Defaults(), nil)
	}
	// Mirror `mdsmith fix`'s on-disk behavior: skip every code
	// action when the document is in the project ignore list.
	// VS Code's editor.codeActionsOnSave can fire `source.fixAll`
	// even on files that never produced diagnostics, so without
	// this guard an ignored buffer would still be rewritten.
	if config.IsIgnored(cfg.Ignore, workspaceRelative(root, doc.path)) {
		_ = s.t.writeResponse(msg.ID, []codeAction{})
		return
	}
	actions := s.computeCodeActions(p, doc, cfg, root)
	_ = s.t.writeResponse(msg.ID, actions)
}

// clientSupportsAnnotatedEdits reports whether the client advertised
// both documentChanges and changeAnnotationSupport in its initialize
// capabilities. Both must be present for the server to use the
// AnnotatedTextEdit / changeAnnotations wire shape.
func (s *Server) clientSupportsAnnotatedEdits() bool {
	s.clientCapsMu.RLock()
	defer s.clientCapsMu.RUnlock()
	caps := s.clientCaps
	if caps.Workspace == nil || caps.Workspace.WorkspaceEdit == nil {
		return false
	}
	we := caps.Workspace.WorkspaceEdit
	return we.DocumentChanges && we.ChangeAnnotationSupport != nil
}

// useAnnotatedEdits returns true when the mdsmith.previewFix setting is
// on AND the client advertises the required capabilities. When the
// setting is on but the client lacks support the fallback is logged once
// per session to the client output channel.
func (s *Server) useAnnotatedEdits() bool {
	s.settingsMu.RLock()
	preview := s.settings.PreviewFix
	s.settingsMu.RUnlock()
	if !preview {
		return false
	}
	if s.clientSupportsAnnotatedEdits() {
		return true
	}
	// Log the fallback at most once per session.
	if s.previewFallbackLogged.CompareAndSwap(false, true) {
		s.clientCapsMu.RLock()
		caps := s.clientCaps
		s.clientCapsMu.RUnlock()
		var reason string
		noDocChanges := caps.Workspace == nil ||
			caps.Workspace.WorkspaceEdit == nil ||
			!caps.Workspace.WorkspaceEdit.DocumentChanges
		if noDocChanges {
			reason = "client does not support documentChanges"
		} else {
			reason = "client does not support changeAnnotationSupport"
		}
		msg := "mdsmith: previewFix is on but " + reason + "; falling back to legacy changes form"
		s.logger.Printf("%s", msg)
		_ = s.t.writeNotification("window/logMessage", logMessageParams{
			Type:    messageTypeWarning,
			Message: msg,
		})
	}
	return false
}

// computeCodeActions returns the set of code actions for one
// codeAction request. When `Only` is supplied we short-circuit kinds
// the client did not ask for so we don't run fix passes whose output
// the client will discard.
//
// Per-rule fix passes are deduped within a single request: a file
// with N MDS006 diagnostics issues only one fix.SourceWithRules call,
// not N. The resulting WorkspaceEdit is shared across the
// per-diagnostic actions, since each one would have produced the
// same whole-file edit anyway. This keeps the latency budget bounded
// even on files with many diagnostics from the same rule.
//
// mdsmith.previewFix's Refactor Preview is scoped to the
// source.fixAll.mdsmith action only. Interactive lightbulb quick fixes
// always apply immediately: the user picked one specific fix, so a
// forced confirmation preview is friction — and worse, it stranded the
// edit in a Refactor Preview pane whose Apply control is easy to miss,
// so a second lightbulb click collided with the still-pending preview
// ("Another refactoring is being previewed"). Preview belongs on the
// auto-applied bulk edit (fix-on-save wires source.fixAll.mdsmith
// through editor.codeActionsOnSave); VS Code's lightbulb still offers a
// per-action "Preview" (the chevron / Ctrl+Enter) for a single fix.
func (s *Server) computeCodeActions(
	p codeActionParams, doc *document, cfg *config.Config, root string,
) []codeAction {
	wantQuickFix := wantsKind(p.Context.Only, kindQuickFix)
	wantFixAll := wantsKind(p.Context.Only, kindSourceFixAll)

	actions := make([]codeAction, 0, len(p.Context.Diagnostics)+1)
	if wantQuickFix {
		actions = s.appendQuickFixActions(actions, p, doc, cfg, root)
	}
	if wantFixAll {
		actions = s.appendFixAllAction(actions, p, doc, cfg, root, s.useAnnotatedEdits())
	}
	return actions
}

// appendQuickFixActions builds one codeAction per diagnostic and appends
// them to actions. The underlying fix pass is deduped per rule: only one
// fix.SourceWithRules call fires per distinct rule regardless of how many
// diagnostics it covers. The same *workspaceEdit is shared across all
// actions for the same rule so the fix only runs once even on noisy files.
//
// The edit is always the immediate (legacy changes-map) form, so VS Code
// applies the quick fix the moment the user picks it — mdsmith.previewFix
// does not route interactive quick fixes through the Refactor Preview (see
// computeCodeActions for why).
func (s *Server) appendQuickFixActions(
	actions []codeAction,
	p codeActionParams, doc *document, cfg *config.Config, root string,
) []codeAction {
	ruleEdits := make(map[string]*workspaceEdit)
	for _, d := range p.Context.Diagnostics {
		if d.Data == nil || d.Data.RuleName == "" {
			continue
		}
		rule := d.Data.RuleName
		edit, seen := ruleEdits[rule]
		if !seen {
			fixed := s.quickFixBytesFor(rule, doc, cfg, root)
			if fixed != nil {
				edit = fullFileEdit(p.TextDocument.URI, doc.text, fixed)
			}
			ruleEdits[rule] = edit
		}
		if edit == nil {
			continue
		}
		actions = append(actions, codeAction{
			Title:       quickFixTitle(s.rules, rule),
			Kind:        kindQuickFix,
			Diagnostics: []Diagnostic{d},
			Edit:        edit,
		})
	}
	return actions
}

// appendFixAllAction computes the source.fixAll.mdsmith action and
// appends it to actions when the fix produces a change.
func (s *Server) appendFixAllAction(
	actions []codeAction,
	p codeActionParams, doc *document, cfg *config.Config, root string,
	annotated bool,
) []codeAction {
	// fix.Source's Path is fed to config glob matching (ignore /
	// override / kind-assignment), which works against repo-style
	// relative paths. Pass the workspace-relative form so LSP
	// fixes match `mdsmith fix` on disk, and a SourceFS rooted
	// at the document's real directory so include/catalog rules
	// still resolve neighbour files independent of the process
	// CWD.
	// Fix-all routes through Session.Fix (today's fix.Source) so the LSP
	// and `mdsmith fix` share one entry point. The session reads
	// neighbours through its OverlayWorkspace, so include/catalog rules
	// resolve against the project root and see open-buffer overlays.
	sess, _ := s.currentSession()
	if sess == nil {
		return actions
	}
	res, err := sess.Fix(workspaceRelative(root, doc.path), doc.text)
	if err == nil && res.Changed {
		fixed := []byte(res.Source)
		edit := buildFileEdit(p.TextDocument.URI, doc.text, fixed,
			annotated, "mdsmith-fix-all", titleFixAllMdsmith)
		actions = append(actions, codeAction{
			Title: titleFixAllMdsmith,
			Kind:  kindSourceFixAll,
			Edit:  edit,
		})
	}
	return actions
}

// buildFileEdit constructs a WorkspaceEdit in either the annotated
// (documentChanges + changeAnnotations) or legacy (changes map) shape.
func buildFileEdit(uri string, before, after []byte, annotated bool, id, label string) *workspaceEdit {
	if annotated {
		return fullFileEditAnnotated(uri, before, after, id, label)
	}
	return fullFileEdit(uri, before, after)
}

// quickFixBytesFor returns the fixed document bytes produced by running
// just `rule` over the buffer, or nil if the rule is not fixable or its
// fix is a no-op against the current buffer.
//
// The caller constructs the WorkspaceEdit in the appropriate shape
// (legacy changes map or annotated documentChanges).
func (s *Server) quickFixBytesFor(
	rule string, doc *document, cfg *config.Config, root string,
) []byte {
	if !isFixable(s.rules, rule) {
		return nil
	}
	// Per-rule quick-fix routes through Session.FixRule (today's
	// fix.SourceWithRules): only `rule`'s violations are rewritten, and
	// neighbours resolve through the session's OverlayWorkspace.
	sess, _ := s.currentSession()
	if sess == nil {
		return nil
	}
	res, err := sess.FixRule(workspaceRelative(root, doc.path), doc.text, []string{rule})
	if err != nil || !res.Changed {
		return nil
	}
	return []byte(res.Source)
}

// wantsKind reports whether the client's `Only` filter accepts the
// given action kind. An empty/missing filter means "all kinds wanted",
// matching the LSP spec.
func wantsKind(only []string, kind string) bool {
	if len(only) == 0 {
		return true
	}
	for _, k := range only {
		// LSP allows kind prefixes (e.g. "source" matches
		// "source.fixAll.mdsmith"); follow that convention.
		if k == kind || strings.HasPrefix(kind, k+".") {
			return true
		}
	}
	return false
}

// quickFixTitle returns the lightbulb label for a rule's quick fix. A
// rule implementing rule.QuickFixTitler supplies its own label (e.g.
// MDS012 → "Wrap in angle brackets"); otherwise the generic "Fix all
// <name> with mdsmith" is used. That phrasing signals the action's
// WorkspaceEdit covers every occurrence of the rule, not only the
// diagnostic the user clicked on — see appendQuickFixActions /
// quickFixBytesFor for why the edit is whole-file scoped.
func quickFixTitle(rules []rule.Rule, name string) string {
	for _, r := range rules {
		if r.Name() != name {
			continue
		}
		if t, ok := r.(rule.QuickFixTitler); ok {
			return t.FixTitle()
		}
		break
	}
	return "Fix all " + name + " with mdsmith"
}

// fullFileEdit returns a WorkspaceEdit that replaces the entire
// document with `after`. The replacement range covers `before`
// (the buffer the client currently has): start at {0, 0} and end at
// documentEndPosition(before) — see that function's doc for the
// exact end coordinates. Sizing the range against `before` matches
// the LSP contract — clients apply a TextEdit by replacing the
// named range in the existing document.
func fullFileEdit(uri string, before, after []byte) *workspaceEdit {
	endLine, endChar := documentEndPosition(before)
	return &workspaceEdit{
		Changes: map[string][]textEdit{
			uri: {
				{
					Range: Range{
						Start: Position{Line: 0, Character: 0},
						End:   Position{Line: endLine, Character: endChar},
					},
					NewText: string(after),
				},
			},
		},
	}
}

// fullFileEditAnnotated returns a WorkspaceEdit using the LSP 3.16
// documentChanges + changeAnnotations path. The annotation is flagged
// needsConfirmation: true so VS Code routes the edit through Refactor
// Preview instead of applying it immediately.
//
// The edit body is a slice of per-hunk AnnotatedTextEdits computed by
// a Myers line diff (same algorithm gopls uses). One whole-file
// AnnotatedTextEdit would still apply correctly, but VS Code's
// Refactor Preview pane diffs each TextEdit independently — so a
// single full-file edit renders as "old file → new file" with the
// changed lines lost in a wall of unchanged context, and the lower
// tree-node previews the entire new document on one line. Emitting
// one edit per hunk gives the preview real ranges to highlight and
// short labels to render.
//
// All hunks carry the same annotationID; VS Code groups them under
// one "Fix all <rule>" confirmation entry.
func fullFileEditAnnotated(uri string, before, after []byte, annotationID, label string) *workspaceEdit {
	edits := annotatedHunkEdits(before, after, annotationID)
	return &workspaceEdit{
		DocumentChanges: []textDocumentEdit{
			{
				TextDocument: optionalVersionedTextDocumentIdentifier{URI: uri},
				Edits:        edits,
			},
		},
		ChangeAnnotations: map[string]changeAnnotation{
			annotationID: {
				Label:             label,
				Description:       "Preview before applying",
				NeedsConfirmation: true,
			},
		},
	}
}

// annotatedHunkEdits computes a line-aligned diff between before and
// after and returns one AnnotatedTextEdit per hunk. Myers may emit
// several adjacent raw edits per hunk — e.g. a Delete-per-line for a
// multi-line removal followed by a zero-width Insert for the
// replacement text. Any run of edits where each one's end position
// touches the next one's start gets coalesced into a single Replace
// covering the combined range so the preview pane shows one entry per
// visible change rather than a list of zero-width inserts and empty
// deletes.
//
// Edits are returned bottom-up (last hunk first) so a naive client
// applying them in slice order doesn't shift the offsets a later
// edit relies on — same convention as sortTextEditsBottomUp in
// rename.go. The LSP spec only forbids overlap; it doesn't pin
// application order.
//
// Each edit's range uses character 0 on both endpoints (line-aligned),
// matching the LSP spec for "replace these whole lines": start at the
// beginning of the first changed line, end at the beginning of the
// line immediately after the last changed line.
func annotatedHunkEdits(before, after []byte, annotationID string) []annotatedTextEdit {
	raw := myers.ComputeEdits(span.URIFromPath(""), string(before), string(after))
	gotextdiff.SortTextEdits(raw)
	out := make([]annotatedTextEdit, 0, len(raw))
	for i := 0; i < len(raw); {
		start, end := lineRange(raw[i])
		var newText strings.Builder
		newText.WriteString(raw[i].NewText)
		j := i + 1
		for j < len(raw) {
			nStart, nEnd := lineRange(raw[j])
			if nStart != end {
				break
			}
			end = nEnd
			newText.WriteString(raw[j].NewText)
			j++
		}
		out = append(out, annotatedTextEdit{
			Range:        Range{Start: start, End: end},
			NewText:      newText.String(),
			AnnotationID: annotationID,
		})
		i = j
	}
	// Coalesce runs above relied on top-down order. Reverse to the
	// codebase's bottom-up emit convention.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// lineRange converts a gotextdiff TextEdit (1-indexed line, column 1)
// to an LSP Range (0-indexed line, character 0).
func lineRange(e gotextdiff.TextEdit) (Position, Position) {
	return Position{Line: e.Span.Start().Line() - 1, Character: 0},
		Position{Line: e.Span.End().Line() - 1, Character: 0}
}

// documentEndPosition returns the LSP end position covering the
// entire `source`. The end position is one-past-the-last-character
// in LSP coordinates:
//
//   - Empty input: (0, 0).
//   - Trailing-newline-terminated content (e.g. "abc\n"): the line
//     index equal to the number of newlines, character 0 — i.e. the
//     virtual empty line just past the final \n. For "abc\n" the
//     result is (1, 0); for "abc\ndef\n" it is (2, 0). This matches
//     LSP §3.18 (TextDocumentItem) where a final \n produces a
//     trailing empty line whose position is the file's end.
//   - No trailing newline: the last line's index plus its UTF-16
//     length, e.g. (0, 3) for "abc" or (1, 3) for "abc\ndef".
func documentEndPosition(source []byte) (int, int) {
	if len(source) == 0 {
		return 0, 0
	}
	if source[len(source)-1] == '\n' {
		// Count newlines; the position past the final \n is the
		// one-past-the-end line, character 0.
		nl := 0
		for _, b := range source {
			if b == '\n' {
				nl++
			}
		}
		return nl, 0
	}
	// No trailing newline: end at last line's UTF-16 length. source
	// is non-empty here (checked above), so splitLines always yields
	// at least one element.
	lines := splitLines(source)
	return len(lines) - 1, utf16Length(lines[len(lines)-1])
}
