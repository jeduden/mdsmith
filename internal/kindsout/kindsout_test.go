package kindsout

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingWriter returns the configured error from every Write so we
// can exercise the error-handling branches in WriteBodyText / etc.
type failingWriter struct {
	err   error
	after int // number of successful writes before erroring
	calls int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls > w.after {
		return 0, w.err
	}
	return len(p), nil
}

func TestRuleCfgValue_AllForms(t *testing.T) {
	assert.Equal(t, false, RuleCfgValue(config.RuleCfg{Enabled: false}))
	assert.Equal(t, true, RuleCfgValue(config.RuleCfg{Enabled: true}))
	v := RuleCfgValue(config.RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"max": 30},
	})
	m, ok := v.(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 30, m["max"])

	// Deep-merge can leave Enabled=false with non-nil Settings (a
	// bool-only layer toggling Enabled while inheriting Settings from
	// an earlier layer). The output value must report `false` so it
	// cannot contradict the `enabled` leaf.
	assert.Equal(t, false, RuleCfgValue(config.RuleCfg{
		Enabled:  false,
		Settings: map[string]any{"max": 30},
	}))
}

func TestRuleCfgJSON_Marshal(t *testing.T) {
	r := RuleCfgJSON{v: false}
	data, err := r.MarshalJSON()
	require.NoError(t, err)
	assert.Equal(t, "false", string(data))
}

func TestMakeBodyJSON_PreservesShape(t *testing.T) {
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"line-length":           {Enabled: true, Settings: map[string]any{"max": 30}},
			"paragraph-readability": {Enabled: false},
		},
		Categories: map[string]bool{"meta": true},
	}
	out := MakeBodyJSON("plan", body, nil)
	assert.Equal(t, "plan", out.Name)
	require.Contains(t, out.Rules, "line-length")
	require.Contains(t, out.Rules, "paragraph-readability")

	enc, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(enc), `"line-length":{"max":30}`)
	assert.Contains(t, string(enc), `"paragraph-readability":false`)
}

func TestWriteBodyText_RendersYAMLBody(t *testing.T) {
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 30}},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "plan", body, nil))
	out := buf.String()
	assert.Contains(t, out, "plan:")
	assert.Contains(t, out, "rules:")
	assert.Contains(t, out, "max: 30")
}

func TestWriteBodyText_RendersPathPattern(t *testing.T) {
	body := config.KindBody{
		PathPattern: "plan/[0-9][0-9]*_*.md",
	}
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "plan", body, nil))
	out := buf.String()
	assert.Contains(t, out, "plan:")
	assert.Contains(t, out, "path-pattern: plan/[0-9][0-9]*_*.md")
}

func TestMakeBodyJSON_IncludesPathPattern(t *testing.T) {
	body := config.KindBody{PathPattern: "plan/*.md"}
	enc, err := json.Marshal(MakeBodyJSON("plan", body, nil))
	require.NoError(t, err)
	assert.Contains(t, string(enc), `"path-pattern":"plan/*.md"`)
}

// TestWriteBodyText_RendersExtendsChain verifies plan-135 surface:
// `kinds show` prints the parent and the resolved chain so the
// inheritance is auditable without re-reading every schema.
func TestWriteBodyText_RendersExtendsChain(t *testing.T) {
	kinds := map[string]config.KindBody{
		"rfc-base": {Schema: config.InlineSchema(map[string]any{"frontmatter": map[string]any{
			"id": `=~"^RFC-[0-9]{4}$"`,
		}})},
		"rfc-ratified": {
			Extends: "rfc-base",
			Schema: config.InlineSchema(map[string]any{"frontmatter": map[string]any{
				"status": `"ratified"`,
			}}),
		},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "rfc-ratified", kinds["rfc-ratified"], kinds))
	out := buf.String()
	assert.Contains(t, out, "rfc-ratified:")
	assert.Contains(t, out, "extends: rfc-base")
	assert.Contains(t, out, "effective-frontmatter:")
	assert.Contains(t, out, "id:")
	assert.Contains(t, out, "from rfc-base")
	assert.Contains(t, out, "status:")
	assert.Contains(t, out, "from rfc-ratified")
}

