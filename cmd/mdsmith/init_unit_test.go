package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/pack"
)

// --- printInitCatalog ---

func TestPrintInitCatalog(t *testing.T) {
	var buf bytes.Buffer
	printInitCatalog(&buf)
	out := buf.String()
	assert.Contains(t, out, "Starters (mdsmith init --starter <name>):")
	assert.Contains(t, out, "Packs (mdsmith init --add <name>):")
	assert.Contains(t, out, "okf")
	assert.Contains(t, out, "Open Knowledge Format bundle config")
	assert.Contains(t, out, "wordlists")
	assert.Contains(t, out, "Curated no-llm-tells word-lists")
	assert.Contains(t, out, "apm")
	assert.Contains(t, out, "APM kind pack")
}

// --- setInitUsage ---

func TestSetInitUsage(t *testing.T) {
	var buf bytes.Buffer
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.BoolVar(new(bool), "force", false, "Overwrite an existing .mdsmith.yml instead of leaving it unchanged")
	setInitUsage(fs, &buf)
	fs.Usage()
	out := buf.String()
	assert.Contains(t, out, "--starter")
	assert.Contains(t, out, "--from-markdownlint")
	assert.Contains(t, out, "--add")
	assert.Contains(t, out, "--force")
	assert.Contains(t, out, "--list")
	// Pins that fs.PrintDefaults() ran: description only appears via PrintDefaults, not the static header.
	assert.Contains(t, out, "Overwrite an existing .mdsmith.yml instead of leaving it unchanged")
}

// --- runInit ---

func TestRunInit_ExtraArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runInit([]string{"extra"})
		assert.Equal(t, 2, code)
	})
}

func TestRunInit_CreatesConfigFile(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldWd) }()
	require.NoError(t, os.Chdir(dir))

	captureStderr(func() {
		code := runInit(nil)
		assert.Equal(t, 0, code)
	})

	data, err := os.ReadFile(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	// Verify it's parseable YAML
	var out map[string]any
	require.NoError(t, yaml.Unmarshal(data, &out))
}

func TestRunInit_AlreadyExists_SkipsAndExitsZero(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldWd) }()
	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.WriteFile(".mdsmith.yml", []byte("rules: {}\n"), 0644))

	// A bare re-run over an existing config is idempotent: it leaves the
	// config unchanged, notes it, and exits 0 (use --force to overwrite).
	var code int
	out := captureStderr(func() { code = runInit(nil) })
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "already exists")
	assert.Contains(t, out, "--force")
	cfg, err := os.ReadFile(".mdsmith.yml")
	require.NoError(t, err)
	assert.Equal(t, "rules: {}\n", string(cfg), "existing config left unchanged")
}

func TestRunInit_Add_Wordlists_ScaffoldsCuratedFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	captureStderr(func() {
		code := runInit([]string{"--add", "wordlists"})
		assert.Equal(t, 0, code)
	})

	// A fresh project: init writes the default config and --add wordlists
	// lands every curated list on disk. (Content is covered in the pack
	// package's TestWordlistsPack_Files.)
	_, err := os.Stat(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err, "default config written alongside the pack")
	for _, name := range []string{"ai-speak", "ai-openers"} {
		_, err := os.Stat(filepath.Join(dir, ".mdsmith", "wordlists", name+".yaml"))
		assert.NoErrorf(t, err, "%s.yaml scaffolded", name)
	}
}

func TestRunInit_Add_ExistingConfigScaffoldsAnyway(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// Already-initialized project: .mdsmith.yml exists. --add must still
	// scaffold and leave the config unchanged (exit 0, no --force needed).
	require.NoError(t, os.WriteFile(".mdsmith.yml", []byte("rules: {}\n"), 0o644))

	captureStderr(func() {
		code := runInit([]string{"--add", "wordlists"})
		assert.Equal(t, 0, code, "--add must work on an initialized project")
	})

	cfg, err := os.ReadFile(".mdsmith.yml")
	require.NoError(t, err)
	assert.Equal(t, "rules: {}\n", string(cfg), "existing config left unchanged")
	for _, name := range []string{"ai-speak", "ai-openers"} {
		_, err := os.Stat(filepath.Join(dir, ".mdsmith", "wordlists", name+".yaml"))
		assert.NoErrorf(t, err, "%s.yaml scaffolded", name)
	}
}

