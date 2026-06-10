package include

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/extract"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rules/requiredstructure"
	"github.com/jeduden/mdsmith/internal/schema"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installTestProjector wires an ExtractProjector backed by the
// real internal/extract pipeline against the config at cfgPath for
// the duration of the calling test. Tests in this package run
// sequentially by default, so resetting the global on cleanup is
// enough — a parallel test must restore itself.
//
// The real cmd/mdsmith projector is wired up in main.go; this
// in-package version sits in test code so the production rule
// stays free of internal/config + internal/rules/requiredstructure
// imports (the rule-boundaries integration test forbids them).
func installTestProjector(t *testing.T, cfgPath string) {
	t.Helper()
	SetExtractProjector(func(
		host *lint.File, readFS fs.FS, targetFile string, data []byte,
	) (any, error) {
		return runTestProjection(cfgPath, host, readFS, targetFile, data)
	})
	t.Cleanup(func() { SetExtractProjector(nil) })
}

// runTestProjection mirrors cmd/mdsmith's projectIncludeExtract
// well enough to drive the include rule's `extract:` integration
// tests. Split out of installTestProjector so each step (config
// load, frontmatter decode, schema compose, validate, extract)
// stays focused and the gocognit budget on the wiring closure
// stays small.
func runTestProjection(
	cfgPath string,
	host *lint.File, readFS fs.FS, targetFile string, data []byte,
) (any, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	fmKinds, fmFields, err := testDecodeFrontMatter(data, targetFile)
	if err != nil {
		return nil, err
	}
	rsSettings, err := testResolveRsSettings(cfg, targetFile, fmKinds, fmFields)
	if err != nil {
		return nil, err
	}
	tf, sch, phs, err := testComposeTargetSchema(
		host, readFS, targetFile, data, rsSettings)
	if err != nil {
		return nil, err
	}
	fmIsCUE := placeholders.HasCUEFrontmatter(phs)
	if err := testValidateAgainstSchema(tf, sch, fmFields, fmIsCUE); err != nil {
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

func testDecodeFrontMatter(
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

func testResolveRsSettings(
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

func testComposeTargetSchema(
	host *lint.File, readFS fs.FS, targetFile string, data []byte,
	rsSettings map[string]any,
) (*lint.File, *schema.Schema, []string, error) {
	tf, err := lint.NewFileFromSource(targetFile, data, host.StripFrontMatter)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parsing %q: %w", targetFile, err)
	}
	tf.MaxInputBytes = host.MaxInputBytes
	tf.FS = readFS
	tf.RootFS = host.RootFS
	tf.RootDir = host.RootDir
	tf.RunCache = host.RunCache

	rsRule := &requiredstructure.Rule{}
	if rsSettings != nil {
		if err := rsRule.ApplySettings(rsSettings); err != nil {
			return nil, nil, nil, fmt.Errorf(
				"loading schema config for %q: %w", targetFile, err)
		}
	}
	sch, err := rsRule.ComposedSchema(tf)
	if err != nil {
		return nil, nil, nil, fmt.Errorf(
			"composing schema for %q: %w", targetFile, err)
	}
	if sch == nil || sch.IsEmpty() {
		return nil, nil, nil, fmt.Errorf(
			"%q declares no schema to extract against", targetFile)
	}
	return tf, sch, rsRule.Placeholders, nil
}

func testValidateAgainstSchema(
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

// minimalMessagingCfg seeds a temp project with a messaging-style
// kind so the include rule can resolve a schema for the target
// file and run the same projection `mdsmith extract messaging`
// would. Kept small to keep these tests focused on the include
// path — the projection-shape contract is enforced by the
// internal/extract suite and the end-to-end extract test.
const minimalMessagingCfg = `front-matter: true
rules:
  required-structure: true
kinds:
  message:
    schema:
      frontmatter:
        title: nonEmpty
      closed: false
      sections:
        - heading: null
        - heading: { regex: '^Tagline$' }
          content:
            - { kind: paragraph, required: true }
        - heading: { regex: '^Headline$' }
          content:
            - { kind: code-block, required: true }
kind-assignment:
  - glob: ["message.md"]
    kinds: [message]
`

const minimalMessagingFile = `---
title: Mdsmith
---
# Mdsmith

## Tagline

Markdown, fast.

## Headline

` + "```markdown\nMark*down*, smithed.\n```\n"

// setupMessagingProject seeds a temp dir with a small messaging
// kind, a conformant file, and returns the temp dir path plus the
// relative path to the host file the test will write to and lint.
func setupMessagingProject(t *testing.T) (root, hostRel string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(minimalMessagingCfg), 0o644))
	msgPath := filepath.Join(dir, "message.md")
	require.NoError(t, os.WriteFile(msgPath, []byte(minimalMessagingFile), 0o644))
	return dir, "host.md"
}

// newHostFile builds a *lint.File whose Source is src and whose
// RootDir / FS / RootFS are wired the same way the engine does for
// a file inside `root`. hostRel is the host file's path relative
// to root — the include rule joins f.Path against the included
// directory, so an absolute host path would cause resolveIncludePath
// to escape the root unexpectedly.
func newHostFile(t *testing.T, root, hostRel, src string) *lint.File {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, hostRel), []byte(src), 0o644))
	f, err := lint.NewFile(hostRel, []byte(src))
	require.NoError(t, err)
	f.SetRootDir(root)
	f.FS = os.DirFS(filepath.Join(root, filepath.Dir(hostRel)))
	return f
}