// TestWriteBodyText_RendersExtendsChainMultiHop checks the chain
// line for deeper inheritance: the audit trail shows every layer
// in child-first order.
func TestWriteBodyText_RendersExtendsChainMultiHop(t *testing.T) {
	kinds := map[string]config.KindBody{
		"a": {Schema: config.InlineSchema(map[string]any{"frontmatter": map[string]any{"a": "string"}})},
		"b": {Extends: "a"},
		"c": {Extends: "b"},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "c", kinds["c"], kinds))
	out := buf.String()
	assert.Contains(t, out, "extends: b")
	assert.Contains(t, out, "extends-chain: c -> b -> a")
}

// TestWriteBodyText_NoExtendsOmitsHeader confirms backwards
// compatibility: a kind without extends does not gain the extends
// header lines.
func TestWriteBodyText_NoExtendsOmitsHeader(t *testing.T) {
	kinds := map[string]config.KindBody{
		"plan": {Schema: config.InlineSchema(map[string]any{"filename": "plan-*.md"})},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "plan", kinds["plan"], kinds))
	out := buf.String()
	assert.NotContains(t, out, "extends:")
	assert.NotContains(t, out, "extends-chain")
}

// TestMakeBodyJSON_PopulatesExtendsAndProvenance pins the JSON
// shape: extends + chain + per-leaf provenance.
func TestMakeBodyJSON_PopulatesExtendsAndProvenance(t *testing.T) {
	kinds := map[string]config.KindBody{
		"rfc-base": {Schema: config.InlineSchema(map[string]any{"frontmatter": map[string]any{
			"id": `=~"^RFC-[0-9]{4}$"`,
		}})},
		"rfc-ratified": {
			Extends: "rfc-base",
			Schema: config.InlineSchema(map[string]any{"frontmatter": map[string]any{
				"status": `"ratified"`,
			}}),
		},
	}
	out := MakeBodyJSON("rfc-ratified", kinds["rfc-ratified"], kinds)
	assert.Equal(t, "rfc-base", out.Extends)
	assert.Equal(t, []string{"rfc-ratified", "rfc-base"}, out.ExtendsChain)
	require.Len(t, out.EffectiveFrontmatter, 2)
	leafBySrc := map[string]FrontmatterLeafJSON{}
	for _, leaf := range out.EffectiveFrontmatter {
		leafBySrc[leaf.Source] = leaf
	}
	assert.Equal(t, "id", leafBySrc["rfc-base"].Key)
	assert.Equal(t, "status", leafBySrc["rfc-ratified"].Key)
}

// TestMakeBodyJSON_NilKindsMapOmitsExtendsMetadata covers the
// fallback for callers that don't have the full kinds map.
func TestMakeBodyJSON_NilKindsMapOmitsExtendsMetadata(t *testing.T) {
	body := config.KindBody{Extends: "base"}
	out := MakeBodyJSON("child", body, nil)
	assert.Equal(t, "base", out.Extends, "the kind's own extends field is preserved")
	assert.Empty(t, out.ExtendsChain, "chain requires the kinds map")
	assert.Empty(t, out.EffectiveFrontmatter, "provenance requires the kinds map")
}

// extendsKindsForTest builds the canonical two-kind map used by
// the writer-error tests so each test isn't a long inline literal.
func extendsKindsForTest() map[string]config.KindBody {
	return map[string]config.KindBody{
		"base": {Schema: config.InlineSchema(map[string]any{"frontmatter": map[string]any{
			"id": "string",
		}})},
		"child": {Extends: "base", Schema: config.InlineSchema(map[string]any{
			"frontmatter": map[string]any{"status": `"ratified"`},
		})},
	}
}

// TestWriteBodyText_ExtendsHeaderWriteError exercises the
// writer-error branch on the `extends:` line: the second write
// fails after the header line printed, so the error must surface
// without crashing.
func TestWriteBodyText_ExtendsHeaderWriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("disk full"), after: 1}
	kinds := extendsKindsForTest()
	err := WriteBodyText(w, "child", kinds["child"], kinds)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

// TestWriteBodyText_ExtendsChainWriteError trips the failure on
// the `extends-chain` line specifically. The chain prints only
// when its length exceeds one.
func TestWriteBodyText_ExtendsChainWriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("disk full"), after: 2}
	kinds := extendsKindsForTest()
	err := WriteBodyText(w, "child", kinds["child"], kinds)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

