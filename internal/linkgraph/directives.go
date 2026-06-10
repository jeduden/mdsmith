package linkgraph

import (
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/piparser"
)

// DirectiveKind enumerates the directive shapes ExtractDirectives
// recognises.
type DirectiveKind int

const (
	// DirectiveInclude is a `<?include file: …?>` directive.
	DirectiveInclude DirectiveKind = iota
	// DirectiveBuild is one `inputs:` entry of a `<?build?>` directive.
	// A literal entry carries Path; a glob entry carries a single-
	// element Globs and reports IsUnresolved.
	DirectiveBuild
	// DirectiveCatalog is a `<?catalog glob: …?>` directive. Catalog
	// targets are glob patterns; concrete files are produced by
	// ExpandCatalog against a workspace file list.
	DirectiveCatalog
)

// DirectiveEdge is one directive's parsed target.
//
// Line and Col are body-relative (post front-matter strip) — same
// convention as Link.Line/Column. Callers needing file-relative
// coordinates must add f.LineOffset themselves.
//
// For DirectiveInclude, Path carries the raw `file:` value verbatim;
// callers resolve it against the host file's directory using
// ResolveRelTarget. For DirectiveBuild, Path carries one literal
// `inputs:` entry (when the entry has no glob metacharacters) and is
// resolved the same way.
//
// For DirectiveCatalog — and for a DirectiveBuild whose `inputs:`
// entry is a glob — Globs carries the raw pattern(s) and Path is
// empty. The IsUnresolved method returns true for those edges so
// reverse-edge queries skip them generically — see the index layer
// for the corresponding Unresolved flag.
type DirectiveEdge struct {
	Line  int
	Col   int
	Kind  DirectiveKind
	Path  string
	Globs []string
}

// IsUnresolved reports whether this directive points at glob patterns
// that need workspace-list expansion before they identify concrete
// files. True for catalog edges and for build edges built from a glob
// `inputs:` entry (those carry Globs and no Path); false for include
// and literal build edges.
func (d DirectiveEdge) IsUnresolved() bool {
	return d.Kind == DirectiveCatalog ||
		(d.Kind == DirectiveBuild && len(d.Globs) > 0)
}

// ExtractDirectives walks f.AST top-level for processing-instruction
// nodes whose name is "include", "build", or "catalog", parses each
// one's YAML body, and returns DirectiveEdges for the targets it
// carries: one per `<?include?>`, one per `<?build?>` inputs: entry,
// and one per `<?catalog?>` (whose Globs hold the whole pattern list).
// Directives with malformed YAML or empty required parameters are
// skipped silently — the dedicated lint rules surface those as
// diagnostics; this extractor only contributes to the link graph.
//
// Like ExtractLinks, ExtractDirectives is pure given its input: it
// does no file reads, no workspace traversal, and no global state
// mutation, so callers can invoke it concurrently across files.
func ExtractDirectives(f *lint.File) []DirectiveEdge {
	if f == nil || f.AST == nil {
		return nil
	}
	var out []DirectiveEdge
	for n := f.AST.FirstChild(); n != nil; n = n.NextSibling() {
		pi, ok := n.(*piparser.ProcessingInstruction)
		if !ok {
			continue
		}
		if strings.HasPrefix(pi.Name, "/") {
			continue
		}
		switch pi.Name {
		case "include", "build", "catalog":
		default:
			continue
		}
		line := directivePILine(f, pi)
		params, ok := parsePIParams(pi, f.Source)
		if !ok {
			continue
		}
		switch pi.Name {
		case "include":
			file := strings.TrimSpace(params["file"])
			if file == "" {
				continue
			}
			out = append(out, DirectiveEdge{
				Line: line,
				Col:  1,
				Kind: DirectiveInclude,
				Path: file,
			})
		case "build":
			out = appendBuildEdges(out, line, params["inputs"])
		case "catalog":
			globs := splitCatalogGlobs(params["glob"])
			out = append(out, DirectiveEdge{
				Line:  line,
				Col:   1,
				Kind:  DirectiveCatalog,
				Globs: globs,
			})
		}
	}
	return out
}