func TestRunInit_Starter_ExistingConfigSkips(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(".mdsmith.yml", []byte("rules: {}\n"), 0o644))

	// A config source over an existing config no longer errors: the config
	// is left unchanged with a notice and the run exits 0.
	var code int
	out := captureStderr(func() { code = runInit([]string{"--starter", "okf"}) })
	assert.Equal(t, 0, code, "--starter over an existing config skips, not errors")
	assert.Contains(t, out, "already exists")
	assert.Contains(t, out, "--force")
	cfg, err := os.ReadFile(".mdsmith.yml")
	require.NoError(t, err)
	assert.Equal(t, "rules: {}\n", string(cfg), "existing config left unchanged")
}

func TestRunInit_Force_Overwrites(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(".mdsmith.yml", []byte("rules: {}\n"), 0o644))

	captureStderr(func() {
		code := runInit([]string{"--starter", "okf", "--force"})
		assert.Equal(t, 0, code)
	})
	cfg, err := os.ReadFile(".mdsmith.yml")
	require.NoError(t, err)
	assert.Contains(t, string(cfg), "required-frontmatter", "--force overwrote with the okf starter")
}

func TestRunInit_StarterWithAdd_ExistingConfigSkipsConfigButScaffolds(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(".mdsmith.yml", []byte("rules: {}\n"), 0o644))

	// The config source is skipped over the existing file, but the additive
	// pack still applies — the two axes are independent, so a combined run
	// no longer aborts the way it once did.
	var code int
	captureStderr(func() { code = runInit([]string{"--starter", "okf", "--add", "wordlists"}) })
	assert.Equal(t, 0, code)
	cfg, err := os.ReadFile(".mdsmith.yml")
	require.NoError(t, err)
	assert.Equal(t, "rules: {}\n", string(cfg), "existing config left unchanged")
	for _, name := range []string{"ai-speak", "ai-openers"} {
		_, err := os.Stat(filepath.Join(dir, ".mdsmith", "wordlists", name+".yaml"))
		assert.NoErrorf(t, err, "%s scaffolded despite the config skip", name)
	}
}

func TestRunInit_FromMarkdownlintWithAdd_ExistingConfigSkipsConfigButScaffolds(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(".mdsmith.yml", []byte("rules: {}\n"), 0o644))

	// The other config source behaves identically: the existing config is
	// skipped (no markdownlint discovery even runs) while --add scaffolds.
	var code int
	captureStderr(func() { code = runInit([]string{"--from-markdownlint", "--add", "wordlists"}) })
	assert.Equal(t, 0, code)
	cfg, err := os.ReadFile(".mdsmith.yml")
	require.NoError(t, err)
	assert.Equal(t, "rules: {}\n", string(cfg), "existing config left unchanged")
	for _, name := range []string{"ai-speak", "ai-openers"} {
		_, err := os.Stat(filepath.Join(dir, ".mdsmith", "wordlists", name+".yaml"))
		assert.NoErrorf(t, err, "%s scaffolded despite the config skip", name)
	}
}

func TestRunInit_UnknownFlag_ExitsTwo(t *testing.T) {
	var code int
	out := captureStderr(func() { code = runInit([]string{"--definitely-not-a-flag"}) })
	assert.Equal(t, 2, code)
	assert.Contains(t, out, "unknown flag")
}

func TestRunInit_ConfigPathIsDir_WriteError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// .mdsmith.yml is a directory, which statTarget reports as absent, so
	// init proceeds to write and os.WriteFile fails with "is a directory" —
	// exercising writeInitConfig's write-error branch.
	require.NoError(t, os.Mkdir(".mdsmith.yml", 0o755))

	var code int
	out := captureStderr(func() { code = runInit([]string{"--force"}) })
	assert.Equal(t, 2, code)
	assert.Contains(t, out, "writing .mdsmith.yml")
}