// TestWriteBodyText_EffectiveFrontmatterHeaderWriteError trips
// the failure on the `effective-frontmatter:` label line.
func TestWriteBodyText_EffectiveFrontmatterHeaderWriteError(t *testing.T) {
	// after = 4 lets the kind header, extends line, extends-chain
	// line, and body YAML through; the next write — the
	// `effective-frontmatter:` label — must trigger the error.
	w := &failingWriter{err: errors.New("disk full"), after: 4}
	kinds := extendsKindsForTest()
	err := WriteBodyText(w, "child", kinds["child"], kinds)
	require.Error(t, err)
}

// TestWriteBodyText_EffectiveFrontmatterLeafWriteError trips the
// failure on a leaf line under `effective-frontmatter:`.
func TestWriteBodyText_EffectiveFrontmatterLeafWriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("disk full"), after: 5}
	kinds := extendsKindsForTest()
	err := WriteBodyText(w, "child", kinds["child"], kinds)
	require.Error(t, err)
}

// TestEffectiveFrontmatterLeaves_ResolverErrorReturnsNil covers
// the err-branch in effectiveFrontmatterLeaves: a malformed kinds
// map (cycle) makes ResolveKindInlineSchema return an error, and
// the renderer treats that as "no leaves to report".
func TestEffectiveFrontmatterLeaves_ResolverErrorReturnsNil(t *testing.T) {
	kinds := map[string]config.KindBody{
		"a": {Extends: "b", Schema: config.InlineSchema(map[string]any{"frontmatter": map[string]any{
			"x": "string",
		}})},
		"b": {Extends: "a", Schema: config.InlineSchema(map[string]any{"frontmatter": map[string]any{
			"y": "string",
		}})},
	}
	out := effectiveFrontmatterLeaves(kinds, "a")
	assert.Nil(t, out)
}

// TestEffectiveFrontmatterLeaves_NoFrontmatterReturnsNil covers
// the empty-frontmatter branch: a resolved schema without any
// frontmatter keys contributes nothing to the audit list.
func TestEffectiveFrontmatterLeaves_NoFrontmatterReturnsNil(t *testing.T) {
	kinds := map[string]config.KindBody{
		"a": {Schema: config.InlineSchema(map[string]any{"filename": "x.md"})},
	}
	out := effectiveFrontmatterLeaves(kinds, "a")
	assert.Nil(t, out)
}

// TestWriteExtendsHeader_NilKindsReturnsAfterExtendsLine covers
// the unit-level branch where the kinds map is nil: the header
// still writes the `extends:` line but skips the chain.
func TestWriteExtendsHeader_NilKindsReturnsAfterExtendsLine(t *testing.T) {
	var buf bytes.Buffer
	body := config.KindBody{Extends: "base"}
	require.NoError(t, writeExtendsHeader(&buf, "child", body, nil))
	out := buf.String()
	assert.Contains(t, out, "extends: base")
	assert.NotContains(t, out, "extends-chain")
}

// TestWriteEffectiveFrontmatter_EmptyLeavesNoOutput exercises
// the early return when no leaves resolve: the function returns
// nil without writing anything.
func TestWriteEffectiveFrontmatter_EmptyLeavesNoOutput(t *testing.T) {
	kinds := map[string]config.KindBody{
		"a": {Schema: config.InlineSchema(map[string]any{"filename": "x.md"})},
	}
	var buf bytes.Buffer
	require.NoError(t, writeEffectiveFrontmatter(&buf, kinds, "a"))
	assert.Empty(t, buf.String())
}

// TestWriteEffectiveFrontmatter_LabelWriteError fails before any
// leaf line is written.
func TestWriteEffectiveFrontmatter_LabelWriteError(t *testing.T) {
	kinds := extendsKindsForTest()
	w := &failingWriter{err: errors.New("disk full"), after: 0}
	err := writeEffectiveFrontmatter(w, kinds, "child")
	require.Error(t, err)
}

