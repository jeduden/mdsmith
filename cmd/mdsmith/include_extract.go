package main

import (
	"fmt"
	"io/fs"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/extract"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/include"
	"github.com/jeduden/mdsmith/internal/rules/requiredstructure"
	"github.com/jeduden/mdsmith/internal/schema"
)

// installIncludeExtractProjector wires the production projector
// that `<?include?>` directives carrying `extract:` consult. The
// projector reads the loaded .mdsmith.yml at cfgPath, resolves the
// target file's kind, composes its schema, parses the target with
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
func installIncludeExtractProjector(cfgPath string) {
	if cfgPath == "" {
		include.SetExtractProjector(nil)
		return
	}
	include.SetExtractProjector(func(
		host *lint.File, readFS fs.FS, targetFile string, data []byte,
	) (any, error) {
		return projectIncludeExtract(cfgPath, host, readFS, targetFile, data)
	})
}

// projectIncludeExtract runs the full schema-compose + extract
// projection on targetFile. Split from the closure above so the
// pipeline is testable as a plain function without resetting the
// rule's package-level projector.
func projectIncludeExtract(
	cfgPath string,
	host *lint.File, readFS fs.FS, targetFile string, data []byte,
) (any, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	fmKinds, fmFields, err := decodeTargetFrontMatter(data, targetFile)
	if err != nil {
		return nil, err
	}

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

	tf, err := lint.NewFileFromSource(targetFile, data, host.StripFrontMatter)
	if err != nil {
		return nil, fmt.Errorf("parsing %q: %w", targetFile, err)
	}
	tf.MaxInputBytes = host.MaxInputBytes
	tf.FS = readFS
	tf.RootFS = host.RootFS
	tf.RootDir = host.RootDir

	rsRule := &requiredstructure.Rule{}
	if rr.Final.Settings != nil {
		if err := rsRule.ApplySettings(rr.Final.Settings); err != nil {
			return nil, fmt.Errorf(
				"loading schema config for %q: %w", targetFile, err)
		}
	}
	sch, err := rsRule.ComposedSchema(tf)
	if err != nil {
		return nil, fmt.Errorf(
			"composing schema for %q: %w", targetFile, err)
	}
	if sch == nil || sch.IsEmpty() {
		return nil, fmt.Errorf(
			"%q declares no schema to extract against", targetFile)
	}

	// Gate projection on a clean schema validation: a non-conformant
	// target would produce a partial / lossy projection that the
	// caller cannot rely on. Bubble the underlying diagnostic up
	// so the include error points at the same root cause `mdsmith
	// check` would surface for the target.
	mkDiag := func(file string, line int, msg string) lint.Diagnostic {
		return lint.Diagnostic{File: file, Line: line, Message: msg}
	}
	if vd := schema.Validate(tf, sch, fmFields, false, mkDiag); len(vd) > 0 {
		return nil, fmt.Errorf(
			"target file does not conform to its schema: %s",
			vd[0].Message)
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
