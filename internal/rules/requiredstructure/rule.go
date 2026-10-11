package requiredstructure

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/fieldinterp"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/oscompat"
	"github.com/jeduden/mdsmith/internal/piparser"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/runcache"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/jeduden/mdsmith/internal/yamlutil"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that a document's heading structure matches a schema.
//
// A rule instance carries an ordered list of schema sources (Sources)
// — one per layer (kind, override, or top-level rule entry) that
// declared a `schema:` (file) or `inline-schema:` (inline map). The
// rule loads each source at Check time and composes them via
// schema.Compose; a file resolving to multiple kinds therefore layers
// each kind's constraints rather than letting the last one win.
//
// Schema and InlineSchema mirror the first source's parsed form when
// exactly one source is present. They support tests that drive the
// rule directly through ApplySettings with the legacy single-source
// keys; the kind-level loader still rejects configurations that set
// both keys on the same layer.
type Rule struct {
	Schema       string         // first source's file path (single-source convenience)
	InlineSchema *schema.Schema // first source's parsed inline schema
	Sources      []SchemaSource // ordered list of schema sources (canonical)
	Placeholders []string       // placeholder tokens to treat as opaque
	PathPatterns []PathPattern  // kind-level path-pattern entries
}

// SchemaSource is one entry in the rule's schema-sources list. Either
// File or Inline is set, never both. Inline schemas are pre-parsed at
// ApplySettings time so a malformed schema surfaces as a config-load
// error rather than a per-file diagnostic at Check time. File sources
// stay as paths because the rule reads them through the lint.File's
// RootFS at Check time.
type SchemaSource struct {
	File   string
	Inline *schema.Schema
}

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

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS020" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "required-structure" }

// WordlistTarget implements rule.WordlistConsumer: resolved `lists:`
// entries union into this rule's "placeholders" setting.
func (r *Rule) WordlistTarget() string { return "placeholders" }

var _ rule.WordlistConsumer = (*Rule)(nil)

// Category implements rule.Rule.
func (r *Rule) Category() string { return "structural" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	// Warn when <?require?> appears in a non-schema file.
	if reqLine := findRequireDirectiveLine(f); reqLine > 0 {
		if !r.isAnySchemaFile(f) {
			d := makeDiag(f.Path, reqLine,
				"<?require?> is only recognized in schema files; this directive has no effect here")
			d.Severity = lint.Warning
			diags = append(diags, d)
		}
	}

	// Kind-level path-pattern constraints run independently of the
	// schema source: a kind may declare `path-pattern:` without an
	// attached schema, and a schema-bearing kind may add a pattern
	// on top of a `<?require filename:?>` directive.
	diags = append(diags, r.checkPathPatterns(f)...)

	sources := r.effectiveSources()
	if len(sources) == 0 {
		return diags
	}

	// Single-source: use the legacy paths so file schemas keep their
	// heading- and body-sync features (`# {id}: {name}`, body lines
	// under Meta-Information). Composition is irrelevant when only
	// one source is configured.
	//
	// Exception: when the file source declares `extends:`, route
	// through the multi-source compose path so plan-135 inheritance
	// applies. The legacy parser does not implement extends; without
	// this re-route the parent's constraints would silently drop.
	// Body-sync still runs through the per-source bodySyncDiagnostics
	// helper inside checkComposedSources, so the child's `{field}`
	// template features survive the switch.
	if len(sources) == 1 {
		src := sources[0]
		if src.Inline != nil {
			return append(diags, r.checkSingleInlineSchema(f, src.Inline)...)
		}
		if src.File != "" {
			return append(diags, r.dispatchSingleFileSchema(f, src.File, sources)...)
		}
		return diags
	}

	return append(diags, r.checkComposedSources(f, sources)...)
}

