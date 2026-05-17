package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/extract/encode"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestEmit(t *testing.T) {
	var buf bytes.Buffer
	assert.Equal(t, 0, emit(&buf, encode.JSON, map[string]any{"a": 1}))
	assert.Contains(t, buf.String(), `"a"`)

	// Encode error.
	orig := extractEncode
	extractEncode = func(encode.Format, any) ([]byte, error) {
		return nil, errors.New("boom")
	}
	assert.Equal(t, 2, emit(&buf, encode.JSON, nil))
	extractEncode = orig

	// Write error.
	assert.Equal(t, 2, emit(failWriter{}, encode.JSON, map[string]any{}))
}

func TestGateResultCode(t *testing.T) {
	// Errors only → 2.
	assert.Equal(t, 2, gateResultCode(&engine.Result{
		FilesChecked: 1, Errors: []error{errors.New("x")},
	}))
	// Diagnostics only → 1.
	assert.Equal(t, 1, gateResultCode(&engine.Result{
		FilesChecked: 1, Diagnostics: []lint.Diagnostic{{Message: "m"}},
	}))
	// Errors AND diagnostics → 1, but errors are still printed
	// (not dropped).
	assert.Equal(t, 1, gateResultCode(&engine.Result{
		FilesChecked: 1,
		Errors:       []error{errors.New("cfg boom")},
		Diagnostics:  []lint.Diagnostic{{Message: "m"}},
	}))
	// A clean pass requires the file to have actually been checked.
	assert.Equal(t, 0, gateResultCode(&engine.Result{FilesChecked: 1}))
	// An empty Result means the file was skipped (ignored): reject.
	assert.Equal(t, 2, gateResultCode(&engine.Result{}))
}

func TestGateExtractCheck_ErrorsOnly(t *testing.T) {
	orig := extractGateRun
	extractGateRun = func(*engine.Runner, string) *engine.Result {
		return &engine.Result{Errors: []error{errors.New("engine boom")}}
	}
	defer func() { extractGateRun = orig }()
	assert.Equal(t, 2, gateExtractCheck(&config.Config{}, "", "p.md", 1<<20))
}

func TestLoadExtractFile_ReadAndParseErrors(t *testing.T) {
	cfg := &config.Config{}

	origRead := extractReadFile
	extractReadFile = func(string, int64) ([]byte, error) {
		return nil, errors.New("no such file")
	}
	_, _, code := loadExtractFile(cfg, "", "p.md", 1)
	assert.Equal(t, 2, code)
	extractReadFile = origRead

	// Read succeeds (seam returns bytes) so the parse branch is
	// the one exercised.
	extractReadFile = func(string, int64) ([]byte, error) {
		return []byte("# T\n"), nil
	}
	origNew := extractNewFile
	extractNewFile = func(string, []byte, bool) (*lint.File, error) {
		return nil, errors.New("parse fail")
	}
	_, _, code = loadExtractFile(cfg, "", "p.md", 1)
	assert.Equal(t, 2, code)
	extractNewFile = origNew
	extractReadFile = origRead
}

func TestDecodeDocFrontMatter(t *testing.T) {
	disabled := false
	fm, code := decodeDocFrontMatter(
		&config.Config{FrontMatter: &disabled}, []byte("anything"), "p.md")
	assert.Equal(t, 0, code)
	assert.Nil(t, fm)

	// Enabled, no front matter block.
	fm, code = decodeDocFrontMatter(&config.Config{}, []byte("# T\n"), "p.md")
	assert.Equal(t, 0, code)
	assert.Nil(t, fm)

	// Enabled, valid mapping front matter.
	fm, code = decodeDocFrontMatter(&config.Config{},
		[]byte("---\nid: x\n---\n# T\n"), "p.md")
	assert.Equal(t, 0, code)
	assert.Equal(t, map[string]any{"id": "x"}, fm)

	// Enabled, non-mapping front matter is a hard error.
	_, code = decodeDocFrontMatter(&config.Config{},
		[]byte("---\n- a\n- b\n---\n# T\n"), "p.md")
	assert.Equal(t, 2, code)
}

func TestComposedSchemaFor_ApplySettingsAndComposeErrors(t *testing.T) {
	f, err := lint.NewFile("doc.md", []byte("# T\n"))
	require.NoError(t, err)

	// Mixing schema + inline-schema is rejected by ApplySettings.
	badApply := &config.FileResolution{
		Rules: map[string]config.RuleResolution{
			"required-structure": {Final: config.RuleCfg{
				Enabled: true,
				Settings: map[string]any{
					"schema": "x.md",
					"inline-schema": map[string]any{
						"sections": []any{},
					},
				},
			}},
		},
	}
	_, code := composedSchemaFor(f, badApply, "k")
	assert.Equal(t, 2, code)

	// A schema-sources file that cannot be loaded makes
	// ComposedSchema return an error.
	missing := &config.FileResolution{
		Rules: map[string]config.RuleResolution{
			"required-structure": {Final: config.RuleCfg{
				Enabled: true,
				Settings: map[string]any{
					"schema-sources": []any{
						map[string]any{"file": "definitely-missing-proto.md"},
					},
				},
			}},
		},
	}
	_, code = composedSchemaFor(f, missing, "k")
	assert.Equal(t, 2, code)
}
