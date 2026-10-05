package requiredstructure

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/schema"
)

// PathPattern records a kind's `path-pattern:` constraint: the kind
// that declared it and the glob the workspace-relative path of every
// file in the kind must match. Populated by the config merge layer
// from KindBody.PathPattern. Build one with newPathPattern so the
// derived fields are set.
type PathPattern struct {
	Kind    string
	Pattern string // as the author wrote it; diagnostics quote this

	// match is Pattern in the form it is matched in, and interp
	// reports whether that form carries a `\#(fmvar(...))`
	// reference (schema.PathPatternMatchForm). Both depend only on
	// the pattern, so they are computed once here rather than for
	// every file on the check hot path.
	match  string
	interp bool
}

// newPathPattern builds the PathPattern for one kind's
// `path-pattern:`, deriving its match form once.
func newPathPattern(kind, pattern string) PathPattern {
	match, interp := schema.PathPatternMatchForm(pattern)
	return PathPattern{Kind: kind, Pattern: pattern, match: match, interp: interp}
}

// parsePathPatterns reads the `path-patterns` rule setting: a list of
// {kind, pattern} maps installed by the config merge layer from each
// kind's `path-pattern:` field. The merge layer is the only documented
// producer; the parser still validates shape so a hand-written rule
// override fails loudly instead of silently dropping entries.
func parsePathPatterns(v any) ([]PathPattern, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf(
			"path-patterns must be a list of {kind, pattern} maps, got %T", v)
	}
	out := make([]PathPattern, 0, len(list))
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"path-patterns[%d] must be a map, got %T", i, item)
		}
		kindV, hasKind := m["kind"]
		patV, hasPat := m["pattern"]
		if !hasKind || !hasPat {
			return nil, fmt.Errorf(
				"path-patterns[%d] must set both `kind` and `pattern`", i)
		}
		kind, ok := kindV.(string)
		if !ok || kind == "" {
			return nil, fmt.Errorf(
				"path-patterns[%d].kind must be a non-empty string, got %T", i, kindV)
		}
		pat, ok := patV.(string)
		if !ok || pat == "" {
			return nil, fmt.Errorf(
				"path-patterns[%d].pattern must be a non-empty string, got %T", i, patV)
		}
		// Validate the pattern as a doublestar glob at config time
		// so an unmatched bracket or other syntax error surfaces as
		// a config error instead of an MDS020 diagnostic on every
		// file assigned to the kind.
		// PathPatternSyntaxForm swaps each `\#(fmvar(...))` reference
		// for a `?*` wildcard, since a reference resolves per document
		// and its own bytes are not glob syntax.
		if !doublestar.ValidatePattern(schema.PathPatternSyntaxForm(pat)) {
			return nil, fmt.Errorf(
				"path-patterns[%d].pattern %q is not a valid doublestar glob",
				i, pat)
		}
		out = append(out, newPathPattern(kind, pat))
	}
	return out, nil
}

// checkPathPatterns validates the workspace-relative path of f
// against every kind-level `path-pattern:` configured on the rule.
// One diagnostic is emitted per failing pattern; a kind whose pattern
// matches contributes no diagnostic. Matching uses the same doublestar
// syntax as overrides:, ignore:, and kind-assignment:, anchored at the
// workspace root.
func (r *Rule) checkPathPatterns(f *lint.File) []lint.Diagnostic {
	if len(r.PathPatterns) == 0 {
		return nil
	}
	rel := filepath.ToSlash(workspaceRelPath(f))
	var diags []lint.Diagnostic
	// The document's front matter is read at most once per call, and
	// only once some pattern actually references it. Nearly every
	// path-pattern is a plain glob, and this runs for every file of
	// every kind on the check hot path.
	var docFM map[string]any
	var fmParseErr error
	fmRead := false
	for _, pp := range r.PathPatterns {
		// pp.match is the pattern with host separators turned into
		// `/`, keeping the `\` that opens each reference, and
		// pp.interp says whether it has one; newPathPattern derived
		// both once, so a plain glob costs no scan here.
		if !pp.interp {
			if matchWorkspacePath(pp.match, rel) {
				continue
			}
			diags = append(diags, pathPatternDiag(f, rel, pp,
				schema.LiteralFmvarHint(pp.Pattern)))
			continue
		}
		if placeholders.HasCUEFrontmatter(r.Placeholders) {
			// The front-matter values are CUE constraints
			// (`name: string`), not data: a reference has no value
			// to substitute, so it matches any non-empty text in one
			// segment and only the literal rest of the pattern is
			// checked.
			if matchWorkspacePath(schema.WildcardGlobRefs(pp.match), rel) {
				continue
			}
			diags = append(diags, pathPatternDiag(f, rel, pp,
				schema.LiteralFmvarHint(pp.Pattern)))
			continue
		}
		if !fmRead {
			// An unparseable block leaves every `fmvar` reference
			// unresolved. Keep the parse failure so the hint names
			// it rather than claiming the field is missing: a kind
			// that declares only `path-pattern:` has no schema path
			// in Check that would report the parse error itself.
			var fmDiags []lint.Diagnostic
			docFM, fmDiags = cachedDocFrontMatterRaw(f)
			fmParseErr = frontMatterParseErr(fmDiags)
			fmRead = true
		}
		resolved, err := schema.ResolveGlobPattern(pp.match, docFM)
		if err != nil {
			// The parse failure is WHY the reference did not
			// resolve, so it takes the "missing" report's place.
			// GlobMismatchHint still appends the rest — a malformed
			// opener elsewhere in the pattern — after "; ".
			if fmParseErr != nil {
				err = fmParseErr
			}
			hint := schema.GlobMismatchHint(err, []string{pp.Pattern})
			diags = append(diags, pathPatternDiag(f, rel, pp, hint))
			continue
		}
		// A resolved pattern is no longer the string ValidatePattern
		// accepted at config-parse time, so use the validating
		// matcher rather than MatchUnvalidated. This branch is off
		// the hot path — only kinds that interpolate reach it.
		ok, mErr := doublestar.Match(resolved, rel)
		if mErr == nil && ok {
			continue
		}
		hint := schema.GlobMismatchHint(nil, []string{pp.Pattern},
			schema.GlobHintForm(pp.match, docFM))
		if mErr != nil {
			// The escaped value keeps the glob valid everywhere but
			// inside a character class, where a reference is not
			// supported: `[\#(fmvar(tag))]` with `tag: "!"` leaves
			// `[!]`. Name the syntax error, as the `filename:`
			// surface does, instead of a plain mismatch.
			hint += "; not a valid glob: " + mErr.Error()
		}
		diags = append(diags, pathPatternDiag(f, rel, pp, hint))
	}
	return diags
}