// directivePILine returns the 1-based body-relative line of the
// directive's opening marker. Goldmark guarantees a parsed PI has at
// least one source line.
func directivePILine(f *lint.File, pi *piparser.ProcessingInstruction) int {
	if pi.Lines().Len() == 0 {
		return 1
	}
	return f.LineOfOffset(pi.Lines().At(0).Start)
}

// parsePIParams converts a PI block's YAML body into a flat string
// map. Single-line PIs (no body) yield an empty map and ok=true.
//
// Malformed YAML yields ok=false, matching the index's
// "if you can't trust the params, don't synthesize an edge from
// them" rule. Dedicated lint rules report the user-facing diagnostic.
func parsePIParams(pi *piparser.ProcessingInstruction, source []byte) (map[string]string, bool) {
	body := extractPIBody(pi, source)
	startLine := 1
	if pi.Lines().Len() > 0 {
		startLine = lineOfOffset(source, pi.Lines().At(0).Start)
	}
	mp := gensection.MarkerPair{StartLine: startLine, YAMLBody: body}
	rawMap, diags := gensection.ParseYAMLBody("", mp, "", "")
	if len(diags) > 0 {
		return nil, false
	}
	gensection.ExtractColumnsRaw(rawMap)
	params, diags := gensection.ValidateStringParams("", startLine, rawMap, "", "")
	if len(diags) > 0 {
		return nil, false
	}
	return params, true
}

func extractPIBody(pi *piparser.ProcessingInstruction, source []byte) string {
	lines := pi.Lines()
	if lines.Len() <= 1 {
		return ""
	}
	var b strings.Builder
	for i := 1; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return b.String()
}

// appendBuildEdges emits one DirectiveBuild edge per non-empty
// `inputs:` entry in rawInputs (the newline-joined list value
// gensection produces). A literal entry becomes a resolved edge with
// Path set; an entry carrying glob metacharacters becomes an
// unresolved edge whose single-element Globs holds the pattern, mirroring
// how catalog globs are handled. Empty-after-trim entries are skipped.
func appendBuildEdges(out []DirectiveEdge, line int, rawInputs string) []DirectiveEdge {
	for _, entry := range strings.Split(rawInputs, "\n") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if hasGlobMeta(entry) {
			out = append(out, DirectiveEdge{
				Line:  line,
				Col:   1,
				Kind:  DirectiveBuild,
				Globs: []string{entry},
			})
			continue
		}
		out = append(out, DirectiveEdge{
			Line: line,
			Col:  1,
			Kind: DirectiveBuild,
			Path: entry,
		})
	}
	return out
}

// hasGlobMeta reports whether s contains a doublestar glob
// metacharacter (*, ?, [, {). It matches the conservative classifier
// MDS039 uses to decide whether an inputs: entry is a literal path or
// a pattern.
func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[{")
}

// splitCatalogGlobs returns the patterns in raw as a slice. The
// catalog directive accepts either a single string or a YAML list;
// gensection.ValidateStringParams normalises list values into a
// newline-joined string, so we split on newlines and drop empty
// entries.
func splitCatalogGlobs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "\n")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ExpandCatalog returns the subset of files that match any of the
// given glob patterns. Patterns prefixed with `!` are exclusion
// patterns — see globpath.MatchAny for the precise semantics.
//
// The function does not walk the filesystem; the caller is
// responsible for supplying the candidate file list (typically the
// workspace-relative paths the discovery layer produced). Order in
// the returned slice matches the order in files.
func ExpandCatalog(globs, files []string) []string {
	if len(globs) == 0 || len(files) == 0 {
		return nil
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		if globpath.MatchAny(globs, f) {
			out = append(out, f)
		}
	}
	return out
}

// lineOfOffset is a body-local 1-based line index for a byte offset.
// Used for marker-pair start lines where a *lint.File is not
// available (e.g. inside parsePIParams' YAML body diagnostic path).
func lineOfOffset(source []byte, offset int) int {
	if offset < 0 {
		return 1
	}
	if offset > len(source) {
		offset = len(source)
	}
	line := 1
	for i := 0; i < offset; i++ {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}
