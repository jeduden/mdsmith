package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/rules"
)

// directiveDocMu guards directiveDocCache.
var directiveDocMu sync.Mutex

// directiveDocCache caches directive doc content keyed by
// "<repoRoot>/<name>" after the first successful read.
var directiveDocCache = make(map[string]string)

// lookupDirectiveDoc returns the Markdown body (front matter stripped) for
// the named directive. It searches docs/guides/directives/ relative to
// repoRoot. Results are cached after the first read so repeated hover
// requests on the same document don't re-read the filesystem.
func lookupDirectiveDoc(name, repoRoot string) string {
	if repoRoot == "" {
		return ""
	}
	key := repoRoot + "/" + name
	directiveDocMu.Lock()
	defer directiveDocMu.Unlock()
	if content, ok := directiveDocCache[key]; ok {
		return content
	}
	dir := filepath.Join(repoRoot, "docs", "guides", "directives")
	content := readDirectiveDocFromDir(name, os.DirFS(dir))
	directiveDocCache[key] = content
	return content
}

// readDirectiveDocFromDir searches an fs.FS (rooted at the directives
// directory) for documentation about the named directive. It first tries
// <name>.md, then scans every .md file for a reference to the directive.
func readDirectiveDocFromDir(name string, fsys fs.FS) string {
	// Exact match: <name>.md
	if data, err := fs.ReadFile(fsys, name+".md"); err == nil {
		return stripDocFrontMatter(string(data))
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			continue
		}
		content := string(data)
		if strings.Contains(content, "<?"+name) || strings.Contains(content, "`"+name+"`") {
			return stripDocFrontMatter(content)
		}
	}
	return ""
}

// stripDocFrontMatter removes the leading YAML front matter block
// (--- ... ---) from a documentation file and the immediately
// following blank lines.
func stripDocFrontMatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return content
	}
	body := content[4+end+5:]
	return strings.TrimLeft(body, "\n")
}

// handleHover processes a textDocument/hover request.
//
// Resolution order:
//  1. Diagnostic-first: if the cursor is inside any active diagnostic
//     range, return MarkupContent with the diagnostic message + rule docs.
//  2. Directive fallback: if the cursor is inside a <?directive?> block,
//     return the directive docs from docs/guides/directives/.
//  3. Return null when neither matches.
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

	// Diagnostic-first.
	if result := s.hoverFromDiagnostics(p.Position, doc); result != nil {
		_ = s.t.writeResponse(msg.ID, result)
		return
	}

	// Directive fallback.
	_, _, root := s.snapshotConfig()
	if result := s.hoverFromDirective(p.Position, doc, root); result != nil {
		_ = s.t.writeResponse(msg.ID, result)
		return
	}

	_ = s.t.writeResponse(msg.ID, nil)
}

// hoverFromDiagnostics returns a hover result when pos falls inside
// any active diagnostic for doc.
func (s *Server) hoverFromDiagnostics(pos Position, doc *document) *hoverResult {
	cfg, configPath, root := s.snapshotConfig()
	if cfg == nil {
		cfg = config.Merge(config.Defaults(), nil)
	}
	relPath := workspaceRelative(root, doc.path)
	maxBytes := s.resolveMaxInputBytes(cfg)
	r := &engine.Runner{
		Config:           cfg,
		Rules:            s.rules,
		StripFrontMatter: frontMatterEnabled(cfg),
		RootDir:          root,
		MaxInputBytes:    maxBytes,
		SourceFS:         dirFSForPath(doc.path),
		ConfigPath:       configPath,
	}
	res := r.RunSource(relPath, doc.text)
	docDiags, _ := partitionDocDiagnostics(res.Diagnostics, relPath)

	lines := splitLines(doc.text)
	for _, d := range docDiags {
		lspDiag := toLSP(d, lines)
		if posInRange(pos, lspDiag.Range) {
			body := buildRuleHoverBody(d.Message, d.RuleID)
			return &hoverResult{
				Contents: markupContent{Kind: "markdown", Value: body},
				Range:    &lspDiag.Range,
			}
		}
	}
	return nil
}

