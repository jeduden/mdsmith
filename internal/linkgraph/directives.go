package linkgraph

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/jeduden/mdsmith/internal/lint"
)

// DirectiveKind enumerates the directives whose argument bodies name
// other files. <?include?> and <?build?> reference a single file each;
// <?catalog?> references a glob and resolves to many files at expansion
// time.
type DirectiveKind int

const (
	// DirectiveInclude is a `<?include file: …?>` directive.
	DirectiveInclude DirectiveKind = iota
	// DirectiveBuild is a `<?build source: …?>` directive.
	DirectiveBuild
	// DirectiveCatalog is a `<?catalog glob: …?>` directive. Catalog
	// edges carry Globs and the Unresolved flag; TargetPath is empty
	// for them. Callers wanting the resolved file set call
	// ExpandCatalog(Globs, files).
	DirectiveCatalog
)

// DirectiveEdge is one parsed reference from a directive to its
// argument target(s). Lines are body-relative — counted from the start
// of the parsed body, not the original file (see Link doc for why).
//
// For DirectiveInclude and DirectiveBuild, TargetPath is the raw
// `file:` / `source:` value as it appeared in the directive body.
// Callers resolve it against the host file via ResolveRelTarget.
//
// For DirectiveCatalog, Unresolved is true, TargetPath is empty, and
// Globs holds the patterns the catalog walks. ExpandCatalog expands
// the patterns against a workspace file list.
type DirectiveEdge struct {
	Line       int
	Column     int
	Kind       DirectiveKind
	TargetPath string
	Globs      []string
	Unresolved bool
}

// ExtractDirectives walks f.AST for include / build / catalog
// processing-instructions at the document root and returns one
// DirectiveEdge per directive whose arguments parse cleanly. Malformed
// YAML bodies, missing required args, and closing markers (<?/name?>)
// are skipped.
func ExtractDirectives(f *lint.File) []DirectiveEdge {
	if f == nil || f.AST == nil {
		return nil
	}
	var out []DirectiveEdge
	for n := f.AST.FirstChild(); n != nil; n = n.NextSibling() {
		pi, ok := n.(*lint.ProcessingInstruction)
		if !ok {
			continue
		}
		if strings.HasPrefix(pi.Name, "/") {
			continue
		}
		params, ok := parsePIParams(pi, f.Source)
		if !ok {
			continue
		}
		line, col := piPosition(f, pi)
		switch pi.Name {
		case "include":
			if file := strings.TrimSpace(params["file"]); file != "" {
				out = append(out, DirectiveEdge{
					Line:       line,
					Column:     col,
					Kind:       DirectiveInclude,
					TargetPath: file,
				})
			}
		case "build":
			if src := strings.TrimSpace(params["source"]); src != "" {
				out = append(out, DirectiveEdge{
					Line:       line,
					Column:     col,
					Kind:       DirectiveBuild,
					TargetPath: src,
				})
			}
		case "catalog":
			globs := splitGlobValue(params["glob"])
			out = append(out, DirectiveEdge{
				Line:       line,
				Column:     col,
				Kind:       DirectiveCatalog,
				Globs:      globs,
				Unresolved: true,
			})
		}
	}
	return out
}

// piPosition returns the 1-based body-relative line and column of a
// processing-instruction's opening marker.
func piPosition(f *lint.File, pi *lint.ProcessingInstruction) (int, int) {
	lines := pi.Lines()
	if lines.Len() == 0 {
		return 1, 1
	}
	start := lines.At(0).Start
	return f.LineOfOffset(start), 1
}

// parsePIParams converts a PI block's YAML body into a flat string
// map. Single-line PIs (no body) yield an empty map and ok=true.
func parsePIParams(pi *lint.ProcessingInstruction, source []byte) (map[string]string, bool) {
	body := extractPIBody(pi, source)
	mp := gensection.MarkerPair{
		StartLine: 1, // line numbers are not used; we set 1 to avoid the zero default
		YAMLBody:  body,
	}
	rawMap, diags := gensection.ParseYAMLBody("", mp, "", "")
	if len(diags) > 0 {
		return nil, false
	}
	gensection.ExtractColumnsRaw(rawMap)
	params, diags := gensection.ValidateStringParams("", mp.StartLine, rawMap, "", "")
	if len(diags) > 0 {
		return nil, false
	}
	return params, true
}

// extractPIBody returns the YAML body of a PI block (every line after
// the opening line, before the closing `?>`).
func extractPIBody(pi *lint.ProcessingInstruction, source []byte) string {
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

// splitGlobValue splits the joined string ValidateStringParams produces
// for a YAML sequence value (entries joined with "\n") back into a
// slice. Empty entries are skipped.
func splitGlobValue(joined string) []string {
	if joined == "" {
		return nil
	}
	var out []string
	for _, entry := range strings.Split(joined, "\n") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// ExpandCatalog expands globs against files (workspace-relative paths)
// and returns the matching files. Patterns prefixed with `!` are
// exclusion patterns; an exclusion match removes the file from the
// result even when another pattern includes it. The result preserves
// the order of files.
//
// Pattern validation: invalid doublestar patterns are skipped (they
// match nothing). The function returns an empty slice when globs is
// empty or no file matches.
func ExpandCatalog(globs, files []string) []string {
	if len(globs) == 0 || len(files) == 0 {
		return nil
	}
	include, exclude := globpath.SplitIncludeExclude(globs)
	if len(include) == 0 {
		return nil
	}
	// Pre-validate exclude patterns once. doublestar.ValidatePattern
	// returns false for malformed patterns; skip them so an unrelated
	// typo doesn't suppress the whole result set.
	validExcludes := make([]string, 0, len(exclude))
	for _, p := range exclude {
		if doublestar.ValidatePattern(p) {
			validExcludes = append(validExcludes, p)
		}
	}
	var out []string
	for _, file := range files {
		if !matchAnyValid(include, file) {
			continue
		}
		if matchAnyValid(validExcludes, file) {
			continue
		}
		out = append(out, file)
	}
	return out
}

// matchAnyValid reports whether any of patterns matches path. Invalid
// patterns are silently skipped (Match already returns false for them
// — this helper exists for symmetry with the validation above).
func matchAnyValid(patterns []string, p string) bool {
	for _, pat := range patterns {
		if globpath.Match(pat, p) {
			return true
		}
	}
	return false
}