// matchWorkspacePath reports whether the workspace-relative path rel
// satisfies the plain (non-interpolated) glob pat.
//
// Match the full workspace-relative path with doublestar directly.
// Going through globpath.Match would also try the basename, which
// would let `path-pattern: README.md` pass for `docs/README.md` —
// defeating the documented root-anchored semantics.
//
// MatchUnvalidated (not Match) because pat already passed
// doublestar.ValidatePattern once, at config-parse time
// (parsePathPatterns); Match's own internal validation step re-runs
// on every call whenever matching reaches the end of rel before the
// end of the pattern — the common case for a mismatching kind, which
// is most kinds for most files.
//
// Passing ValidatePattern does not guarantee Match and
// MatchUnvalidated agree, though: a brace alternative (e.g.
// "{[!mdb[],docs/**/*.md}") can contain a syntax error in a
// non-final alternative that Match's internal validation aborts on
// before trying later alternatives, while MatchUnvalidated tries them
// all — confirmed directly against the vendored doublestar source.
// That divergence requires brace syntax (a 1.3M+-case differential
// fuzz over brace-free, ValidatePattern-accepted patterns found zero
// disagreement), so a pattern using "{" falls back to the safe
// (slower, but provably correct) Match.
func matchWorkspacePath(pat, rel string) bool {
	if strings.IndexByte(pat, '{') < 0 {
		return doublestar.MatchUnvalidated(pat, rel)
	}
	ok, err := doublestar.Match(pat, rel)
	return err == nil && ok
}

// pathPatternDiag builds the MDS020 diagnostic for a file whose
// workspace-relative path does not satisfy its kind's
// `path-pattern:`. hint, when non-empty, carries the extra line an
// interpolated pattern needs — the pattern with front matter applied,
// or why a `\#(fmvar(...))` reference could not be resolved.
//
// path-pattern checks the workspace-relative path (which may include
// directories), so the field label is "path" rather than "filename".
// The latter is reserved for basename-only checks emitted by
// validateFilename / checkFilenamePattern.
func pathPatternDiag(
	f *lint.File, rel string, pp PathPattern, hint string,
) lint.Diagnostic {
	d := schema.SchemaDiagnostic{
		Field:     "path",
		Actual:    strconv.Quote(rel),
		Expected:  "path matching glob " + pp.Pattern,
		Hint:      hint,
		SchemaRef: fmt.Sprintf("kinds[%s] / path-pattern", pp.Kind),
	}
	return d.Emit(makeDiag, f.Path, schema.NonBodyDiagLine(f))
}

// workspaceRelPath returns the file path relative to the workspace
// root when RootDir is set, falling back to the file's own Path. The
// returned path is slash-normalized so glob patterns written with
// forward slashes match on every platform. Both RootDir and Path are
// resolved through filepath.Abs first so the relative computation
// works when the CLI was invoked with a relative `--config` path
// (e.g. `--config sub/.mdsmith.yml` makes RootDir relative). The
// `_ :=` discards mirror isSchemaFileAt's pattern in rule.go:
// filepath.Abs only fails when os.Getwd fails, which the engine
// would already have surfaced during file discovery.
func workspaceRelPath(f *lint.File) string {
	if f.RootDir == "" {
		return filepath.ToSlash(f.Path)
	}
	absRoot, _ := filepath.Abs(f.RootDir)
	absPath, _ := filepath.Abs(f.Path)
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return filepath.ToSlash(f.Path)
	}
	return filepath.ToSlash(rel)
}

// checkFilenamePattern checks that the document basename matches the
// schema's filename glob pattern (if configured).
func checkFilenamePattern(
	f *lint.File, sch *parsedSchema, schemaSource string,
	docFM map[string]any, fmErr error, fmIsCUE bool,
) []lint.Diagnostic {
	// schema.FilenameDiagnostic owns the wording, the
	// `\#(fmvar(name))` resolution against the document's front
	// matter (a wildcard when fmIsCUE marks the values as CUE
	// constraints), and the OR-list semantics, so the legacy proto.md
	// path and the inline/composed path (schema.validateFilename)
	// cannot drift apart.
	d := schema.FilenameDiagnostic(
		sch.Config.FilenamePatterns, filepath.Base(f.Path), docFM,
		fmErr, fmIsCUE, buildSchemaRefForLegacy(schemaSource))
	if d == nil {
		return nil
	}
	// Filename diagnostics describe the document as a whole;
	// use the non-body anchor so filterGeneratedDiags can't
	// drop them when body line 1 sits inside a generated
	// section.
	return []lint.Diagnostic{d.Emit(makeDiag, f.Path, schema.NonBodyDiagLine(f))}
}
