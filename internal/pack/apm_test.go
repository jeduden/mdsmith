package pack_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/pack"
)

func TestAPMPack_Registered(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok, "apm pack is registered")
	assert.Equal(t, "apm", p.Name)
	assert.NotEmpty(t, p.Summary)
}

func TestAPMPack_InNamesAndAll(t *testing.T) {
	assert.Contains(t, pack.Names(), "apm")
	names := pack.Names()
	found := false
	for _, p := range pack.All() {
		if p.Name == "apm" {
			found = true
			assert.Equal(t, names[indexOfName(names, "apm")], p.Name)
			break
		}
	}
	assert.True(t, found, "apm must appear in pack.All()")
}

func TestAPMPack_Files_HasFourKindFiles(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	require.Len(t, files, 4, "apm pack must return exactly four kind files")
}

func TestAPMPack_Files_Paths(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()

	byPath := map[string][]byte{}
	for _, f := range files {
		byPath[f.Path] = f.Data
	}

	require.Contains(t, byPath, ".mdsmith/kinds/apm-skill.yaml")
	require.Contains(t, byPath, ".mdsmith/kinds/apm-prompt.yaml")
	require.Contains(t, byPath, ".mdsmith/kinds/apm-instruction.yaml")
	require.Contains(t, byPath, ".mdsmith/kinds/apm-agent.yaml")
}

func TestAPMPack_SkillKind_PathPattern(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	data := findFile(files, ".mdsmith/kinds/apm-skill.yaml")
	require.NotNil(t, data, "apm-skill.yaml must be in the pack")
	body := string(data)
	assert.Contains(t, body, `path-pattern: ".apm/skills/*/SKILL.md"`,
		"apm-skill.yaml must declare the correct path-pattern")
}

func TestAPMPack_SkillKind_RequiredFrontmatter(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	data := findFile(files, ".mdsmith/kinds/apm-skill.yaml")
	require.NotNil(t, data)
	body := string(data)
	assert.Contains(t, body, "name: nonEmpty",
		"apm-skill.yaml must require name")
	assert.Contains(t, body, "description: nonEmpty",
		"apm-skill.yaml must require description")
}

func TestAPMPack_SkillKind_SizeLimits(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	data := findFile(files, ".mdsmith/kinds/apm-skill.yaml")
	require.NotNil(t, data)
	body := string(data)
	// APM docs cap SKILL.md at 500 lines / 5000 tokens.
	assert.Contains(t, body, "max: 500", "SKILL.md line cap is 500")
	assert.Contains(t, body, "max: 5000", "SKILL.md token cap is 5000")
}

func TestAPMPack_PromptKind_PathPattern(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	data := findFile(files, ".mdsmith/kinds/apm-prompt.yaml")
	require.NotNil(t, data)
	body := string(data)
	assert.Contains(t, body, `path-pattern: ".apm/prompts/*.prompt.md"`)
}

func TestAPMPack_PromptKind_HasAPMInputToken(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	data := findFile(files, ".mdsmith/kinds/apm-prompt.yaml")
	require.NotNil(t, data)
	body := string(data)
	assert.Contains(t, body, "apm-input-token",
		"apm-prompt.yaml must opt rules into the apm-input-token placeholder")
}

func TestAPMPack_PromptKind_RequiredFrontmatter(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	data := findFile(files, ".mdsmith/kinds/apm-prompt.yaml")
	require.NotNil(t, data)
	body := string(data)
	assert.Contains(t, body, "description: nonEmpty",
		"apm-prompt.yaml must require description")
}

func TestAPMPack_InstructionKind_PathPattern(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	data := findFile(files, ".mdsmith/kinds/apm-instruction.yaml")
	require.NotNil(t, data)
	body := string(data)
	assert.Contains(t, body, `path-pattern: ".apm/instructions/*.instructions.md"`)
}

func TestAPMPack_InstructionKind_RequiredFrontmatter(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	data := findFile(files, ".mdsmith/kinds/apm-instruction.yaml")
	require.NotNil(t, data)
	body := string(data)
	assert.Contains(t, body, "description: nonEmpty",
		"apm-instruction.yaml must require description")
}

func TestAPMPack_AgentKind_PathPattern(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	data := findFile(files, ".mdsmith/kinds/apm-agent.yaml")
	require.NotNil(t, data)
	body := string(data)
	assert.Contains(t, body, `path-pattern: ".apm/agents/*.agent.md"`)
}

func TestAPMPack_AgentKind_RequiredFrontmatter(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	data := findFile(files, ".mdsmith/kinds/apm-agent.yaml")
	require.NotNil(t, data)
	body := string(data)
	assert.Contains(t, body, "name: nonEmpty",
		"apm-agent.yaml must require name")
	assert.Contains(t, body, "description: nonEmpty",
		"apm-agent.yaml must require description")
}

func TestAPMPack_AgentKind_MaxFileLength300(t *testing.T) {
	p, ok := pack.Get("apm")
	require.True(t, ok)
	files := p.Files()
	data := findFile(files, ".mdsmith/kinds/apm-agent.yaml")
	require.NotNil(t, data)
	body := string(data)
	assert.Contains(t, body, "max: 300",
		"apm-agent.yaml must cap agents at 300 lines")
}

// findFile returns the Data bytes for the file at path, or nil if absent.
func findFile(files []pack.File, path string) []byte {
	for _, f := range files {
		if f.Path == path {
			return f.Data
		}
	}
	return nil
}

// indexOfName returns the index of name in names, or -1.
func indexOfName(names []string, name string) int {
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
}
