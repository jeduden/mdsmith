package lsp

import (
	"encoding/json"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/index"
)

// handleCompletion handles textDocument/completion requests. It locates the
// completion context at the cursor, queries the workspace index for matching
// items, and returns a completionList. An empty list (not null) is returned
// when the cursor is not in a completable position.
func (s *Server) handleCompletion(msg *requestMessage) {
	var p completionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid completion params")
		return
	}
	source, rel, ok := s.docTextOrFile(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, completionList{Items: []completionItem{}})
		return
	}

	line := p.Position.Line + 1
	col := lspPositionToByteColumn(source, line, p.Position.Character)
	ctx := index.Locator{Path: rel}.CompletionContext(source, line, col)
	if ctx.Tag == index.CompletionNone {
		_ = s.t.writeResponse(msg.ID, completionList{IsIncomplete: false, Items: []completionItem{}})
		return
	}
	var idx *index.Index
	if ctx.Tag != index.CompletionKindValue {
		idx = s.ensureIndex()
	}

	items := s.completionItems(ctx, rel, idx)
	_ = s.t.writeResponse(msg.ID, completionList{IsIncomplete: false, Items: items})
}

// completionItems dispatches on ctx.Tag and returns the matching items.
func (s *Server) completionItems(ctx index.CompletionContext, rel string, idx *index.Index) []completionItem {
	switch ctx.Tag {
	case index.CompletionAnchorCurrentFile:
		return s.anchorItems(rel, ctx.Prefix, idx, true)
	case index.CompletionAnchorOtherFile:
		if ctx.TargetFile == "" {
			return []completionItem{}
		}
		return s.anchorItems(ctx.TargetFile, ctx.Prefix, idx, false)
	case index.CompletionRefLabel:
		return s.refLabelItems(rel, ctx.Prefix, idx)
	case index.CompletionKindValue:
		return s.kindItems(ctx.Prefix)
	case index.CompletionDirectivePath:
		return s.directivePathItems(rel, ctx.Prefix, idx)
	}
	return []completionItem{}
}

// anchorItems returns completion items for heading anchors in the given file.
// sameFile controls sort priority: same-file anchors sort lexicographically
// before cross-file anchors so they appear first in the list.
func (s *Server) anchorItems(file, prefix string, idx *index.Index, sameFile bool) []completionItem {
	fe, ok := idx.File(file)
	if !ok {
		return []completionItem{}
	}
	prefixLower := strings.ToLower(prefix)
	var items []completionItem
	for _, sym := range fe.Symbols {
		if sym.Kind != index.SymbolHeading || sym.Anchor == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(sym.Anchor), prefixLower) {
			continue
		}
		sortPfx := "b"
		if sameFile {
			sortPfx = "a"
		}
		items = append(items, completionItem{
			Label:    sym.Anchor,
			Kind:     completionItemKindReference,
			Detail:   file,
			SortText: sortPfx + sym.Anchor,
		})
	}
	sortItems(items)
	return items
}

// refLabelItems returns completion items for link-reference labels defined
// in the given file. CommonMark link refs are file-local, so labels from
// other files are intentionally excluded.
func (s *Server) refLabelItems(file, prefix string, idx *index.Index) []completionItem {
	fe, ok := idx.File(file)
	if !ok {
		return []completionItem{}
	}
	prefixLower := strings.ToLower(prefix)
	var items []completionItem
	for _, sym := range fe.Symbols {
		if sym.Kind != index.SymbolLinkRef {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(sym.Anchor), prefixLower) {
			continue
		}
		items = append(items, completionItem{
			Label:  sym.Anchor,
			Kind:   completionItemKindReference,
			Detail: file,
		})
	}
	sortItems(items)
	return items
}

// kindItems returns completion items for kind names declared in .mdsmith.yml.
func (s *Server) kindItems(prefix string) []completionItem {
	cfg, _, _ := s.snapshotConfig()
	if cfg == nil {
		return []completionItem{}
	}
	prefixLower := strings.ToLower(prefix)
	var items []completionItem
	for k := range cfg.Kinds {
		if !strings.HasPrefix(strings.ToLower(k), prefixLower) {
			continue
		}
		items = append(items, completionItem{
			Label:  k,
			Kind:   completionItemKindEnumMember,
			Detail: ".mdsmith.yml",
		})
	}
	sortItems(items)
	return items
}

// directivePathItems returns workspace Markdown paths (both .md and .markdown)
// matching prefix, expressed relative to the open buffer's directory (matching
// how include/build/catalog directives resolve paths via ResolveRelTarget).
//
// When prefix starts with "!" (glob exclusion syntax), the leading "!" is
// stripped for matching and re-prepended on each returned label so that users
// can complete negation patterns like `- "!docs/internal/`.
func (s *Server) directivePathItems(rel, prefix string, idx *index.Index) []completionItem {
	dir := path.Dir(rel)
	if dir == "." {
		dir = ""
	}
	exclude := strings.HasPrefix(prefix, "!")
	matchPrefix := prefix
	if exclude {
		matchPrefix = prefix[1:]
	}
	prefixLower := strings.ToLower(matchPrefix)
	files := idx.Files()
	var items []completionItem
	for _, f := range files {
		relF := relFromDir(dir, f)
		if !strings.HasPrefix(strings.ToLower(relF), prefixLower) {
			continue
		}
		label := relF
		if exclude {
			label = "!" + relF
		}
		items = append(items, completionItem{
			Label: label,
			Kind:  completionItemKindFile,
		})
	}
	sortItems(items)
	return items
}

// relFromDir returns the path of target relative to dir (both workspace-
// relative, forward-slash separated). When dir is empty, target is returned
// unchanged.
func relFromDir(dir, target string) string {
	if dir == "" {
		return target
	}
	rel, _ := filepath.Rel(filepath.FromSlash(dir), filepath.FromSlash(target))
	return filepath.ToSlash(rel)
}

// sortItems sorts items by SortText then Label.
func sortItems(items []completionItem) {
	sort.Slice(items, func(i, j int) bool {
		si := items[i].SortText
		if si == "" {
			si = items[i].Label
		}
		sj := items[j].SortText
		if sj == "" {
			sj = items[j].Label
		}
		if si != sj {
			return si < sj
		}
		return items[i].Label < items[j].Label
	})
}