// dispatchSingleFileSchema loads the schema once, peeks at the
// front matter for the reserved `extends:` key, and routes to the
// legacy single-file path or the compose path accordingly. Loading
// here avoids the double read the previous helper introduced: the
// legacy path reuses the bytes via checkSingleFileSchemaFromData,
// and the compose path re-loads through schema.ParseFile only when
// extends actually applies.
func (r *Rule) dispatchSingleFileSchema(
	f *lint.File, schemaPath string, sources []SchemaSource,
) []lint.Diagnostic {
	res := r.cachedRawSchema(f, schemaPath)
	if res.err != nil {
		return []lint.Diagnostic{r.diag(f.Path, 1,
			fmt.Sprintf("cannot read schema %q: %v", schemaPath, res.err))}
	}
	if res.extends {
		return r.checkComposedSources(f, sources)
	}
	return r.checkSingleFileSchemaFromData(f, schemaPath, res.data, schemaPath)
}

// rawSchemaResult bundles readSchemaFile's return value with the
// extends-peek schemaDataDeclaresExtends derives from it, so
// cachedRawSchema can memoize both behind a single RunCache slot.
// err is the raw read error, not wrapped with a caller's schemaPath
// string: two kinds can reference the same absolute schema file
// through differently-spelled (but path-equivalent) schemaPath
// strings — "docs/proto.md" vs "./docs/proto.md" both resolve to the
// same absSchemaCacheKey — and whichever caller's spelling populated
// the cache first must not leak into a later caller's diagnostic.
// dispatchSingleFileSchema formats the caller-facing message from its
// own schemaPath argument instead.
type rawSchemaResult struct {
	data    []byte
	err     error
	extends bool
}

// cachedRawSchema returns readSchemaFile's result for schemaPath plus
// the schemaDataDeclaresExtends peek over that data, computed at
// most once per absolute schema path per RunCache lifetime. Without
// this, a schema referenced by every file under a workspace-wide
// kind was re-read from disk and re-unmarshalled as YAML once per
// host file — see docs/development/high-performance-go.md's
// "memoize per-input computations" pattern. absSchemaCacheKey
// returning "" (no RunCache, or no stable absolute identity for
// schemaPath) falls back to building directly, matching the
// pre-cache behavior for the struct-literal unit-test path.
func (r *Rule) cachedRawSchema(f *lint.File, schemaPath string) rawSchemaResult {
	build := func() any {
		data, err := readSchemaFile(f, schemaPath)
		if err != nil {
			return rawSchemaResult{err: err}
		}
		return rawSchemaResult{
			data:    data,
			extends: schemaDataDeclaresExtends(data),
		}
	}
	if f.RunCache != nil {
		if absPath := absSchemaCacheKey(f, schemaPath); absPath != "" {
			return f.RunCache.RawSchemaFile(absPath, build).(rawSchemaResult)
		}
	}
	return build().(rawSchemaResult)
}

// schemaDataDeclaresExtends reports whether the raw schema bytes
// carry a reserved `extends:` key in their YAML front matter. The
// modern `schema.ParseFile` pipeline rejects malformed `extends:`
// values (non-string, empty/whitespace string) with clear errors;
// routing here on any present-and-non-null `extends:` lets those
// errors surface to the user instead of being silently swallowed
// by the legacy parser. An explicit YAML `null` matches the
// no-extends case so legacy diagnostics line up.
//
// A parse failure or missing front matter returns false — the
// legacy parser then surfaces those errors with its existing
// diagnostic shape.
func schemaDataDeclaresExtends(data []byte) bool {
	prefix, _ := lint.StripFrontMatter(data)
	if prefix == nil {
		return false
	}
	yamlBytes := lint.FrontMatterYAML(prefix)
	var raw map[string]any
	if err := yamlutil.UnmarshalSafe(yamlBytes, &raw); err != nil {
		return false
	}
	v, ok := raw["extends"]
	if !ok {
		return false
	}
	// Explicit YAML `null` matches schema.ParseFile's no-extends
	// treatment; stay on the legacy path so diagnostics align.
	// Every other value (non-empty string, malformed string,
	// non-string type) routes to the compose path so the modern
	// parser can surface its specific error.
	return v != nil
}

// effectiveSources returns the rule's sources list, falling back to
// a single-entry list built from the legacy Schema / InlineSchema
// fields when Sources is empty. This lets tests drive the rule
// directly with the older fields while still routing through the
// new multi-source code path.
func (r *Rule) effectiveSources() []SchemaSource {
	if len(r.Sources) > 0 {
		return r.Sources
	}
	if r.InlineSchema != nil && !r.InlineSchema.IsEmpty() {
		return []SchemaSource{{Inline: r.InlineSchema}}
	}
	if r.Schema != "" {
		return []SchemaSource{{File: r.Schema}}
	}
	return nil
}