// hoverFromDirective returns a hover result when pos falls inside a
// <?directive?> block in doc's source.
func (s *Server) hoverFromDirective(pos Position, doc *document, root string) *hoverResult {
	span, name, found := findDirectiveAtPos(doc.text, pos)
	if !found {
		return nil
	}
	content := lookupDirectiveDoc(name, root)
	if content == "" {
		content = fmt.Sprintf("No documentation found for directive `%s`.", name)
	}
	return &hoverResult{
		Contents: markupContent{Kind: "markdown", Value: content},
		Range:    &span,
	}
}

// buildRuleHoverBody constructs the Markdown hover body: the diagnostic
// message, a blank line, then the rule's full documentation. Falls back
// to a "see mdsmith help" line when no docs are found.
func buildRuleHoverBody(message, ruleID string) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString("\n\n")
	docs, err := rules.LookupRule(ruleID)
	if err != nil || strings.TrimSpace(docs) == "" {
		sb.WriteString(fmt.Sprintf("See `mdsmith help rule %s` for details.", ruleID))
	} else {
		sb.WriteString(docs)
	}
	return sb.String()
}

// posInRange reports whether pos is contained within r. A pos at the
// end character of a single-line range is considered outside (exclusive
// end), matching LSP semantics. For multi-line ranges the cursor on the
// last line before end.Character is inside.
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

// directiveBlock is a parsed <?directive?> span in the source.
type directiveBlock struct {
	name      string
	startLine int // 0-based
	endLine   int // 0-based inclusive
}

// findDirectiveAtPos scans source for <?directive?> blocks and returns
// the block's Range, the directive name, and true when pos falls within
// any block. Handles single-line (<?name ... ?>) and multi-line blocks.
func findDirectiveAtPos(source []byte, pos Position) (Range, string, bool) {
	lines := bytes.Split(source, []byte{'\n'})
	blocks := collectDirectiveBlocks(lines)
	for _, b := range blocks {
		if pos.Line < b.startLine || pos.Line > b.endLine {
			continue
		}
		var endLine []byte
		if b.endLine < len(lines) {
			endLine = lines[b.endLine]
		}
		r := Range{
			Start: Position{Line: b.startLine, Character: 0},
			End:   Position{Line: b.endLine, Character: utf16Length(endLine) + 1},
		}
		return r, b.name, true
	}
	return Range{}, "", false
}

// collectDirectiveBlocks scans lines for <?directive?> blocks and
// returns them in document order.
func collectDirectiveBlocks(lines [][]byte) []directiveBlock {
	var blocks []directiveBlock
	inBlock := false
	var cur directiveBlock
	for i, line := range lines {
		trimmed := bytes.TrimLeft(line, " \t")
		if !inBlock {
			b, name, ok := parseDirectiveOpen(trimmed, i)
			if !ok {
				continue
			}
			cur = b
			trimmedRight := bytes.TrimRight(trimmed, " \t\r\n")
			if bytes.Contains(trimmedRight[2:], []byte("?>")) {
				blocks = append(blocks, cur)
				cur = directiveBlock{}
			} else {
				_ = name
				inBlock = true
			}
		} else {
			cur.endLine = i
			trimmedLine := bytes.TrimSpace(line)
			if bytes.HasPrefix(trimmedLine, []byte("?>")) {
				inBlock = false
				blocks = append(blocks, cur)
				cur = directiveBlock{}
			}
		}
	}
	if inBlock {
		blocks = append(blocks, cur)
	}
	return blocks
}

// parseDirectiveOpen tries to parse an opening <?name?> marker from
// trimmed. Returns the initial block, the name, and true on success.
func parseDirectiveOpen(trimmed []byte, lineIdx int) (directiveBlock, string, bool) {
	if !bytes.HasPrefix(trimmed, []byte("<?")) {
		return directiveBlock{}, "", false
	}
	rest := trimmed[2:]
	if len(rest) > 0 && rest[0] == '/' {
		return directiveBlock{}, "", false // closing marker
	}
	name := extractDirectiveName(rest)
	if name == "" {
		return directiveBlock{}, "", false
	}
	return directiveBlock{name: name, startLine: lineIdx, endLine: lineIdx}, name, true
}

// extractDirectiveName returns the directive name from the bytes
// following "<?". The name ends at the first whitespace or "?>".
func extractDirectiveName(b []byte) string {
	b = bytes.TrimRight(b, "\r\n")
	for i, c := range b {
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			return string(b[:i])
		}
		if c == '?' && i+1 < len(b) && b[i+1] == '>' {
			return string(b[:i])
		}
	}
	return string(b)
}