// =====================================================================
// extract: end-to-end — section content
// =====================================================================

func TestCheck_ExtractParagraphSection(t *testing.T) {
	root, hostRel := setupMessagingProject(t)
	installTestProjector(t, filepath.Join(root, ".mdsmith.yml"))
	src := "<?include\nfile: message.md\nextract: tagline.text\n?>\n" +
		"Markdown, fast.\n<?/include?>\n"
	f := newHostFile(t, root, hostRel, src)

	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_ExtractObjectWithSingleContentKey(t *testing.T) {
	// `extract: tagline` (without `.text`) splices the paragraph
	// because the wrapper carries a single recognised content key.
	root, hostRel := setupMessagingProject(t)
	installTestProjector(t, filepath.Join(root, ".mdsmith.yml"))
	src := "<?include\nfile: message.md\nextract: tagline\n?>\n" +
		"Markdown, fast.\n<?/include?>\n"
	f := newHostFile(t, root, hostRel, src)

	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_ExtractCodeBlock(t *testing.T) {
	root, hostRel := setupMessagingProject(t)
	installTestProjector(t, filepath.Join(root, ".mdsmith.yml"))
	src := "<?include\nfile: message.md\nextract: headline.code\n?>\n" +
		"Mark*down*, smithed.\n<?/include?>\n"
	f := newHostFile(t, root, hostRel, src)

	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_ExtractFrontmatterScalar(t *testing.T) {
	root, hostRel := setupMessagingProject(t)
	installTestProjector(t, filepath.Join(root, ".mdsmith.yml"))
	src := "<?include\nfile: message.md\nextract: frontmatter.title\n?>\n" +
		"Mdsmith\n<?/include?>\n"
	f := newHostFile(t, root, hostRel, src)

	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// extract: failure paths
// =====================================================================

func TestCheck_ExtractMissingPath(t *testing.T) {
	root, hostRel := setupMessagingProject(t)
	installTestProjector(t, filepath.Join(root, ".mdsmith.yml"))
	src := "<?include\nfile: message.md\nextract: nope.x\n?>\n" +
		"old\n<?/include?>\n"
	f := newHostFile(t, root, hostRel, src)

	r := &Rule{}
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "nope")
}

func TestCheck_ExtractOnFileWithNoKind(t *testing.T) {
	// A file whose resolved-kind set is empty has no extract
	// contract — surface a hard error instead of silently
	// projecting an empty object.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("rules: {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untyped.md"),
		[]byte("# Untyped\n\nJust text.\n"), 0o644))
	src := "<?include\nfile: untyped.md\nextract: text\n?>\n" +
		"old\n<?/include?>\n"
	f := newHostFile(t, dir, "host.md", src)
	installTestProjector(t, cfgPath)

	r := &Rule{}
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "no resolved kind")
}