// isAnySchemaFile reports whether f matches any of the configured
// file sources. When a file plays the role of its own schema (e.g.
// rule-readme's proto.md), the warning-on-misplaced-<?require?>
// check must skip it.
func (r *Rule) isAnySchemaFile(f *lint.File) bool {
	for _, src := range r.effectiveSources() {
		if src.File == "" {
			continue
		}
		if r.isSchemaFileAt(f, src.File) {
			return true
		}
	}
	return false
}

// checkSingleInlineSchema runs the validator against a single inline
// schema. Inline schemas do not support frontmatter-body {field}
// sync (no source body content) so the legacy syncPoints code path
// is skipped.
func (r *Rule) checkSingleInlineSchema(f *lint.File, sch *schema.Schema) []lint.Diagnostic {
	diags := make([]lint.Diagnostic, 0, 8)
	docFMRaw, fmDiags := cachedDocFrontMatterRaw(f)
	diags = append(diags, fmDiags...)
	fmIsCUE := placeholders.HasCUEFrontmatter(r.Placeholders)
	diags = append(diags, schema.ValidateWithParseErr(f, sch, docFMRaw,
		frontMatterParseErr(fmDiags), fmIsCUE, makeDiag)...)
	diags = append(diags, r.applyScopeRules(f, sch, docFMRaw)...)
	diags = append(diags, schema.ValidateCrossReferences(f, sch, makeDiag)...)
	diags = append(diags, schema.ValidateAcronyms(f, sch, docFMRaw, makeDiag)...)
	diags = append(diags, schema.ValidateIndex(f, sch, makeDiag)...)
	return diags
}

// Fix implements rule.FixableRule. For single file-based schemas it
// rewrites body lines whose {field} template matches but whose value
// disagrees with the document's front matter (body-sync fix). For
// any configured inline schema (single-source or composed across
// kinds) that declares an `index:` block, Fix also emits the JSON
// side-output next to the source file. `mdsmith check` skips the
// write, preserving check's read-only contract (plan 143).
//
// Fix swallows errors (composition and WriteIndex both). WriteIndex
// itself records any I/O failure in the package-level cache keyed
// by f.Path; the next Check reads that cache and surfaces the
// underlying error in place of the generic "missing / out of date"
// message, so users are not trapped in a fix loop without signal.
// Composition errors are similarly swallowed — they re-surface on
// the next Check pass through the same checkComposedSources path.
func (r *Rule) Fix(f *lint.File) []byte {
	sch, err := r.composedSchemaForFix(f)
	if err == nil && sch != nil && !sch.IsEmpty() && sch.Index != nil {
		_ = schema.WriteIndex(f, sch)
	}
	sources := r.effectiveSources()
	if len(sources) == 1 && sources[0].File != "" && !r.isSchemaFileAt(f, sources[0].File) {
		schData, schPath, loadErr := r.loadSchemaAt(f, sources[0].File)
		if loadErr == nil {
			parsedSch, parseErr := cachedParseSchema(f, schData, schPath)
			if parseErr == nil {
				docFMRaw, _ := cachedDocFrontMatterRaw(f)
				return fixBodySyncIn(f, parsedSch, docFMRaw)
			}
		}
	}
	return f.Source
}

// fixBodySyncIn rewrites body lines whose {field} template matches but
// whose resolved front-matter value disagrees with the document text.
// It returns f.Source unchanged when no lines need rewriting.
func fixBodySyncIn(f *lint.File, sch *parsedSchema, docFM map[string]any) []byte {
	if len(docFM) == 0 || len(sch.SyncPoints) == 0 {
		return f.Source
	}
	docHeadings := extractHeadings(f)
	work := make([][]byte, len(f.Lines))
	copy(work, f.Lines)
	modified := false
	docIdx := 0
	for schIdx, req := range sch.Headings {
		if isSectionWildcard(req) {
			continue
		}
		syncs := sch.SyncPoints[schIdx]
		if len(syncs) == 0 {
			_, docIdx = advanceToMatch(req, docHeadings, docIdx)
			continue
		}
		matchedDoc, newIdx := advanceToMatch(req, docHeadings, docIdx)
		docIdx = newIdx
		if matchedDoc < 0 {
			continue
		}
		dh := docHeadings[matchedDoc]
		startLine := dh.Line + 1
		endLine := len(f.Lines)
		if matchedDoc+1 < len(docHeadings) {
			endLine = docHeadings[matchedDoc+1].Line - 1
		}
		for _, sp := range syncs {
			if patchedLine, ok := resolveBodySyncLine(sp, docFM, work, startLine, endLine); ok {
				work[patchedLine.idx] = patchedLine.val
				modified = true
			}
		}
	}
	if !modified {
		return f.Source
	}
	return bytes.Join(work, []byte("\n"))
}

