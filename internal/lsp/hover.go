package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/yuin/goldmark/ast"

	directives "github.com/jeduden/mdsmith/docs/guides/directives"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules"
)

// directiveToDocFile maps directive names to the guide file that documents them.
// Only directives with a dedicated section in the mapped guide are listed here;
// toc (MDS038) and ignore have no guide page yet and are omitted.
var directiveToDocFile = map[string]string{
	"catalog":             "generating-content.md",
	"include":             "generating-content.md",
	"build":               "build.md",
	"allow-empty-section": "enforcing-structure.md",
	"require":             "enforcing-structure.md",
}

// ruleInfoCache holds pre-loaded rule metadata, built once on first use.
// Content is pre-stripped of front matter so hover does not redo the work
// per request.
var ruleInfoCache struct {
	sync.Once
	infos map[string]rules.RuleInfo // uppercase rule ID → RuleInfo with stripped Content
}

// directiveDocCache holds parsed directive doc content, loaded once.
var directiveDocCache struct {
	sync.Once
	docs map[string]string // filename → front-matter-stripped content
}

// directiveDocFor returns the documentation body for the named directive,
// or ("", false) when no documentation file is registered for that name.
func directiveDocFor(name string) (string, bool) {
	filename, ok := directiveToDocFile[name]
	if !ok {
		return "", false
	}
	directiveDocCache.Do(func() {
		directiveDocCache.docs = loadDirectiveDocs(directives.FS)
	})
	content, ok := directiveDocCache.docs[filename]
	return content, ok && content != ""
}

// loadDirectiveDocs reads the distinct guide files referenced by
// directiveToDocFile from fsys, strips front matter, and returns the
// resulting filename→content map. Files that cannot be read (e.g. not
// yet embedded) are silently omitted so hover returns null for those
// directives rather than crashing.
func loadDirectiveDocs(fsys fs.FS) map[string]string {
	docs := make(map[string]string)
	seen := make(map[string]bool)
	for _, fname := range directiveToDocFile {
		if seen[fname] {
			continue
		}
		seen[fname] = true
		data, err := fs.ReadFile(fsys, fname)
		if err != nil {
			continue
		}
		docs[fname] = rules.StripFrontMatter(string(data))
	}
	return docs
}

// handleHover resolves a textDocument/hover request. Resolution order:
//  1. If the cursor falls within an active diagnostic range, return the
//     rule's help body prefixed by the diagnostic message.
//  2. If the cursor falls within a directive block, return the directive docs.
//  3. Otherwise return null (no hover).
func (s *Server) handleHover(msg *requestMessage) {
	var p hoverParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid hover params")
		return
	}
	doc, ok := s.docs.get(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, nil)
		return
	}
	pos := p.Position

	// Diagnostic-first: check the cached diagnostics for this document.
	s.diagsMu.RLock()
	diags := s.diags[p.TextDocument.URI]
	s.diagsMu.RUnlock()

	for _, d := range diags {
		if !posInRange(pos, d.Range) {
			continue
		}
		if d.Code == "" {
			continue
		}
		content := ruleHoverContent(d)
		_ = s.t.writeResponse(msg.ID, hoverResult{
			Contents: markupContent{Kind: "markdown", Value: content},
			Range:    &d.Range,
		})
		return
	}

	// Directive fallback: check if cursor is within a directive block.
	if result := directiveHoverAt(doc.text, pos); result != nil {
		_ = s.t.writeResponse(msg.ID, *result)
		return
	}

	// No match.
	_ = s.t.writeResponse(msg.ID, nil)
}

// ruleHoverContent builds the hover body for a diagnostic: the diagnostic
// message on its own line, a blank line, then the rule's help text.
// Unknown rules get a brief fallback pointing at `mdsmith help rule`.
func ruleHoverContent(d Diagnostic) string {
	info, ok := cachedRuleInfo(d.Code)
	if !ok {
		return fmt.Sprintf("**%s** %s\n\nSee `mdsmith help rule %s` for details.", d.Code, d.Message, d.Code)
	}
	body := fmt.Sprintf("**%s** %s\n\n%s", d.Code, d.Message, info.Content)
	if info.Maintainability != nil && info.Maintainability.ForDiagnostic {
		body += fmt.Sprintf("\n\nSuggested remediation: %s", info.Maintainability.Fix)
	}
	return body
}