func TestRunInit_SymlinkConfig_Refused(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// A symlinked .mdsmith.yml must not be written through, even with
	// --force: init refuses instead of following the link out of the tree.
	require.NoError(t, os.Symlink(filepath.Join(dir, "target.yml"), ".mdsmith.yml"))

	var code int
	out := captureStderr(func() { code = runInit([]string{"--force"}) })
	assert.Equal(t, 2, code)
	assert.Contains(t, out, "symlink")
	_, statErr := os.Stat(filepath.Join(dir, "target.yml"))
	assert.True(t, os.IsNotExist(statErr), "must not write through the symlink")
}

func TestRunInit_Add_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// pflag splits "wordlists, " on the comma into ["wordlists", " "]; the
	// blank entry and the surrounding space must be normalized away so the
	// pack still resolves instead of failing as an unknown name.
	var code int
	captureStderr(func() { code = runInit([]string{"--add", "wordlists, "}) })
	assert.Equal(t, 0, code)
	_, err := os.Stat(filepath.Join(dir, ".mdsmith", "wordlists", "ai-speak.yaml"))
	require.NoError(t, err, "trimmed pack name still scaffolds")
}

func TestRunInit_Add_UnknownPack_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var code int
	out := captureStderr(func() { code = runInit([]string{"--add", "bogus"}) })
	assert.Equal(t, 2, code)
	assert.Contains(t, out, `unknown pack "bogus"`)
	// Fails before writing anything: an invalid pack name never touches the
	// config or the .mdsmith directory.
	_, err := os.Stat(filepath.Join(dir, ".mdsmith.yml"))
	assert.True(t, os.IsNotExist(err), "no config written when a pack name is invalid")
}

func TestRunInit_Add_ScaffoldError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// .mdsmith is a regular file, so the pack's parent-directory checks
	// fail and runInit must surface the scaffold error as exit 2.
	require.NoError(t, os.WriteFile(".mdsmith", []byte("x"), 0o644))

	captureStderr(func() {
		code := runInit([]string{"--add", "wordlists"})
		assert.Equal(t, 2, code)
	})
}

func TestRunInit_List(t *testing.T) {
	out := captureStdout(func() {
		code := runInit([]string{"--list"})
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "Starters")
	assert.Contains(t, out, "okf")
	assert.Contains(t, out, "Packs")
	assert.Contains(t, out, "wordlists")
	assert.Contains(t, out, "apm")
}

// --- --apm flag ---

func TestRunInit_APM_FreshRepo_WritesKindFilesAndPosture(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	captureStderr(func() {
		code := runInit([]string{"--apm"})
		assert.Equal(t, 0, code)
	})

	// Config must exist with ignore: block for APM-deployed files.
	data, err := os.ReadFile(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "ignore:", "posture written to fresh config")
	assert.Contains(t, body, "apm_modules/**", "APM module cache always ignored")
	assert.Contains(t, body, "AGENTS.md", "compiled root file in ignore")
	assert.Contains(t, body, "CLAUDE.md", "compiled root file in ignore")
	assert.Contains(t, body, "GEMINI.md", "compiled root file in ignore")

	// The default config already has "ignore: []"; the APM flag must not add
	// a second "ignore:" key — that would produce invalid YAML.
	assert.Equal(t, 1, strings.Count(body, "\nignore:"),
		"exactly one ignore: key in the config (no duplicate from APM posture)")

	// All four kind files must be scaffolded.
	for _, name := range []string{"apm-skill", "apm-prompt", "apm-instruction", "apm-agent"} {
		_, statErr := os.Stat(filepath.Join(dir, ".mdsmith", "kinds", name+".yaml"))
		assert.NoErrorf(t, statErr, "%s.yaml must be scaffolded", name)
	}
}

func TestRunInit_APM_ExistingConfig_PrintsMergeHint(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(".mdsmith.yml", []byte("rules: {}\n"), 0o644))

	out := captureStderr(func() {
		code := runInit([]string{"--apm"})
		assert.Equal(t, 0, code)
	})

	// Merge hint must appear on stderr with the posture block.
	assert.Contains(t, out, "merge", "hint message must instruct user to merge")
	assert.Contains(t, out, "apm_modules/**", "merge hint includes ignore globs")
	assert.Contains(t, out, "AGENTS.md", "merge hint includes compiled root files")

	// Existing config must not be clobbered.
	cfg, err := os.ReadFile(".mdsmith.yml")
	require.NoError(t, err)
	assert.Equal(t, "rules: {}\n", string(cfg), "existing config left unchanged")
}