// TestWriteEffectiveFrontmatter_LeafLineWriteError fails on the
// first leaf line after the label printed successfully.
func TestWriteEffectiveFrontmatter_LeafLineWriteError(t *testing.T) {
	kinds := extendsKindsForTest()
	w := &failingWriter{err: errors.New("disk full"), after: 1}
	err := writeEffectiveFrontmatter(w, kinds, "child")
	require.Error(t, err)
}

func TestWriteBodyText_EmptyBodyRendersPlaceholder(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "ghost", config.KindBody{}, nil))
	out := buf.String()
	assert.Contains(t, out, "ghost:")
	assert.Contains(t, out, "(empty)")
}

func TestWriteBodyText_HeaderWriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("disk full"), after: 0}
	err := WriteBodyText(w, "plan", config.KindBody{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

func TestWriteBodyText_BodyWriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("nope"), after: 1}
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{"x": {Enabled: true}},
	}
	err := WriteBodyText(w, "plan", body, nil)
	require.Error(t, err)
}

func TestWriteBodyText_EmptyPlaceholderWriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("nope"), after: 1}
	err := WriteBodyText(w, "ghost", config.KindBody{}, nil)
	require.Error(t, err)
}

// TestWriteBodyText_DefinedInWriteError pins the error path
// on the SourcePath `defined-in:` line — surfaces a write
// error rather than swallowing it.
func TestWriteBodyText_DefinedInWriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("nope"), after: 1}
	body := config.KindBody{
		SourcePath: "/repo/.mdsmith/kinds/foo.yaml",
	}
	err := WriteBodyText(w, "foo", body, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
}

// makeFileResolution builds a minimal FileResolution with one rule and
// two layers so writers exercise both the kinds and rules branches.
func makeFileResolution(t *testing.T) *config.FileResolution {
	t.Helper()
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]config.KindBody{
			"short": {Rules: map[string]config.RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 30}},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"x.md"}, Kinds: []string{"short"}},
		},
	}
	return config.ResolveFile(cfg, "x.md", nil, nil)
}

func TestWriteFileResolutionText_Full(t *testing.T) {
	res := makeFileResolution(t)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	out := buf.String()
	assert.Contains(t, out, "file: x.md")
	assert.Contains(t, out, "short (from kind-assignment[0]: glob x.md)")
	assert.Contains(t, out, "line-length")
	assert.Contains(t, out, "settings.max = 30")
	assert.Contains(t, out, "(from kinds.short)")
}