// patchedLine carries the index and new value for a line that needs rewriting.
type patchedLine struct {
	idx int
	val []byte
}

// resolveBodySyncLine returns the line index and replacement bytes for sp
// if the document contains a stale template-match line in [startLine, endLine).
// ok is false when sp is not a body sync point, the field is missing, the
// line already matches, or no template-matching line is found.
func resolveBodySyncLine(
	sp syncPoint, docFM map[string]any,
	work [][]byte, startLine, endLine int,
) (patchedLine, bool) {
	if !sp.InBody {
		return patchedLine{}, false
	}
	path := fieldinterp.ParseCUEPath(sp.Field)
	if path == nil {
		return patchedLine{}, false
	}
	if _, err := fieldinterp.ResolvePath(docFM, path); err != nil {
		return patchedLine{}, false
	}
	// Convert expected once so per-line comparisons and the eventual
	// replacement both work from the same []byte, no string() cast.
	expectedBytes := []byte(resolveFields(sp.BodyText, docFM))
	re := sp.compiled
	for i := startLine - 1; i < endLine && i < len(work); i++ {
		trimmed := bytes.TrimSpace(work[i])
		if bytes.Equal(trimmed, expectedBytes) {
			continue // already correct; keep scanning for stale duplicates
		}
		if re.Match(trimmed) {
			leadLen := len(work[i]) - len(bytes.TrimLeft(work[i], " \t"))
			val := make([]byte, leadLen, leadLen+len(expectedBytes))
			copy(val, work[i][:leadLen])
			val = append(val, expectedBytes...)
			return patchedLine{idx: i, val: val}, true
		}
	}
	return patchedLine{}, false
}

// buildSchemaHeading constructs a schemaHeading from a docHeading,
// pre-compiling the field-interpolation regex when the text contains
// {field} references so matchesSchema pays no per-call compile cost.
// Pattern construction is delegated to buildFieldPattern, which owns
// the QuoteMeta+".+" logic and its MustCompile invariant. cache is
// forwarded to buildFieldPattern; see its doc comment.
func buildSchemaHeading(h docHeading, cache *fieldPatternCache) schemaHeading {
	sh := schemaHeading{Level: h.Level, Text: h.Text}
	if fieldinterp.ContainsField(h.Text) {
		sh.compiled = buildFieldPattern(h.Text, cache)
	}
	return sh
}

// composedSchemaForFix returns the same composed *schema.Schema
// that checkComposedSources validates against — but without
// running validation or body-sync (Fix doesn't need either).
// Returns nil with no error when the rule has no schema source or
// every source is empty / self-referential. A file source pointing
// at the file currently being fixed is skipped so a schema
// doesn't drive its own index side-output.
func (r *Rule) composedSchemaForFix(f *lint.File) (*schema.Schema, error) {
	sources := r.effectiveSources()
	if len(sources) == 0 {
		return nil, nil
	}
	parsed := make([]*schema.Schema, 0, len(sources))
	for _, src := range sources {
		if src.Inline != nil {
			if src.Inline.IsEmpty() {
				continue
			}
			parsed = append(parsed, src.Inline)
			continue
		}
		if src.File == "" {
			continue
		}
		sch, err := r.parseFileSchemaForCompose(f, src.File)
		if err != nil {
			return nil, err
		}
		if sch == nil {
			continue
		}
		parsed = append(parsed, sch)
	}
	if len(parsed) == 0 {
		return nil, nil
	}
	return schema.Compose(parsed...)
}