func TestRunInit_APM_ExistingConfig_KindFilesStillScaffolded(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(".mdsmith.yml", []byte("rules: {}\n"), 0o644))

	captureStderr(func() {
		code := runInit([]string{"--apm"})
		assert.Equal(t, 0, code)
	})

	// Kind files are scaffolded even when the config already exists.
	for _, name := range []string{"apm-skill", "apm-prompt", "apm-instruction", "apm-agent"} {
		_, statErr := os.Stat(filepath.Join(dir, ".mdsmith", "kinds", name+".yaml"))
		assert.NoErrorf(t, statErr, "%s.yaml must be scaffolded despite existing config", name)
	}
}

func TestRunInit_APM_ScopedToDetectedHarnessDirs(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// Create only the .github/ harness directory.
	require.NoError(t, os.Mkdir(".github", 0o755))

	captureStderr(func() {
		code := runInit([]string{"--apm"})
		assert.Equal(t, 0, code)
	})

	data, err := os.ReadFile(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	body := string(data)

	// .github/ is present, so GitHub harness paths must appear.
	assert.Contains(t, body, ".github/prompts/**")
	// .claude/ is absent, so Claude harness must not appear.
	assert.NotContains(t, body, ".claude/rules/**")
}

func TestRunInit_APM_NoHarnessDirs_OnlyRootFilesAndModules(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	captureStderr(func() {
		code := runInit([]string{"--apm"})
		assert.Equal(t, 0, code)
	})

	data, err := os.ReadFile(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	body := string(data)

	// Without any harness dirs, only the always-present entries must appear.
	assert.Contains(t, body, "apm_modules/**")
	assert.Contains(t, body, "AGENTS.md")
	// Harness-specific paths must be absent.
	assert.NotContains(t, body, ".github/prompts/**")
	assert.NotContains(t, body, ".claude/rules/**")
}

func TestRunInit_APM_Force_AppendsPostureToNewConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(".mdsmith.yml", []byte("rules: {}\n"), 0o644))

	captureStderr(func() {
		code := runInit([]string{"--apm", "--force"})
		assert.Equal(t, 0, code)
	})

	// --force overwrites, so the posture is written into the new config.
	data, err := os.ReadFile(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "apm_modules/**",
		"--apm --force must write posture to the new overwritten config")
	assert.Equal(t, 1, strings.Count(body, "\nignore:"),
		"--apm --force must not produce duplicate ignore: keys")
}

func TestRunInit_APM_WithAdd_BothApply(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	captureStderr(func() {
		code := runInit([]string{"--apm", "--add", "wordlists"})
		assert.Equal(t, 0, code)
	})

	// Kind files and wordlists both scaffolded.
	for _, name := range []string{"apm-skill", "apm-prompt", "apm-instruction", "apm-agent"} {
		_, statErr := os.Stat(filepath.Join(dir, ".mdsmith", "kinds", name+".yaml"))
		assert.NoErrorf(t, statErr, "%s.yaml scaffolded", name)
	}
	_, err := os.Stat(filepath.Join(dir, ".mdsmith", "wordlists", "ai-speak.yaml"))
	assert.NoError(t, err, "wordlists scaffolded alongside apm")
}

func TestRunInit_APM_AllHarnessDirs_AllGlobsPresent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// Create all six harness directories so every conditional glob is included.
	for _, d := range []string{".github", ".claude", ".agents", ".windsurf", ".kiro", ".cursor"} {
		require.NoError(t, os.Mkdir(d, 0o755))
	}

	captureStderr(func() {
		code := runInit([]string{"--apm"})
		assert.Equal(t, 0, code)
	})

	data, err := os.ReadFile(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, ".github/prompts/**")
	assert.Contains(t, body, ".claude/rules/**")
	assert.Contains(t, body, ".agents/skills/**")
	assert.Contains(t, body, ".windsurf/rules/**")
	assert.Contains(t, body, ".kiro/steering/**")
	assert.Contains(t, body, ".cursor/rules/**")
}