// TestWriteFileResolutionText_RendersSourcePath pins plan 208's
// CLI surface: a file resolution carrying a SourcePath on each
// kind prints `defined-in <path>` after the assignment metadata.
// The format is "<name> (from <source>)<space>defined-in <path>"
// — the path lives outside the `from (...)` parens so the
// existing `(from ...)` substring assertions in the e2e tests
// keep matching.
func TestWriteFileResolutionText_RendersSourcePath(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 30}},
		},
		Kinds: map[string]config.KindBody{
			"audit-log": {
				Rules: map[string]config.RuleCfg{
					"line-length": {Enabled: true, Settings: map[string]any{"max": 30}},
				},
				SourcePath: "/repo/.mdsmith/kinds/audit-log.yaml",
			},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Glob: []string{"x.md"}, Kinds: []string{"audit-log"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	out := buf.String()
	assert.Contains(t, out,
		"audit-log (from kind-assignment[0]: glob x.md) defined-in /repo/.mdsmith/kinds/audit-log.yaml")
}

// TestFileResolutionJSON_IncludesSourcePath pins the
// `source-path` field in JSON output. LSP and audit tooling
// keys off this stable field rather than parsing text.
func TestFileResolutionJSON_IncludesSourcePath(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{"line-length": {Enabled: true}},
		Kinds: map[string]config.KindBody{
			"audit-log": {SourcePath: "/repo/.mdsmith/kinds/audit-log.yaml"},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Glob: []string{"x.md"}, Kinds: []string{"audit-log"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	out := FileResolution(res)
	require.Len(t, out.Kinds, 1)
	assert.Equal(t, "/repo/.mdsmith/kinds/audit-log.yaml",
		out.Kinds[0].SourcePath)
}

// TestFileResolutionJSON_IncludesSchemaSourcePath pins plan 241's CLI
// surface: a kind that references a `.mdsmith/schemas/` schema by name
// carries `schema-source-path` (the schema's .yaml path) in JSON,
// distinct from `source-path` (the kind's own file).
func TestFileResolutionJSON_IncludesSchemaSourcePath(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{},
		Kinds: map[string]config.KindBody{
			"rfc": {
				SourcePath: "/repo/.mdsmith.yml",
				Schema: config.InlineSchemaWithSource(
					"rfc-v1",
					map[string]any{"filename": "RFC-*.md"},
					"/repo/.mdsmith/schemas/rfc-v1.yaml"),
			},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Glob: []string{"x.md"}, Kinds: []string{"rfc"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	out := FileResolution(res)
	require.Len(t, out.Kinds, 1)
	assert.Equal(t, "/repo/.mdsmith.yml", out.Kinds[0].SourcePath)
	assert.Equal(t, "/repo/.mdsmith/schemas/rfc-v1.yaml",
		out.Kinds[0].SchemaSourcePath)
}

// TestFileResolutionJSON_OmitsSchemaSourcePathForInline pins that an
// inline-on-kind schema leaves `schema-source-path` empty (omitted in
// JSON) — `source-path` already names the defining file.
func TestFileResolutionJSON_OmitsSchemaSourcePathForInline(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{},
		Kinds: map[string]config.KindBody{
			"rfc": {
				SourcePath: "/repo/.mdsmith.yml",
				Schema:     config.InlineSchema(map[string]any{"filename": "RFC-*.md"}),
			},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Glob: []string{"x.md"}, Kinds: []string{"rfc"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	out := FileResolution(res)
	require.Len(t, out.Kinds, 1)
	assert.Empty(t, out.Kinds[0].SchemaSourcePath)
}

// TestWriteFileResolutionText_RendersSchemaSource pins the text
// surface: a named-YAML schema prints ` schema-in <path>` after the
// kind's `defined-in` clause.
func TestWriteFileResolutionText_RendersSchemaSource(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{},
		Kinds: map[string]config.KindBody{
			"rfc": {
				SourcePath: "/repo/.mdsmith.yml",
				Schema: config.InlineSchemaWithSource(
					"rfc-v1",
					map[string]any{"filename": "RFC-*.md"},
					"/repo/.mdsmith/schemas/rfc-v1.yaml"),
			},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Glob: []string{"x.md"}, Kinds: []string{"rfc"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	assert.Contains(t, buf.String(),
		"defined-in /repo/.mdsmith.yml schema-in /repo/.mdsmith/schemas/rfc-v1.yaml")
}

// TestWriteFileResolutionText_RendersConvention pins plan 209's CLI
// surface: when a user convention is active, the resolution text adds a
// `convention: <name> (user) defined-in <path>` line so a reader can
// jump to the defining file (parallel to a kind's `defined-in`).
func TestWriteFileResolutionText_RendersConvention(t *testing.T) {
	cfg := &config.Config{
		Convention: "portable-strict",
		Conventions: map[string]config.UserConvention{
			"portable-strict": {
				SourcePath: "/repo/.mdsmith/conventions/portable-strict.yaml",
			},
		},
		Rules: map[string]config.RuleCfg{"line-length": {Enabled: true}},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	assert.Contains(t, buf.String(),
		"convention: portable-strict (user) defined-in /repo/.mdsmith/conventions/portable-strict.yaml")
}

// TestWriteFileResolutionText_BuiltinConventionHasNoPath pins that a
// built-in convention is reported by name with no `defined-in` suffix
// and no `(user)` tag — built-ins are compiled in and carry no file.
func TestWriteFileResolutionText_BuiltinConventionHasNoPath(t *testing.T) {
	cfg := &config.Config{
		Convention: "github",
		Rules:      map[string]config.RuleCfg{"line-length": {Enabled: true}},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	out := buf.String()
	assert.Contains(t, out, "convention: github")
	assert.NotContains(t, out, "defined-in")
	assert.NotContains(t, out, "(user)")
}

// TestFileResolutionJSON_IncludesConvention pins the `convention`
// object in JSON output (name + user + source-path) — the stable field
// LSP and audit tooling key off rather than parsing text.
func TestFileResolutionJSON_IncludesConvention(t *testing.T) {
	cfg := &config.Config{
		Convention: "portable-strict",
		Conventions: map[string]config.UserConvention{
			"portable-strict": {
				SourcePath: "/repo/.mdsmith/conventions/portable-strict.yaml",
			},
		},
		Rules: map[string]config.RuleCfg{"line-length": {Enabled: true}},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	out := FileResolution(res)
	require.NotNil(t, out.Convention)
	assert.Equal(t, "portable-strict", out.Convention.Name)
	assert.True(t, out.Convention.User)
	assert.Equal(t, "/repo/.mdsmith/conventions/portable-strict.yaml",
		out.Convention.SourcePath)
}

// TestFileResolutionJSON_NoConventionOmitsField pins that the
// `convention` object is absent when no convention is selected.
func TestFileResolutionJSON_NoConventionOmitsField(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{"line-length": {Enabled: true}},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	out := FileResolution(res)
	assert.Nil(t, out.Convention)
}

// TestMakeBodyJSON_IncludesSourcePath pins the new `source-path`
// JSON key on the body output for `kinds list` / `kinds show`.
func TestMakeBodyJSON_IncludesSourcePath(t *testing.T) {
	body := config.KindBody{
		Rules:      map[string]config.RuleCfg{"line-length": {Enabled: true}},
		SourcePath: "/repo/.mdsmith/kinds/audit-log.yaml",
	}
	out := MakeBodyJSON("audit-log", body, nil)
	assert.Equal(t, "/repo/.mdsmith/kinds/audit-log.yaml", out.SourcePath)
}

// TestWriteBodyText_RendersDefinedIn pins the text surface for
// `kinds show` / `kinds list`: the source path appears on a
// `defined-in:` line right under the kind name.
func TestWriteBodyText_RendersDefinedIn(t *testing.T) {
	body := config.KindBody{
		Rules:      map[string]config.RuleCfg{"line-length": {Enabled: true}},
		SourcePath: "/repo/.mdsmith/kinds/audit-log.yaml",
	}
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "audit-log", body, nil))
	assert.Contains(t, buf.String(),
		"defined-in: /repo/.mdsmith/kinds/audit-log.yaml")
}

func TestWriteFileResolutionText_NoKinds(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{"line-length": {Enabled: true}},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	assert.Contains(t, buf.String(), "(none)")
}

func TestWriteFileResolutionText_WriteErrorPropagates(t *testing.T) {
	res := makeFileResolution(t)
	for after := 0; after < 6; after++ {
		w := &failingWriter{err: errors.New("io"), after: after}
		err := WriteFileResolutionText(w, res)
		assert.Error(t, err, "expected error for write #%d", after)
	}
}

func TestWriteRuleResolutionText_FullAndNoOpLayers(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length":           {Enabled: true, Settings: map[string]any{"max": 80}},
			"paragraph-readability": {Enabled: true},
		},
		Kinds: map[string]config.KindBody{
			// short does not touch line-length, so it appears as a no-op layer.
			"short": {Rules: map[string]config.RuleCfg{
				"paragraph-readability": {Enabled: false},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"x.md"}, Kinds: []string{"short"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	rr := res.Rules["line-length"]
	var buf bytes.Buffer
	require.NoError(t, WriteRuleResolutionText(&buf, "x.md", rr))
	out := buf.String()
	assert.Contains(t, out, "rule: line-length")
	assert.Contains(t, out, "default")
	assert.Contains(t, out, "no-op")
	assert.Contains(t, out, "kinds.short")
	assert.Contains(t, out, "winning source: default")
}

func TestWriteRuleResolutionText_WriteErrorPropagates(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"r": {Enabled: true, Settings: map[string]any{"v": 1}},
		},
		Kinds: map[string]config.KindBody{
			"k": {Rules: map[string]config.RuleCfg{
				"r": {Enabled: true, Settings: map[string]any{"v": 2}},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"x.md"}, Kinds: []string{"k"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	rr := res.Rules["r"]
	for after := 0; after < 8; after++ {
		w := &failingWriter{err: errors.New("io"), after: after}
		err := WriteRuleResolutionText(w, "x.md", rr)
		assert.Error(t, err, "expected error for write #%d", after)
	}
}

func TestWriteJSON_RendersIndentedJSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteJSON(&buf, map[string]int{"a": 1}))
	assert.Contains(t, buf.String(), "\"a\": 1")
	assert.Equal(t, "\n", buf.String()[len(buf.String())-1:])
}

func TestWriteJSON_EncodingError(t *testing.T) {
	// channels are not encodable
	err := WriteJSON(&bytes.Buffer{}, make(chan int))
	require.Error(t, err)
}

func TestWriteJSON_WriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("io"), after: 0}
	err := WriteJSON(w, map[string]int{"a": 1})
	require.Error(t, err)
}

func TestFormatValue_Scalars(t *testing.T) {
	assert.Equal(t, "30", FormatValue(30))
	assert.Equal(t, "true", FormatValue(true))
	assert.Equal(t, "\"hi\"", FormatValue("hi"))
	assert.Equal(t, "null", FormatValue(nil))
}

func TestFormatValue_FallbackForUnmarshalable(t *testing.T) {
	// channel is not JSON-encodable; FormatValue falls back to %v,
	// which prints the channel's pointer address.
	out := FormatValue(make(chan int))
	assert.NotEmpty(t, out)
	assert.True(t, strings.HasPrefix(out, "0x"),
		"expected pointer-like fallback, got %q", out)
}

func TestFileResolutionJSON_Shape(t *testing.T) {
	res := makeFileResolution(t)
	out := FileResolution(res)
	assert.Equal(t, "x.md", out.File)
	require.Len(t, out.Kinds, 1)
	assert.Equal(t, "short", out.Kinds[0].Name)
	assert.Equal(t, "kind-assignment[0]", out.Kinds[0].Source)
	rr, ok := out.Rules["line-length"]
	require.True(t, ok)
	var sawMax bool
	for _, l := range rr.Leaves {
		if l.Path == "settings.max" {
			sawMax = true
			assert.Equal(t, "kinds.short", l.Source)
		}
	}
	assert.True(t, sawMax)
}

func TestRuleResolutionJSON_IncludesNoOpLayers(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length":           {Enabled: true, Settings: map[string]any{"max": 80}},
			"paragraph-readability": {Enabled: true},
		},
		Kinds: map[string]config.KindBody{
			"short": {Rules: map[string]config.RuleCfg{
				"paragraph-readability": {Enabled: false},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"x.md"}, Kinds: []string{"short"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	rr := res.Rules["line-length"]
	out := RuleResolution("x.md", rr)
	require.Len(t, out.Layers, 2)
	assert.Equal(t, "default", out.Layers[0].Source)
	assert.True(t, out.Layers[0].Set)
	assert.Equal(t, "kinds.short", out.Layers[1].Source)
	assert.False(t, out.Layers[1].Set, "no-op layer for line-length")
}

func TestLeavesJSON_PreservesChain(t *testing.T) {
	res := makeFileResolution(t)
	rr := res.Rules["line-length"]
	out := RuleResolution("x.md", rr)
	for _, l := range out.Leaves {
		if l.Path == "settings.max" {
			require.Len(t, l.Chain, 2)
			assert.Equal(t, "default", l.Chain[0].Source)
			assert.Equal(t, "kinds.short", l.Chain[1].Source)
			return
		}
	}
	t.Fatalf("settings.max leaf missing from %v", out.Leaves)
}

// erroringYAMLMarshaler implements yaml.Marshaler with a method that
// always returns an error so we can drive yaml.Marshal down its
// error-return path.
type erroringYAMLMarshaler struct{}

func (erroringYAMLMarshaler) MarshalYAML() (any, error) {
	return nil, errors.New("synthetic marshal error")
}

// TestWriteBodyText_YAMLMarshalError covers the yaml.Marshal failure
// branch: a setting whose value implements yaml.Marshaler and returns
// an error makes yaml.Marshal surface that error rather than panicking.
func TestWriteBodyText_YAMLMarshalError(t *testing.T) {
	var buf bytes.Buffer
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"x": {
				Enabled:  true,
				Settings: map[string]any{"bad": erroringYAMLMarshaler{}},
			},
		},
	}
	err := WriteBodyText(&buf, "k", body, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "synthetic marshal error")
}

// TestWriteFileResolutionText_NoneWriteError covers the (none) Fprintln
// error branch: an empty kinds list combined with a writer that fails
// after the first two writes.
func TestWriteFileResolutionText_NoneWriteError(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{"r": {Enabled: true}},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	w := &failingWriter{err: errors.New("io"), after: 2}
	err := WriteFileResolutionText(w, res)
	require.Error(t, err)
}

// TestSanitizeControl_StripsCtrls covers the sanitizeControl helper.
func TestSanitizeControl_StripsCtrls(t *testing.T) {
	assert.Equal(t, "ab", sanitizeControl("a\nb"))     // C0: LF
	assert.Equal(t, "ab", sanitizeControl("a\x07b"))   // C0: BEL
	assert.Equal(t, "ab", sanitizeControl("a\x1bb"))   // C0: ESC
	assert.Equal(t, "ab", sanitizeControl("a\u009fb")) // C1: U+009F
	assert.Equal(t, "hello", sanitizeControl("hello"))
}

// TestWriteBodyText_SanitizesKindName ensures control chars in the kind
// name are stripped from the header line.
func TestWriteBodyText_SanitizesKindName(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "evil\nkind", config.KindBody{}, nil))
	assert.NotContains(t, buf.String(), "\n\n") // no extra blank line from injected newline
	assert.Contains(t, buf.String(), "evilkind:")
}

// TestWriteFileResolutionText_SanitizesKindName ensures control chars
// in kind names (from user YAML) are stripped from the text output.
func TestWriteFileResolutionText_SanitizesKindName(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true},
		},
		Kinds: map[string]config.KindBody{
			"evil\x1bkind": {Rules: map[string]config.RuleCfg{
				"line-length": {Enabled: true},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"x.md"}, Kinds: []string{"evil\x1bkind"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	assert.NotContains(t, buf.String(), "\x1b")
	assert.Contains(t, buf.String(), "evilkind")
}

// TestWriteRuleResolutionText_SanitizesFields ensures control chars in
// file/rule/source fields are stripped from the text output.
func TestWriteRuleResolutionText_SanitizesFields(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line\x07length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}
	res := config.ResolveFile(cfg, "evil\x07file.md", nil, nil)
	rr := res.Rules["line\x07length"]
	var buf bytes.Buffer
	require.NoError(t, WriteRuleResolutionText(&buf, "evil\x07file.md", rr))
	out := buf.String()
	assert.NotContains(t, out, "\x07")
	assert.Contains(t, out, "linelength")  // BEL stripped from rule name
	assert.Contains(t, out, "evilfile.md") // BEL stripped from file name
}

// Ensure WriteBodyText output is sorted deterministically.
func TestWriteBodyText_DeterministicOutput(t *testing.T) {
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"a": {Enabled: false},
			"b": {Enabled: false},
			"c": {Enabled: false},
		},
	}
	var first, second bytes.Buffer
	require.NoError(t, WriteBodyText(&first, "plan", body, nil))
	require.NoError(t, WriteBodyText(&second, "plan", body, nil))
	assert.Equal(t, first.String(), second.String())
	// All three names appear.
	for _, name := range []string{"a", "b", "c"} {
		assert.True(t, strings.Contains(first.String(), name))
	}
}

func TestWriteFileResolutionText_ShowsConventionLayer(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 120}},
		},
		ExplicitRules: map[string]bool{"line-length": true},
		Convention:    "portable",
		ConventionPreset: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	out := buf.String()
	// User's max=120 wins because the user layer sits above the
	// convention layer in the merge chain.
	assert.Contains(t, out, "settings.max = 120")
	assert.Contains(t, out, "(from user)")
}

func TestWriteRuleResolutionText_ShowsConventionLayer(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 120}},
		},
		ExplicitRules: map[string]bool{"line-length": true},
		Convention:    "portable",
		ConventionPreset: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil, nil)
	rr := res.Rules["line-length"]
	var buf bytes.Buffer
	require.NoError(t, WriteRuleResolutionText(&buf, "x.md", rr))
	out := buf.String()
	assert.Contains(t, out, "convention.portable",
		"convention layer must appear in chain")
	assert.Contains(t, out, "winning source: user",
		"user value wins over convention preset")
}