// cachedRuleInfo returns the rule metadata for a given diagnostic code, or
// (zero, false) when not found. Content is already stripped of front matter.
// The first call loads all embedded rule READMEs; later calls are O(1) lookups.
// rules.ListRules can only fail if the embedded FS itself is corrupt, in
// which case the cache stays empty and every lookup returns false — the same
// behavior as an unknown code.
func cachedRuleInfo(code string) (rules.RuleInfo, bool) {
	ruleInfoCache.Do(func() {
		all, _ := rules.ListRules()
		m := make(map[string]rules.RuleInfo, len(all))
		for _, r := range all {
			r.Content = rules.StripFrontMatter(r.Content)
			m[strings.ToUpper(r.ID)] = r
		}
		ruleInfoCache.infos = m
	})
	info, ok := ruleInfoCache.infos[strings.ToUpper(code)]
	return info, ok
}

// directiveHoverAt checks whether pos falls within a processing-instruction
// block in source. Returns a *hoverResult when a documented directive is
// found at the cursor, nil otherwise.
func directiveHoverAt(source []byte, pos Position) *hoverResult {
	// Skip the full parse when the document contains no PIs at all.
	if !bytes.Contains(source, []byte("<?")) {
		return nil
	}
	f, _ := lint.NewFile("hover", source)
	lines := splitLines(source)
	var result *hoverResult
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || result != nil {
			return ast.WalkContinue, nil
		}
		pi, ok := n.(*lint.ProcessingInstruction)
		if !ok {
			return ast.WalkContinue, nil
		}

		openSeg := pi.Lines().At(0)
		openContent := source[openSeg.Start:openSeg.Stop]
		// startChar is the column of <? (indent may be 0–3 spaces per grammar).
		startChar := len(openContent) - len(bytes.TrimLeft(openContent, " "))
		startLine := f.LineOfOffset(openSeg.Start) - 1 // 0-based

		var endLine int
		if pi.HasClosure() {
			endLine = f.LineOfOffset(pi.ClosureLine.Start) - 1
		} else {
			endLine = f.LineOfOffset(pi.Lines().At(pi.Lines().Len()-1).Start) - 1
		}

		if pos.Line < startLine || pos.Line > endLine {
			return ast.WalkContinue, nil
		}
		// On the opening line the cursor must be at or after the <? column.
		if pos.Line == startLine && pos.Character < startChar {
			return ast.WalkContinue, nil
		}
		// For single-line PIs, skip cursor positions past the ?> close.
		closeIdx := -1
		if startLine == endLine {
			closeIdx = bytes.Index(openContent, []byte("?>"))
			if closeIdx >= 0 && pos.Character >= utf16Length(openContent[:closeIdx+2]) {
				return ast.WalkContinue, nil
			}
		}

		docContent, ok := directiveDocFor(pi.Name)
		if !ok {
			// Cursor is inside the block but no docs registered; stop
			// searching (no point checking other nodes at the same position).
			return ast.WalkStop, nil
		}

		endChar := utf16Length(currentLineBytes(lines, endLine+1))
		if startLine == endLine && closeIdx >= 0 {
			endChar = utf16Length(openContent[:closeIdx+2])
		}
		hoverRange := Range{
			Start: Position{Line: startLine, Character: startChar},
			End:   Position{Line: endLine, Character: endChar},
		}
		result = &hoverResult{
			Contents: markupContent{Kind: "markdown", Value: docContent},
			Range:    &hoverRange,
		}
		return ast.WalkStop, nil
	})
	return result
}

// posInRange reports whether pos falls within LSP range r. Range.End is
// exclusive per the LSP spec, so a position exactly at End is outside.
func posInRange(pos Position, r Range) bool {
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}
	if pos.Line == r.Start.Line && pos.Character < r.Start.Character {
		return false
	}
	if pos.Line == r.End.Line && pos.Character >= r.End.Character {
		return false
	}
	return true
}
