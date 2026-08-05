package main

import (
	"fmt"
	"io/fs"
	"sync"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/extract"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rules/include"
	"github.com/jeduden/mdsmith/internal/rules/requiredstructure"
	"github.com/jeduden/mdsmith/internal/schema"
)

// installIncludeExtractProjector wires the production projector
// that `<?include?>` directives carrying `extract:` consult. The
// projector reads the .mdsmith.yml at cfgPath, resolves the target
// file's kind, composes its schema, parses the target with
// internal/lint, runs internal/extract.Extract, and returns the
// resulting JSON tree.
//
// The include rule package cannot import internal/config or
// internal/rules/requiredstructure directly (the rule-boundaries
// integration test forbids cross-rule imports, and internal/config
// blank-imports the rule for registration so the reverse arrow
// would form a compile cycle). Wiring lives here in cmd/mdsmith
// so the rule stays at its layer of the dependency graph.
//
// An empty cfgPath clears the projector — `<?include?>` then
// surfaces a clear diagnostic on any `extract:` use, the same
// outcome as a project without `.mdsmith.yml`.
//
// cfgPath is captured inside the closure handed to
// SetExtractProjector. Each install creates a fresh function value
// that owns its own cfgPath, so a concurrent LSP reload that calls
// installIncludeExtractProjector with a new path cannot race
// against in-flight rule.Check goroutines reading the path — the
// mutex inside include.SetExtractProjector serialises the pointer
// swap, and the old function value's captured cfgPath stays valid
// until the last caller returns.
func installIncludeExtractProjector(cfgPath string) {
	if cfgPath == "" {
		include.SetExtractProjector(nil)
		return
	}
	// Memoized once per installed-projector generation: cfgPath is
	// fixed for the life of this closure, and a config reload calls
	// installIncludeExtractProjector again with a fresh closure (see
	// the cfgPath-capture note above), which naturally invalidates
	// this cache. Without it, every <?include extract:?> directive
	// resolved under one generation re-read and re-parsed .mdsmith.yml
	// from disk — a workspace catalog with many extract entries redid
	// that work once per entry, on every check/fix/export pass
	// (docs/development/high-performance-go.md "Memoize per-input
	// computations").
	loadCfg := sync.OnceValues(func() (*config.Config, error) {
		return config.Load(cfgPath)
	})
	include.SetExtractProjector(func(
		host *lint.File, readFS fs.FS, targetFile string, data []byte,
	) (any, error) {
		return projectIncludeExtractWithConfig(
			loadCfg, host, readFS, targetFile, data)
	})
}

// projectIncludeExtract runs the full schema-compose + extract
// projection on targetFile, loading cfgPath fresh on every call. Split
// from the closure above so the pipeline is testable as a plain
// function without resetting the rule's package-level projector.
func projectIncludeExtract(
	cfgPath string,
	host *lint.File, readFS fs.FS, targetFile string, data []byte,
) (any, error) {
	return projectIncludeExtractWithConfig(
		func() (*config.Config, error) { return config.Load(cfgPath) },
		host, readFS, targetFile, data)
}

// projectIncludeExtractWithConfig is projectIncludeExtract with the
// config-loading step factored out behind loadCfg, so
// installIncludeExtractProjector can pass a memoized loader shared
// across every directive resolved under one projector generation.
func projectIncludeExtractWithConfig(
	loadCfg func() (*config.Config, error),
	host *lint.File, readFS fs.FS, targetFile string, data []byte,
) (any, error) {
	cfg, err := loadCfg()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	fmKinds, fmFields, err := decodeTargetFrontMatter(data, targetFile)
	if err != nil {
		return nil, err
	}
	rsSettings, err := resolveRequiredStructureSettings(
		cfg, targetFile, fmKinds, fmFields)
	if err != nil {
		return nil, err
	}
	tf := buildTargetFile(host, readFS, targetFile, data)
	sch, phs, err := composeTargetSchema(tf, targetFile, rsSettings)
	if err != nil {
		return nil, err
	}
	fmIsCUE := placeholders.HasCUEFrontmatter(phs)
	if err := validateTargetAgainstSchema(tf, sch, fmFields, fmIsCUE); err != nil {
		return nil, err
	}
	mt := schema.BuildMatchTree(tf, sch, fmFields)
	tree, diags := extract.Extract(tf, sch, mt)
	if len(diags) > 0 {
		return nil, fmt.Errorf(
			"projection failed for %q: %s",
			targetFile, diags[0].Message)
	}
	return tree, nil
}

