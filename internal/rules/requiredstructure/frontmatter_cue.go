package requiredstructure

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/piparser"
	"github.com/jeduden/mdsmith/internal/runcache"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/jeduden/mdsmith/internal/yamlutil"
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
