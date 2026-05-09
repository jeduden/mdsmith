package lsp

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sync"

	"github.com/yuin/goldmark/ast"

	directives "github.com/jeduden/mdsmith/docs/guides/directives"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules"
)

// directiveToDocFile maps directive names to the guide file that documents them.
var directiveToDocFile = map[string]string{
	"catalog":             "generating-content.md",
	"include":             "generating-content.md",
	"toc":                 "generating-content.md",
	"build":               "build.md",
	"allow-empty-section": "enforcing-structure.md",
	"require":             "enforcing-structure.md",
	"ignore":              "enforcing-structure.md",
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
		directiveDocCache.docs = make(map[string]string)
		entries, err := fs.ReadDir(directives.FS, ".")
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := fs.ReadFile(directives.FS, e.Name())
			if err != nil {
				continue
			}
			directiveDocCache.docs[e.Name()] = rules.StripFrontMatter(string(data))
		}
	})
	content, ok := directiveDocCache.docs[filename]
	return content, ok && content != ""
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
	docs, err := rules.LookupRule(d.Code)
	if err != nil {
		return fmt.Sprintf("**%s** %s\n\nSee `mdsmith help rule %s` for details.", d.Code, d.Message, d.Code)
	}
	return fmt.Sprintf("**%s** %s\n\n%s", d.Code, d.Message, docs)
}

// directiveHoverAt checks whether pos falls within a processing-instruction
// block in source. Returns a *hoverResult when a documented directive is
// found at the cursor, nil otherwise.
func directiveHoverAt(source []byte, pos Position) *hoverResult {
	f, err := lint.NewFile("hover", source)
	if err != nil {
		return nil
	}
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
		if pi.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}

		startLine := f.LineOfOffset(pi.Lines().At(0).Start) - 1 // 0-based

		var endLine int
		if pi.HasClosure() {
			endLine = f.LineOfOffset(pi.ClosureLine.Start) - 1
		} else {
			last := pi.Lines().Len() - 1
			endLine = f.LineOfOffset(pi.Lines().At(last).Start) - 1
		}

		if pos.Line < startLine || pos.Line > endLine {
			return ast.WalkContinue, nil
		}

		docContent, ok := directiveDocFor(pi.Name)
		if !ok {
			// Cursor is inside the block but no docs registered; stop
			// searching (no point checking other nodes at the same position).
			return ast.WalkStop, nil
		}

		hoverRange := Range{
			Start: Position{Line: startLine, Character: 0},
			End:   Position{Line: endLine, Character: utf16Length(currentLineBytes(lines, endLine+1))},
		}
		result = &hoverResult{
			Contents: markupContent{Kind: "markdown", Value: docContent},
			Range:    &hoverRange,
		}
		return ast.WalkStop, nil
	})
	return result
}

// posInRange reports whether pos falls within LSP range r (end-exclusive).
func posInRange(pos Position, r Range) bool {
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}
	if pos.Line == r.Start.Line && pos.Character < r.Start.Character {
		return false
	}
	if pos.Line == r.End.Line && pos.Character > r.End.Character {
		return false
	}
	return true
}