// resolveRequiredStructureSettings looks up the kind set for
// targetFile and returns the required-structure settings the
// projector should apply to its private rule instance. An empty
// kind set or a disabled required-structure surfaces as an error
// — both block projection at the same point CLI extract would.
func resolveRequiredStructureSettings(
	cfg *config.Config, targetFile string,
	fmKinds []string, fmFields map[string]any,
) (map[string]any, error) {
	res := config.ResolveFile(cfg, targetFile, fmKinds, fmFields)
	if len(res.Kinds) == 0 {
		return nil, fmt.Errorf(
			"%q has no resolved kind; cannot project a typed value",
			targetFile)
	}
	rr, ok := res.Rules["required-structure"]
	if !ok || !rr.Final.Enabled {
		return nil, fmt.Errorf(
			"required-structure is disabled for %q; "+
				"no schema to project against", targetFile)
	}
	return rr.Final.Settings, nil
}

// buildTargetFile parses data as Markdown the same way the engine
// would, with the host's strip-frontmatter / max-input-bytes /
// FS settings copied over so the projection sees the same
// coordinate system the rest of the lint uses.
//
// lint.NewFileFromSource never errors with the current goldmark
// configuration (same invariant cmd/mdsmith/export.go and four
// other in-tree rules already rely on via the same nolint:errcheck
// annotation), so the parse error is discarded; if NewFile ever
// becomes fallible the upgrade lands across every site in lockstep.
func buildTargetFile(
	host *lint.File, readFS fs.FS, targetFile string, data []byte,
) *lint.File {
	tf, _ := lint.NewFileFromSource(targetFile, data, host.StripFrontMatter) //nolint:errcheck // never errors today
	tf.MaxInputBytes = host.MaxInputBytes
	tf.FS = readFS
	tf.RootFS = host.RootFS
	tf.RootDir = host.RootDir
	// Forward the engine-owned RunCache so repeated `<?include
	// extract:?>` directives against the same target across a single
	// engine.Run reuse the host's read-side cache rather than
	// independently re-reading + re-parsing the target on every
	// directive.
	tf.RunCache = host.RunCache
	return tf
}

// composeTargetSchema builds the composed schema MDS020 would
// validate tf against. Returns the composed schema and the
// placeholder vocabulary the rsRule resolved (so the caller can
// derive fmIsCUE for schema.Validate). A nil schema means the kind
// declares no schema sources, which is a hard error here — there is
// nothing to project against.
func composeTargetSchema(
	tf *lint.File, targetFile string, rsSettings map[string]any,
) (*schema.Schema, []string, error) {
	rsRule := &requiredstructure.Rule{}
	if rsSettings != nil {
		if err := rsRule.ApplySettings(rsSettings); err != nil {
			return nil, nil, fmt.Errorf(
				"loading schema config for %q: %w", targetFile, err)
		}
	}
	sch, err := rsRule.ComposedSchema(tf)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"composing schema for %q: %w", targetFile, err)
	}
	if sch == nil || sch.IsEmpty() {
		return nil, nil, fmt.Errorf(
			"%q declares no schema to extract against", targetFile)
	}
	return sch, rsRule.Placeholders, nil
}

// validateTargetAgainstSchema gates projection on a clean schema
// validation. A non-conformant target would produce a partial /
// lossy projection; bubble the underlying diagnostic up so the
// include error points at the same root cause `mdsmith check`
// would surface for the target. fmIsCUE comes from the resolved
// placeholder vocabulary (matching requiredstructure.Rule's own
// Check), so kinds declaring `cue-frontmatter` do not produce
// spurious frontmatter-parse diagnostics.
func validateTargetAgainstSchema(
	tf *lint.File, sch *schema.Schema, fmFields map[string]any,
	fmIsCUE bool,
) error {
	mkDiag := func(file string, line int, msg string) lint.Diagnostic {
		return lint.Diagnostic{File: file, Line: line, Message: msg}
	}
	if vd := schema.Validate(tf, sch, fmFields, fmIsCUE, mkDiag); len(vd) > 0 {
		return fmt.Errorf(
			"target file does not conform to its schema: %s",
			vd[0].Message)
	}
	return nil
}

// decodeTargetFrontMatter returns the frontmatter kinds list and
// raw fields from the target file's bytes. A file without
// frontmatter returns (nil, nil, nil); a decode failure surfaces
// as an error so the diagnostic points at the parse problem
// instead of silently projecting an empty object.
func decodeTargetFrontMatter(
	data []byte, targetFile string,
) ([]string, map[string]any, error) {
	prefix, _ := lint.StripFrontMatter(data)
	if len(prefix) == 0 {
		return nil, nil, nil
	}
	fields, err := lint.ParseFrontMatterFields(prefix)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"parsing frontmatter of %q: %w", targetFile, err)
	}
	var kinds []string
	if raw, ok := fields["kinds"]; ok {
		switch v := raw.(type) {
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					kinds = append(kinds, s)
				}
			}
		case string:
			kinds = []string{v}
		}
	}
	return kinds, fields, nil
}