// ComposedSchema parses and composes every schema source the rule
// resolved for f and returns the composed schema, or nil when the
// rule has no schema source. Exposed for the `extract` subcommand,
// which projects the same composed schema MDS020 validates against;
// it reuses composedSchemaForFix so the two paths cannot drift.
func (r *Rule) ComposedSchema(f *lint.File) (*schema.Schema, error) {
	return r.composedSchemaForFix(f)
}

// checkSingleFileSchemaFromData runs the legacy validation with a
// pre-loaded schema buffer. The Check dispatch reads the schema
// once and routes the bytes here when extends is not declared, so
// the common single-source path avoids a second read.
func (r *Rule) checkSingleFileSchemaFromData(
	f *lint.File, schemaPath string, schData []byte, schPath string,
) []lint.Diagnostic {
	var diags []lint.Diagnostic
	sch, err := cachedParseSchema(f, schData, schPath)
	if err != nil {
		return append(diags, r.diag(f.Path, 1,
			fmt.Sprintf("invalid schema %q: %v", schemaPath, err)))
	}

	// Skip the schema file itself when schemas come from disk.
	if r.isSchemaFileAt(f, schemaPath) {
		return diags
	}

	docHeadings := extractHeadings(f)
	docFMRaw, fmDiags := cachedDocFrontMatterRaw(f)
	diags = append(diags, fmDiags...)
	// The cue-frontmatter placeholder token marks the front-matter
	// values as CUE expressions rather than concrete data.
	fmIsCUE := placeholders.HasCUEFrontmatter(r.Placeholders)

	fmErr := frontMatterParseErr(fmDiags)

	// Check filename pattern.
	diags = append(diags,
		checkFilenamePattern(f, sch, r.Schema, docFMRaw, fmErr, fmIsCUE)...)

	// Check structure: required headings present and in order.
	diags = append(diags, checkStructure(f, sch, docHeadings, r.Schema)...)

	// Validate document front matter against schema-embedded CUE
	// constraints, unless the values are CUE expressions themselves
	// or failed to parse: the parse diagnostic above names the cause,
	// and every field would otherwise read as "<missing>".
	if !fmIsCUE && fmErr == nil {
		fmSch := &schema.Schema{
			Frontmatter:      sch.Config.Frontmatter,
			FrontmatterLines: sch.Config.FrontmatterLines,
			FrontmatterMeta:  sch.Config.FrontmatterMeta,
			Source:           r.Schema,
		}
		diags = append(diags, schema.ValidateFrontmatterDiags(f, fmSch, docFMRaw, makeDiag)...)
	}

	// Check frontmatter-body sync using raw map for nested access.
	diags = append(diags, checkSync(f, sch, docHeadings, docFMRaw)...)

	return diags
}

// checkComposedSources loads every source, composes them via
// schema.Compose, and validates the document against the composed
// schema. Each FILE source ALSO runs the legacy heading- and
// body-sync check (proto.md `# {id}: {name}` and Meta-Information
// body lines) — composition cannot express those checks today, so
// per-source legacy validation preserves them. Sources that name a
// file the rule is currently linting are skipped (self-validation).
func (r *Rule) checkComposedSources(f *lint.File, sources []SchemaSource) []lint.Diagnostic {
	var diags []lint.Diagnostic
	docFMRaw, fmDiags := cachedDocFrontMatterRaw(f)
	diags = append(diags, fmDiags...)

	parsed := make([]*schema.Schema, 0, len(sources))
	for _, src := range sources {
		if src.Inline != nil {
			if src.Inline.IsEmpty() {
				continue
			}
			parsed = append(parsed, src.Inline)
			continue
		}
		if src.File == "" {
			continue
		}
		// Per-source legacy body-sync. Loads via the legacy parser so
		// proto.md-style {field} interpolation and Meta-Information
		// body sync still fire for each file source.
		if !r.isSchemaFileAt(f, src.File) {
			diags = append(diags, r.bodySyncDiagnostics(f, src.File, docFMRaw)...)
		}
		sch, err := r.parseFileSchemaForCompose(f, src.File)
		if err != nil {
			diags = append(diags, r.diag(f.Path, 1, err.Error()))
			continue
		}
		if sch == nil {
			// f was the schema itself — skip its composition entry.
			continue
		}
		parsed = append(parsed, sch)
	}

	if len(parsed) == 0 {
		return diags
	}

	composed, err := schema.Compose(parsed...)
	if err != nil {
		return append(diags, r.diag(f.Path, 1,
			fmt.Sprintf("composing schemas: %v", err)))
	}
	// composed is non-nil here: parsed contains at least one
	// non-nil schema (the empty-source filter above guarantees it),
	// and schema.Compose returns its single input unchanged when
	// len(parsed) == 1. IsEmpty() can still hold when every parsed
	// schema was itself empty (e.g. a proto.md with no headings).
	if composed.IsEmpty() {
		return diags
	}

	fmIsCUE := placeholders.HasCUEFrontmatter(r.Placeholders)
	diags = append(diags, schema.ValidateWithParseErr(f, composed, docFMRaw,
		frontMatterParseErr(fmDiags), fmIsCUE, makeDiag)...)
	diags = append(diags, r.applyScopeRules(f, composed, docFMRaw)...)
	diags = append(diags, schema.ValidateCrossReferences(f, composed, makeDiag)...)
	diags = append(diags, schema.ValidateAcronyms(f, composed, docFMRaw, makeDiag)...)
	diags = append(diags, schema.ValidateIndex(f, composed, makeDiag)...)
	return diags
}

