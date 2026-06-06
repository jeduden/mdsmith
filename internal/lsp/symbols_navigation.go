package lsp

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
)

// LSP navigation handlers: textDocument/definition, /implementation,
// and /references, plus the target-resolution helpers they share. Split
// out of symbols.go so each LSP dispatch group owns its own file
// (cf. rename.go, completion.go).

// handleDefinition resolves textDocument/definition.
func (s *Server) handleDefinition(msg *requestMessage) {
	var p textDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid definition params")
		return
	}
	locs := s.resolveTargets(p, false)
	if len(locs) == 0 {
		_ = s.t.writeResponse(msg.ID, nil)
		return
	}
	_ = s.t.writeResponse(msg.ID, locs[0])
}

// handleImplementation returns every match. For most tags this is the
// same answer as Definition; only `kind:` values and headings (with
// references) produce multi-target sets.
func (s *Server) handleImplementation(msg *requestMessage) {
	var p textDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid implementation params")
		return
	}
	locs := s.resolveTargets(p, true)
	_ = s.t.writeResponse(msg.ID, locs)
}

// resolveTargets is the shared core for definition and implementation.
// When wantAll is false the slice is truncated to the first match.
func (s *Server) resolveTargets(p textDocumentPositionParams, wantAll bool) []location {
	source, rel, ok := s.docTextOrFile(p.TextDocument.URI)
	if !ok {
		return nil
	}
	idx := s.ensureIndex()

	line := p.Position.Line + 1
	col := lspPositionToByteColumn(source, line, p.Position.Character)
	res := index.Locator{Path: rel}.Locate(source, line, col)
	return s.resolveByTag(p, res, line, source, rel, idx, wantAll)
}

// resolveByTag dispatches on the locator's TokenTag and returns the
// matching navigation targets. Split out of resolveTargets so the
// switch stays small enough for funlen.
func (s *Server) resolveByTag(
	p textDocumentPositionParams, res index.LocateResult,
	line int, source []byte, rel string, idx *index.Index, wantAll bool,
) []location {
	switch res.Tag {
	case index.TokenAnchorLink:
		return s.locationsForAnchor(rel, res.TargetAnchor, idx, source)
	case index.TokenFileLink:
		return s.locationsForFileLink(res.TargetFile, res.TargetAnchor, idx)
	case index.TokenRefUse, index.TokenRefDef:
		if loc, ok := s.locationForRefDef(rel, res.Label, source); ok {
			return []location{loc}
		}
	case index.TokenDirectiveArg:
		return s.directiveArgLocations(rel, res.DirectiveTargetFile)
	case index.TokenHeading:
		return s.headingTargets(p, rel, res.Anchor, line, source, idx, wantAll)
	case index.TokenFileTop:
		return []location{{
			URI:   p.TextDocument.URI,
			Range: rangeAt(1, 1, source),
		}}
	case index.TokenFrontMatterValue:
		return s.frontMatterValueTargets(res.FrontMatterKey, res.FrontMatterValue, idx, wantAll)
	}
	return nil
}

func (s *Server) directiveArgLocations(rel, target string) []location {
	if target == "" {
		return nil
	}
	tgt := linkgraph.ResolveRelTarget(rel, target)
	if tgt == "" {
		return nil
	}
	return []location{{
		URI:   s.workspaceURI(tgt),
		Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
	}}
}

// headingTargets returns the heading itself for definition, plus
// every link to it for implementation.
func (s *Server) headingTargets(
	p textDocumentPositionParams, rel, anchor string,
	line int, source []byte, idx *index.Index, wantAll bool,
) []location {
	decl := []location{{
		URI:   p.TextDocument.URI,
		Range: rangeAt(line, 1, source),
	}}
	if !wantAll {
		return decl
	}
	return append(decl, s.locationsForRefsToHeading(rel, anchor, idx)...)
}

// frontMatterValueTargets handles the `kind:` / `kinds:` value arm:
// definition resolves to the kind block in `.mdsmith.yml`,
// implementation widens to every file with that kind.
func (s *Server) frontMatterValueTargets(key, val string, idx *index.Index, wantAll bool) []location {
	if key != "kind" && key != "kinds" {
		return nil
	}
	defs := s.locationsForKindDefinition(val)
	if !wantAll {
		return defs
	}
	return append(defs, s.locationsForFilesByKind(val, idx)...)
}