func TestSetInitUsage_IncludesAPMFlag(t *testing.T) {
	var buf bytes.Buffer
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.BoolVar(new(bool), "apm", false, "Scaffold the APM kind pack and coexistence posture")
	fs.BoolVar(new(bool), "force", false, "Overwrite an existing .mdsmith.yml instead of leaving it unchanged")
	setInitUsage(fs, &buf)
	fs.Usage()
	out := buf.String()
	assert.Contains(t, out, "--apm")
}

func TestRunInit_FromMarkdownlint_NoConfig_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// No markdownlint config to discover -> initConfigBytes errors ->
	// writeInitConfig propagates it -> exit 2.
	captureStderr(func() {
		code := runInit([]string{"--from-markdownlint"})
		assert.Equal(t, 2, code)
	})
}

func TestRunInit_FromMarkdownlint_Converts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(".markdownlint.json", []byte(`{"MD013": false}`), 0o644))

	captureStderr(func() {
		code := runInit([]string{"--from-markdownlint"})
		assert.Equal(t, 0, code)
	})
	// The converted config ("created from" branch of writeInitConfig) was
	// written.
	_, err := os.Stat(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
}

// --- applyPacks / writeScaffolds ---

func TestWriteScaffolds_WritesAndSkips(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	files := []pack.File{
		{Path: filepath.Join(".mdsmith", "wordlists", "a.yaml"), Data: []byte("entries:\n  - x\n")},
	}

	var buf bytes.Buffer
	require.NoError(t, writeScaffolds(files, &buf))
	assert.Contains(t, buf.String(), "created")
	_, err := os.Stat(files[0].Path)
	require.NoError(t, err, "file written")

	// A second call skips the now-existing file rather than erroring or
	// clobbering it.
	buf.Reset()
	require.NoError(t, writeScaffolds(files, &buf))
	assert.Contains(t, buf.String(), "already exists, skipping")
	got, err := os.ReadFile(files[0].Path)
	require.NoError(t, err)
	assert.Equal(t, "entries:\n  - x\n", string(got), "existing file not clobbered")
}

func TestWriteScaffolds_MkdirError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// .mdsmith is a real directory, but .mdsmith/wordlists is a regular
	// file. The parent chain has no symlink, so refuseSymlinkedParents
	// passes and MkdirAll(.mdsmith/wordlists) then fails because the target
	// already exists as a file — driving the directory-creation error
	// branch.
	require.NoError(t, os.Mkdir(".mdsmith", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(".mdsmith", "wordlists"), []byte("x"), 0o644))

	files := []pack.File{{Path: filepath.Join(".mdsmith", "wordlists", "a.yaml"), Data: []byte("entries:\n  - x\n")}}
	err := writeScaffolds(files, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating "+filepath.Join(".mdsmith", "wordlists"))
}

func TestWriteScaffolds_RefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	wlDir := filepath.Join(dir, ".mdsmith", "wordlists")
	require.NoError(t, os.MkdirAll(wlDir, 0o755))
	// A symlink at the target path must be refused, not followed: writing
	// through it would land the pack file wherever the link points. Even a
	// dangling link (its target does not exist) is refused rather than
	// treated as absent-and-writable.
	require.NoError(t, os.Symlink(filepath.Join(dir, "escape.yaml"), filepath.Join(wlDir, "a.yaml")))

	files := []pack.File{{Path: filepath.Join(".mdsmith", "wordlists", "a.yaml"), Data: []byte("entries:\n  - x\n")}}
	err := writeScaffolds(files, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	assert.Contains(t, err.Error(), "a.yaml")
	// The link's target was never written.
	_, statErr := os.Stat(filepath.Join(dir, "escape.yaml"))
	assert.True(t, os.IsNotExist(statErr), "must not write through the symlink")
}