// parseFileSchemaForCompose loads a proto.md file source via the
// unified schema.ParseFile parser so the result composes with inline
// schemas. Returns (nil, nil) when f is the schema file itself —
// the caller skips that entry so a schema doesn't validate against
// itself.
func (r *Rule) parseFileSchemaForCompose(f *lint.File, schemaPath string) (*schema.Schema, error) {
	if r.isSchemaFileAt(f, schemaPath) {
		return nil, nil
	}
	reader := &schema.FileReader{
		RootFS:   f.RootFS,
		RootDir:  f.RootDir,
		MaxBytes: f.MaxInputBytes,
	}
	sch, err := schema.ParseFile(reader, schemaPath)
	if err != nil {
		return nil, fmt.Errorf("cannot load schema %q: %v", schemaPath, err)
	}
	return sch, nil
}

// bodySyncDiagnostics runs only the heading- and body-sync portion
// of the legacy file-schema check for a single file source. The
// composed structure validation runs separately; calling
// checkSingleFileSchema here would double-report missing-section
// and frontmatter-CUE diagnostics.
func (r *Rule) bodySyncDiagnostics(f *lint.File, schemaPath string, docFMRaw map[string]any) []lint.Diagnostic {
	var diags []lint.Diagnostic
	if len(docFMRaw) == 0 {
		return diags
	}
	data, _, err := r.loadSchemaAt(f, schemaPath)
	if err != nil {
		return append(diags, r.diag(f.Path, 1, err.Error()))
	}
	sch, err := cachedParseSchema(f, data, schemaPath)
	if err != nil {
		// The compose path reports schema-parse errors separately;
		// avoid duplicating them here.
		return diags
	}
	docHeadings := extractHeadings(f)
	return append(diags, checkSync(f, sch, docHeadings, docFMRaw)...)
}

// loadSchemaAt reads the named schema file using the file's RootFS
// when configured, falling back to the OS filesystem.
func (r *Rule) loadSchemaAt(f *lint.File, schemaPath string) ([]byte, string, error) {
	data, err := readSchemaFile(f, schemaPath)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read schema %q: %v", schemaPath, err)
	}
	return data, schemaPath, nil
}

// isSchemaFileAt reports whether f is the schema file at the named
// path. It normalizes f.Path against f.RootDir so the check still
// succeeds when mdsmith runs from a subdirectory while `schema:`
// paths remain project-root-relative.
func (r *Rule) isSchemaFileAt(f *lint.File, schemaPath string) bool {
	if schemaPath == "" {
		return false
	}
	if isSchemaFile(f.Path, schemaPath) {
		return true
	}
	if f.RootDir == "" {
		return false
	}
	abs, _ := filepath.Abs(f.Path)
	rel, err := filepath.Rel(f.RootDir, abs)
	if err != nil {
		return false
	}
	return isSchemaFile(rel, schemaPath)
}