// locationsForAnchor returns the in-file heading targeted by an
// anchor reference. It always returns at most one location — the
// matching heading itself; multi-target widening for headings (the
// implementation behavior) lives in resolveTargets' TokenHeading
// arm, where the declaration is paired with all incoming links.
func (s *Server) locationsForAnchor(rel, anchor string, idx *index.Index, source []byte) []location {
	if anchor == "" {
		return nil
	}
	if fe, ok := idx.File(rel); ok {
		for _, sym := range fe.Symbols {
			if sym.Kind == index.SymbolHeading && sym.Anchor == anchor {
				return []location{{
					URI:   s.workspaceURI(rel),
					Range: rangeAt(sym.SelectionLine, sym.SelectionCol, source),
				}}
			}
		}
	}
	return nil
}

// locationsForFileLink resolves `[text](./other.md#anchor)` to either
// a heading in the target file or the file's first line.
func (s *Server) locationsForFileLink(targetFile, anchor string, idx *index.Index) []location {
	tgt := index.NormalizePath(targetFile)
	if tgt == "" {
		return nil
	}
	if anchor == "" {
		return []location{{
			URI:   s.workspaceURI(tgt),
			Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
		}}
	}
	fe, ok := idx.File(tgt)
	if !ok {
		// File lives outside the index (or wasn't loaded yet).
		// Return a best-effort target at line 1.
		return []location{{
			URI:   s.workspaceURI(tgt),
			Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
		}}
	}
	for _, sym := range fe.Symbols {
		if sym.Kind == index.SymbolHeading && sym.Anchor == anchor {
			return []location{{
				URI:   s.workspaceURI(tgt),
				Range: rangeAt(sym.SelectionLine, sym.SelectionCol, nil),
			}}
		}
	}
	return []location{{
		URI:   s.workspaceURI(tgt),
		Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
	}}
}

// locationForRefDef returns the position of `[label]: …` in the
// current file.
func (s *Server) locationForRefDef(rel, label string, source []byte) (location, bool) {
	idx := s.ensureIndex()
	fe, ok := idx.File(rel)
	if !ok {
		return location{}, false
	}
	for _, sym := range fe.Symbols {
		if sym.Kind == index.SymbolLinkRef && sym.Anchor == label {
			return location{
				URI:   s.workspaceURI(rel),
				Range: rangeAt(sym.SelectionLine, sym.SelectionCol, source),
			}, true
		}
	}
	return location{}, false
}