func TestStatTarget(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// Missing path: absent, no error.
	exists, err := statTarget("missing.yml")
	require.NoError(t, err)
	assert.False(t, exists)

	// A regular file exists.
	require.NoError(t, os.WriteFile("real.yml", []byte("x"), 0o644))
	exists, err = statTarget("real.yml")
	require.NoError(t, err)
	assert.True(t, exists)

	// A symlink is refused, never followed.
	require.NoError(t, os.Symlink("real.yml", "link.yml"))
	_, err = statTarget("link.yml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")

	// A directory is not a regular file: reported absent (no error) so the
	// caller's write runs and fails clearly instead of silently skipping.
	require.NoError(t, os.Mkdir("adir", 0o755))
	exists, err = statTarget("adir")
	require.NoError(t, err)
	assert.False(t, exists, "a directory is not treated as an existing file")

	// A non-ENOENT lstat failure — a path component is a file, so ENOTDIR —
	// is surfaced as a checking error rather than read as absent.
	_, err = statTarget(filepath.Join("real.yml", "child"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checking")
}

func TestApplyPacks_UnknownPack(t *testing.T) {
	// applyPacks re-checks each name defensively; a bogus name returns an
	// unknown-pack error even though runInit validates first.
	err := applyPacks([]string{"bogus"}, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown pack "bogus"`)
}

func TestWriteScaffolds_WriteError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// The target path is itself a directory, so statTarget reports it
	// absent and os.WriteFile then fails with "is a directory" —
	// exercising the write-error branch.
	require.NoError(t, os.MkdirAll(filepath.Join(".mdsmith", "wordlists", "a.yaml"), 0o755))

	files := []pack.File{{Path: filepath.Join(".mdsmith", "wordlists", "a.yaml"), Data: []byte("entries:\n  - x\n")}}
	err := writeScaffolds(files, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing")
	assert.Contains(t, err.Error(), "a.yaml")
}

func TestNormalizePackNames(t *testing.T) {
	assert.Equal(t, []string{"wordlists", "stopwords"},
		normalizePackNames([]string{"wordlists", " stopwords", "  ", ""}))
	assert.Empty(t, normalizePackNames([]string{"  ", ""}))
}

func TestValidatePackPath(t *testing.T) {
	// Under .mdsmith/ is the contract; an internal ".." that stays inside
	// is harmless and allowed.
	require.NoError(t, validatePackPath(filepath.Join(".mdsmith", "wordlists", "a.yaml")))
	require.NoError(t, validatePackPath(filepath.Join(".mdsmith", "kinds", "..", "a.yaml")))

	// Absolute paths and any escape out of .mdsmith/ are refused.
	assert.Error(t, validatePackPath(filepath.FromSlash("/etc/passwd")))
	assert.Error(t, validatePackPath(filepath.Join("..", "evil.yaml")))
	assert.Error(t, validatePackPath(filepath.Join(".mdsmith", "..", "..", "evil.yaml")))
	assert.Error(t, validatePackPath(filepath.Join("other", "a.yaml")))
}

func TestWriteScaffolds_RejectsEscapingPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// A pack that returns a path escaping .mdsmith/ is refused before any
	// write, so nothing lands outside the workspace.
	files := []pack.File{{Path: filepath.Join("..", "escape.yaml"), Data: []byte("x")}}
	err := writeScaffolds(files, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "under .mdsmith/")
	_, statErr := os.Stat(filepath.Join(filepath.Dir(dir), "escape.yaml"))
	assert.True(t, os.IsNotExist(statErr), "nothing written outside the workspace")
}

func TestWriteScaffolds_RefusesSymlinkedMdsmithDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// .mdsmith itself is a symlink to another directory; pack writes must
	// not be redirected through it.
	target := filepath.Join(dir, "elsewhere")
	require.NoError(t, os.Mkdir(target, 0o755))
	require.NoError(t, os.Symlink(target, ".mdsmith"))

	files := []pack.File{{Path: filepath.Join(".mdsmith", "wordlists", "a.yaml"), Data: []byte("x")}}
	err := writeScaffolds(files, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	_, statErr := os.Stat(filepath.Join(target, "wordlists", "a.yaml"))
	assert.True(t, os.IsNotExist(statErr), "nothing written through the symlinked .mdsmith")
}

func TestWriteScaffolds_RefusesSymlinkedIntermediateDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.Mkdir(".mdsmith", 0o755))
	target := filepath.Join(dir, "elsewhere")
	require.NoError(t, os.Mkdir(target, 0o755))
	// .mdsmith is a real directory, but an intermediate component
	// (.mdsmith/wordlists) is a symlink out of tree. The write must still
	// be refused rather than following it.
	require.NoError(t, os.Symlink(target, filepath.Join(".mdsmith", "wordlists")))

	files := []pack.File{{Path: filepath.Join(".mdsmith", "wordlists", "a.yaml"), Data: []byte("x")}}
	err := writeScaffolds(files, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	_, statErr := os.Stat(filepath.Join(target, "a.yaml"))
	assert.True(t, os.IsNotExist(statErr), "nothing written through the intermediate symlink")
}

func TestRefuseSymlinkedParents(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// A path whose parent is the cwd (dir == ".") has nothing to check.
	require.NoError(t, refuseSymlinkedParents("a.yaml"))

	// Parents that don't exist yet are fine — MkdirAll makes them real.
	require.NoError(t, refuseSymlinkedParents(filepath.Join(".mdsmith", "wordlists", "a.yaml")))

	// A fully real parent chain passes.
	require.NoError(t, os.MkdirAll(filepath.Join(".mdsmith", "wordlists"), 0o755))
	require.NoError(t, refuseSymlinkedParents(filepath.Join(".mdsmith", "wordlists", "a.yaml")))

	// A symlinked component is refused.
	require.NoError(t, os.Symlink(dir, filepath.Join(".mdsmith", "link")))
	err := refuseSymlinkedParents(filepath.Join(".mdsmith", "link", "a.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")

	// A non-ENOENT lstat error — a component is a file, so ENOTDIR — is
	// surfaced as a checking error.
	require.NoError(t, os.WriteFile(filepath.Join(".mdsmith", "afile"), []byte("x"), 0o644))
	err = refuseSymlinkedParents(filepath.Join(".mdsmith", "afile", "child", "a.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checking")
}

// --- init config generation ---

func TestDefaultConfigBytes(t *testing.T) {
	data, err := defaultConfigBytes()
	require.NoError(t, err)

	s := string(data)
	assert.Contains(t, s, "rules:")
	assert.Contains(t, s, "front-matter: true")
	assert.Contains(t, s, "line-length")
}

func TestInitConfigBytes_EmptyFlagUsesDefaults(t *testing.T) {
	var buf bytes.Buffer
	data, source, err := initConfigBytes("", "", &buf)
	require.NoError(t, err)

	assert.Empty(t, source)
	assert.Contains(t, string(data), "rules:")
	assert.Empty(t, buf.String(), "defaults conversion emits no notes")
}

func TestInitConfigBytes_Starter(t *testing.T) {
	var buf bytes.Buffer
	data, source, err := initConfigBytes("", "okf", &buf)
	require.NoError(t, err)

	assert.Equal(t, "the okf starter", source)
	assert.Contains(t, string(data), "required-frontmatter")
	assert.Empty(t, buf.String(), "starter emits no notes")
}

func TestInitConfigBytes_UnknownStarter(t *testing.T) {
	var buf bytes.Buffer
	_, _, err := initConfigBytes("", "nope", &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown starter")
}

func TestInitConfigBytes_ConvertsNamedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".markdownlint.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"MD013": {"line_length": 120}, "MD024": {"siblings_only": true}}`), 0o644))

	var buf bytes.Buffer
	data, source, err := initConfigBytes(path, "", &buf)
	require.NoError(t, err)

	assert.Equal(t, path, source)
	assert.Contains(t, string(data), "max: 120")
	assert.Contains(t, buf.String(), "siblings_only",
		"untranslated options are echoed as notes")
}