func (r *Rule) diag(file string, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     file,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Error,
		Message:  msg,
	}
}

var (
	_ rule.Configurable       = (*Rule)(nil)
	_ rule.ListMerger         = (*Rule)(nil)
	_ rule.FixableRule        = (*Rule)(nil)
	_ rule.SettingsTranslator = (*Rule)(nil)
)

// validateCUESchemaSyntax checks that schema compiles as CUE.
//
// The free function form (no RunCache) is the canonical entry point
// kept for tests. The cache-bound variant validateCUESchemaSyntaxWith
// is reached from parseSchemaFrontMatter when a cache is in scope,
// so two schemas with identical CUE source share one compile per
// Run.
func validateCUESchemaSyntax(schema string) error {
	return validateCUESchemaSyntaxWith(nil, schema)
}

// validateCUESchemaSyntaxWith is validateCUESchemaSyntax with the
// CompileString site routed through cache when non-nil. A nil cache
// compiles a fresh value (the test-direct path).
func validateCUESchemaSyntaxWith(cache *runcache.Cache, schema string) error {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	if err := cachedCompiledCUEWith(cache, schema).Err(); err != nil {
		return fmt.Errorf("invalid schema frontmatter CUE: %w", err)
	}
	return nil
}

func validateFrontMatterCUE(schema string, fm map[string]any) error {
	if strings.TrimSpace(schema) == "" {
		return nil
	}

	compiled := cachedCompiledCUEWith(nil, schema)
	if err := compiled.Err(); err != nil {
		return fmt.Errorf("invalid CUE schema: %w", err)
	}

	if fm == nil {
		fm = map[string]any{}
	}

	// Validate the front-matter map directly against the cached schema, no
	// JSON marshal round-trip (plan 218). The context-free schema Value is
	// shared safely with no per-file recompile.
	if err := compiled.Value.CompileMap(fm).Validate(); err != nil {
		return err
	}

	return nil
}

// docFrontMatter is one readDocFrontMatterRaw result, boxed so
// f.MemoFile can cache the (map, diagnostics) pair behind one key.
type docFrontMatter struct {
	raw   map[string]any
	diags []lint.Diagnostic
}

// docFrontMatterMemoKey names the per-file memo entry holding the
// decoded front matter.
const docFrontMatterMemoKey = "MDS020.docFrontMatter"

// cachedDocFrontMatterRaw is readDocFrontMatterRaw decoded at most
// once per file: the kind path-pattern check and the schema check
// both need the front matter, and without the memo a kind that has
// both an interpolating path-pattern and a schema would pay two YAML
// decodes per file. Every caller gets the same map and must treat it
// as read-only; none of them writes to it.
//
// A file with no front matter returns before the memo, so it pays no
// memo entry against the rule's allocation budget. The read is
// checked: MemoFile keeps a nil value when a builder panicked, and
// that case decodes directly rather than panicking a second time.
func cachedDocFrontMatterRaw(f *lint.File) (map[string]any, []lint.Diagnostic) {
	if len(f.FrontMatter) == 0 {
		return nil, nil
	}
	v, ok := f.MemoFile(docFrontMatterMemoKey, buildDocFrontMatter).(*docFrontMatter)
	if !ok {
		return readDocFrontMatterRaw(f)
	}
	return v.raw, v.diags
}

// buildDocFrontMatter is cachedDocFrontMatterRaw's memo builder. It is
// a package-level function so the memo call allocates no closure.
func buildDocFrontMatter(f *lint.File) any {
	raw, diags := readDocFrontMatterRaw(f)
	return &docFrontMatter{raw: raw, diags: diags}
}

// readDocFrontMatterRaw reads YAML frontmatter from the document.
func readDocFrontMatterRaw(f *lint.File) (map[string]any, []lint.Diagnostic) {
	if len(f.FrontMatter) == 0 {
		return nil, nil
	}

	yamlBytes := lint.FrontMatterYAML(f.FrontMatter)
	var raw map[string]any
	if err := yamlutil.UnmarshalSafe(yamlBytes, &raw); err != nil {
		format := "front matter: invalid YAML: %v"
		if errors.Is(err, yamlutil.ErrAliases) {
			format = "front matter: %v"
		}
		return nil, []lint.Diagnostic{makeDiag(f.Path, schema.NonBodyDiagLine(f),
			fmt.Sprintf(format, err))}
	}
	return raw, nil
}