func TestCheck_ExtractTargetFailsSchema(t *testing.T) {
	// When the target file is non-conformant against its kind's
	// schema, the directive should surface a schema-level
	// diagnostic rather than projecting partial data.
	root, hostRel := setupMessagingProject(t)
	installTestProjector(t, filepath.Join(root, ".mdsmith.yml"))
	// Overwrite message.md with a non-conformant body (no Tagline).
	require.NoError(t, os.WriteFile(filepath.Join(root, "message.md"),
		[]byte("---\ntitle: Mdsmith\n---\n# Mdsmith\n\n## Headline\n\n"+
			"```markdown\nMark*down*, smithed.\n```\n"), 0o644))
	src := "<?include\nfile: message.md\nextract: tagline.text\n?>\n" +
		"old\n<?/include?>\n"
	f := newHostFile(t, root, hostRel, src)

	r := &Rule{}
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	// The diagnostic should reference the schema mismatch.
	assert.Contains(t, diags[0].Message,
		"target file does not conform to its schema")
}

func TestCheck_ExtractWithoutProjector(t *testing.T) {
	// With no projector installed, an `extract:` directive surfaces
	// a clear diagnostic instead of silently emitting an empty body.
	root, hostRel := setupMessagingProject(t)
	src := "<?include\nfile: message.md\nextract: tagline.text\n?>\n" +
		"old\n<?/include?>\n"
	f := newHostFile(t, root, hostRel, src)
	// No installTestProjector call; projector remains nil.

	r := &Rule{}
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "no extract projector")
}

// =====================================================================
// Fix path — regenerates the block body from the projection
// =====================================================================

func TestFix_ExtractParagraphSection(t *testing.T) {
	root, hostRel := setupMessagingProject(t)
	installTestProjector(t, filepath.Join(root, ".mdsmith.yml"))
	src := "<?include\nfile: message.md\nextract: tagline.text\n?>\n" +
		"stale\n<?/include?>\n"
	f := newHostFile(t, root, hostRel, src)

	r := &Rule{}
	got := string(r.Fix(f))
	want := "<?include\nfile: message.md\nextract: tagline.text\n?>\n" +
		"Markdown, fast.\n<?/include?>\n"
	assert.Equal(t, want, got)
}

func TestFix_ExtractRoundTripStable(t *testing.T) {
	// Running fix twice should be byte-stable.
	root, hostRel := setupMessagingProject(t)
	installTestProjector(t, filepath.Join(root, ".mdsmith.yml"))
	src := "<?include\nfile: message.md\nextract: tagline.text\n?>\n" +
		"stale\n<?/include?>\n"
	f := newHostFile(t, root, hostRel, src)

	r := &Rule{}
	first := r.Fix(f)
	f2 := newHostFile(t, root, hostRel, string(first))
	second := r.Fix(f2)
	assert.Equal(t, string(first), string(second))
}

// =====================================================================
// Plan 243: <?include extract: title?> splices the H1 plain text
// =====================================================================

// TestCheck_ExtractH1Title verifies that `extract: title` in an
// include directive splices the H1 plain text from an H2-rooted
// schema file. The minimalMessagingFile has `# Mdsmith` as its H1.
func TestCheck_ExtractH1Title(t *testing.T) {
	root, hostRel := setupMessagingProject(t)
	installTestProjector(t, filepath.Join(root, ".mdsmith.yml"))
	src := "<?include\nfile: message.md\nextract: title\n?>\n" +
		"Mdsmith\n<?/include?>\n"
	f := newHostFile(t, root, hostRel, src)

	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// TestFix_ExtractH1Title verifies that fixing regenerates the body
// from the H1 plain text when the include path is `extract: title`.
func TestFix_ExtractH1Title(t *testing.T) {
	root, hostRel := setupMessagingProject(t)
	installTestProjector(t, filepath.Join(root, ".mdsmith.yml"))
	src := "<?include\nfile: message.md\nextract: title\n?>\n" +
		"stale\n<?/include?>\n"
	f := newHostFile(t, root, hostRel, src)

	r := &Rule{}
	got := string(r.Fix(f))
	want := "<?include\nfile: message.md\nextract: title\n?>\n" +
		"Mdsmith\n<?/include?>\n"
	assert.Equal(t, want, got)
}