func TestConvertedConfigBytes_AutoDiscovers(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".markdownlint.yaml"),
		[]byte("MD041: false\n"), 0o644))
	t.Chdir(dir)

	var buf bytes.Buffer
	data, source, err := convertedConfigBytes("auto", &buf)
	require.NoError(t, err)

	assert.Equal(t, ".markdownlint.yaml", source)
	assert.Contains(t, string(data), "first-line-heading: false")
}

func TestConvertedConfigBytes_MissingFile(t *testing.T) {
	var buf bytes.Buffer
	_, _, err := convertedConfigBytes(filepath.Join(t.TempDir(), "nope.json"), &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope.json")
}

func TestConvertedConfigBytes_ParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".markdownlintrc")
	require.NoError(t, os.WriteFile(path, []byte(`{"MD013" true}`), 0o644))

	var buf bytes.Buffer
	_, _, err := convertedConfigBytes(path, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".markdownlintrc")
}

// --- appendAPMPosture ---

func TestAppendAPMPosture_NoEmptyIgnore_Appends(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".mdsmith.yml")
	// A config with no "ignore: []" line (e.g. a starter output).
	require.NoError(t, os.WriteFile(cfg, []byte("rules: {}\n"), 0o644))

	require.NoError(t, appendAPMPosture(cfg, []string{"apm_modules/**"}))

	data, err := os.ReadFile(cfg)
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "rules: {}", "original content preserved")
	assert.Contains(t, body, "apm_modules/**", "posture appended")
}