// frontMatterParseErr returns the front-matter parse failure that
// readDocFrontMatterRaw reported as fmDiags, or nil when the block
// parsed. The `filename:` and `path-pattern:` hints name it in place
// of "frontmatter value missing", and the field check is skipped
// because every field would read as absent.
func frontMatterParseErr(fmDiags []lint.Diagnostic) error {
	if len(fmDiags) == 0 {
		return nil
	}
	return errors.New(fmDiags[0].Message)
}

// findRequireDirectiveLine returns the 1-based line number of the first
// <?require?> PI in the file, or 0 if none is found.
func findRequireDirectiveLine(f *lint.File) int {
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		pi, ok := c.(*piparser.ProcessingInstruction)
		if ok && pi.Name == "require" {
			return f.LineOfOffset(pi.Lines().At(0).Start)
		}
	}
	return 0
}

// isSchemaFile checks if the document path is the configured schema.
func isSchemaFile(docPath, schemaPath string) bool {
	docInfo, errDoc := os.Stat(docPath)
	schemaInfo, errSchema := os.Stat(schemaPath)
	if errDoc == nil && errSchema == nil && oscompat.SameFile(docInfo, schemaInfo) {
		return true
	}
	// Fall back to path equality when Stat fails or sameFile returns false
	// (e.g. on tinygo/wasm builds where os.SameFile is not implemented).
	docAbs, errDocAbs := filepath.Abs(docPath)
	schemaAbs, errSchemaAbs := filepath.Abs(schemaPath)
	if errDocAbs != nil || errSchemaAbs != nil {
		return false
	}
	return docAbs == schemaAbs
}

// formatHeading returns a markdown-style heading string.
func formatHeading(level int, text string) string {
	return strings.Repeat("#", level) + " " + text
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
// `_ :=` discards mirror isSchemaFile's pattern above: filepath.Abs
// only fails when os.Getwd fails, which the engine would already
// have surfaced during file discovery.
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

// readSchemaFile reads the schema file using the file's RootFS when available,
// falling back to os.ReadFile. When RootFS is set, absolute and
// parent-traversal paths are rejected to prevent reading outside the
// project root.
func readSchemaFile(f *lint.File, schema string) ([]byte, error) {
	if f.RootFS != nil {
		// Reject absolute paths and parent traversals.
		if filepath.IsAbs(schema) {
			return nil, fmt.Errorf("absolute schema path not allowed")
		}
		clean := filepath.ToSlash(filepath.Clean(schema))
		clean = strings.TrimPrefix(clean, "./")
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("schema path %q escapes project root", schema)
		}
		return bytelimit.ReadFSFileLimited(f.RootFS, clean, f.MaxInputBytes)
	}
	return bytelimit.ReadFileLimited(schema, f.MaxInputBytes)
}

// schemaRootFS returns f's workspace RootFS, or nil when f is nil. It is
// the seam cachedParseSchema uses to feed RootFS-based <?include?>
// fragment reads (see parseSchemaWithRootFS).
func schemaRootFS(f *lint.File) fs.FS {
	if f == nil {
		return nil
	}
	return f.RootFS
}

// readSchemaInclude reads a schema <?include?> fragment. It prefers the
// workspace RootFS — the Session/WASM and LSP path, where os.ReadFile is
// absent or "not implemented on js" — and falls back to the OS filesystem
// for the on-disk CLI. includedPath is project-relative
// (resolveSchemaIncludePath rejects ".."), so it is a valid fs.FS key.
func readSchemaInclude(rootFS fs.FS, includedPath string, maxBytes int64) ([]byte, error) {
	if rootFS != nil {
		return bytelimit.ReadFSFileLimited(rootFS, filepath.ToSlash(includedPath), maxBytes)
	}
	return bytelimit.ReadFileLimited(includedPath, maxBytes)
}

func makeDiag(file string, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     file,
		Line:     line,
		Column:   1,
		RuleID:   "MDS020",
		RuleName: "required-structure",
		Severity: lint.Error,
		Message:  msg,
	}
}

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Sync section to front matter" }