// locationsForRefsToHeading scans every file's outgoing edges for
// references to (rel, anchor) and returns one location per match.
func (s *Server) locationsForRefsToHeading(rel, anchor string, idx *index.Index) []location {
	if anchor == "" {
		return nil
	}
	edges := idx.IncomingEdges(rel, anchor)
	out := make([]location, 0, len(edges))
	for _, e := range edges {
		out = append(out, location{
			URI:   s.workspaceURI(e.SourceFile),
			Range: rangeAt(e.SourceLine, e.SourceCol, nil),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URI != out[j].URI {
			return out[i].URI < out[j].URI
		}
		return out[i].Range.Start.Line < out[j].Range.Start.Line
	})
	return out
}

// locationsForKindDefinition reports the location of the kind block
// in `.mdsmith.yml`. We surface the config file at line 1 when the
// kind is declared; absent kinds yield nothing.
func (s *Server) locationsForKindDefinition(kind string) []location {
	cfg, configPath, _ := s.snapshotConfig()
	if cfg == nil || configPath == "" {
		return nil
	}
	if _, ok := cfg.Kinds[kind]; !ok {
		return nil
	}
	return []location{{
		URI:   pathToURI(configPath),
		Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
	}}
}

// locationsForFilesByKind returns one Location per workspace file
// whose front-matter `kinds:` includes kind.
func (s *Server) locationsForFilesByKind(kind string, idx *index.Index) []location {
	files := idx.FilesByKind(kind)
	out := make([]location, 0, len(files))
	for _, rel := range files {
		out = append(out, location{
			URI:   s.workspaceURI(rel),
			Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URI < out[j].URI })
	return out
}

// handleReferences resolves textDocument/references.
func (s *Server) handleReferences(msg *requestMessage) {
	var p referencesParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid references params")
		return
	}
	source, rel, ok := s.docTextOrFile(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, []location{})
		return
	}
	idx := s.ensureIndex()
	line := p.Position.Line + 1
	col := lspPositionToByteColumn(source, line, p.Position.Character)
	res := index.Locator{Path: rel}.Locate(source, line, col)

	var out []location
	switch res.Tag {
	case index.TokenHeading:
		out = s.locationsForRefsToHeading(rel, res.Anchor, idx)
		if p.Context.IncludeDeclaration {
			out = prependLocation(out, location{
				URI:   p.TextDocument.URI,
				Range: rangeAt(p.Position.Line+1, 1, source),
			})
		}
	case index.TokenRefDef:
		// Every reference-style use of `label` in this file.
		out = s.locationsForRefUses(rel, res.Label, idx)
		if p.Context.IncludeDeclaration {
			if loc, ok := s.locationForRefDef(rel, res.Label, source); ok {
				out = prependLocation(out, loc)
			}
		}
	case index.TokenFileTop:
		// Every link target that names this file with no anchor.
		out = s.locationsForFileTop(rel, idx)
	case index.TokenFrontMatterValue:
		if res.FrontMatterKey == "kind" || res.FrontMatterKey == "kinds" {
			out = s.locationsForFilesByKind(res.FrontMatterValue, idx)
		}
	case index.TokenDirectiveArg:
		// References on a directive argument resolve to "every
		// workspace edge that points at this file" — file links
		// (no anchor) plus every <?include?>, <?build?>, and
		// <?catalog?>. Limiting to EdgeFileLink (the previous
		// behavior) hid the directive-to-directive references that
		// users actually need when navigating include / build chains.
		if res.DirectiveTargetFile != "" {
			if tgt := linkgraph.ResolveRelTarget(rel, res.DirectiveTargetFile); tgt != "" {
				out = s.locationsForFileReferences(tgt, idx)
			}
		}
	}
	if out == nil {
		out = []location{}
	}
	_ = s.t.writeResponse(msg.ID, out)
}

func prependLocation(rest []location, loc location) []location {
	out := make([]location, 0, len(rest)+1)
	out = append(out, loc)
	out = append(out, rest...)
	return out
}

// locationsForRefUses returns every `[text][label]` in rel.
func (s *Server) locationsForRefUses(rel, label string, idx *index.Index) []location {
	fe, ok := idx.File(rel)
	if !ok {
		return nil
	}
	var out []location
	for _, e := range fe.Outgoing {
		if e.Kind != index.EdgeRefLink {
			continue
		}
		if !strings.EqualFold(e.TargetLabel, label) {
			continue
		}
		out = append(out, location{
			URI:   s.workspaceURI(rel),
			Range: rangeAt(e.SourceLine, e.SourceCol, nil),
		})
	}
	return out
}

// locationsForFileTop returns every workspace link whose path
// component points at file (with empty anchor).
func (s *Server) locationsForFileTop(file string, idx *index.Index) []location {
	edges := idx.IncomingEdges(file, "")
	var out []location
	for _, e := range edges {
		if e.Kind != index.EdgeFileLink {
			continue
		}
		if e.TargetAnchor != "" {
			continue
		}
		out = append(out, location{
			URI:   s.workspaceURI(e.SourceFile),
			Range: rangeAt(e.SourceLine, e.SourceCol, nil),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URI != out[j].URI {
			return out[i].URI < out[j].URI
		}
		return out[i].Range.Start.Line < out[j].Range.Start.Line
	})
	return out
}

// locationsForFileReferences returns every workspace edge whose
// target is file: the union of file-top links and the include /
// build / catalog directives that target this file. Reference-style
// link uses are not included because they target a label, not a
// file path.
func (s *Server) locationsForFileReferences(file string, idx *index.Index) []location {
	edges := idx.IncomingEdges(file, "")
	var out []location
	for _, e := range edges {
		switch e.Kind {
		case index.EdgeFileLink:
			if e.TargetAnchor != "" {
				continue
			}
		case index.EdgeInclude, index.EdgeBuild, index.EdgeCatalog:
			// keep
		default:
			continue
		}
		out = append(out, location{
			URI:   s.workspaceURI(e.SourceFile),
			Range: rangeAt(e.SourceLine, e.SourceCol, nil),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URI != out[j].URI {
			return out[i].URI < out[j].URI
		}
		return out[i].Range.Start.Line < out[j].Range.Start.Line
	})
	return out
}