func TestAppendAPMPosture_ReadError(t *testing.T) {
	err := appendAPMPosture(filepath.Join(t.TempDir(), "nonexistent.yml"), []string{"apm_modules/**"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "appending APM posture")
}

func TestAppendAPMPosture_OpenFileError(t *testing.T) {
	// /proc/version is readable on Linux but rejects O_WRONLY (EINVAL) even as
	// root. Its content has no "ignore: []", so appendAPMPosture reaches the
	// fallback os.OpenFile call and hits the error branch (line 223).
	if _, err := os.Stat("/proc/version"); err != nil {
		t.Skip("/proc/version not available")
	}
	err := appendAPMPosture("/proc/version", []string{"apm_modules/**"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "appending APM posture")
}

func TestAppendAPMPosture_WriteError(t *testing.T) {
	// Mount a 16k tmpfs, write a config file that exactly fills one 4k page
	// (so any append overflows into a new page), then fill the remaining three
	// pages with a filler file. appendAPMPosture's fallback OpenFile succeeds
	// but f.Write fails with ENOSPC — covering the write-error branch (line 227).
	// Skip gracefully when mounting is unavailable.
	mntDir := t.TempDir()
	out, mountErr := exec.Command("mount", "-t", "tmpfs", "-o", "size=16k", "tmpfs", mntDir).CombinedOutput()
	if mountErr != nil {
		t.Skipf("cannot mount tmpfs: %v %s", mountErr, out)
	}
	t.Cleanup(func() { exec.Command("umount", mntDir).Run() }) //nolint:errcheck

	cfg := filepath.Join(mntDir, ".mdsmith.yml")
	// Exactly 4096 bytes — fills one page, no "ignore: []" content.
	require.NoError(t, os.WriteFile(cfg, []byte(strings.Repeat("#", 4095)+"\n"), 0o644))

	// Fill remaining three 4k pages with a filler file so the tmpfs is full.
	filler := filepath.Join(mntDir, "filler")
	f, err := os.Create(filler)
	require.NoError(t, err)
	buf := make([]byte, 4096)
	for {
		if _, werr := f.Write(buf); werr != nil {
			break
		}
	}
	f.Close() //nolint:errcheck

	// The config occupies one full page; any append needs a new page which is
	// unavailable — os.File.Write returns ENOSPC.
	err = appendAPMPosture(cfg, []string{"apm_modules/**"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "appending APM posture")
}

func TestRunInit_APM_Force_Starter_AppendsPosture(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	captureStderr(func() {
		code := runInit([]string{"--apm", "--force", "--starter", "okf"})
		assert.Equal(t, 0, code)
	})

	// The okf starter has no "ignore: []" line, so appendAPMPosture falls
	// back to the os.OpenFile append path rather than the in-place replace.
	data, err := os.ReadFile(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "required-frontmatter", "okf starter content written")
	assert.Contains(t, body, "apm_modules/**", "APM posture appended via fallback path")
}
