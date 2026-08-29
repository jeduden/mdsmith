package pack

import "path/filepath"

func init() {
	register(Pack{
		Name:    "apm",
		Summary: "APM kind pack for .apm/ source files (skills, prompts, instructions, agents)",
		files:   apmKindFiles,
	})
}

// apmKindFiles returns the four kind files for the APM kind pack.
// Each file validates one APM primitive type's frontmatter contracts and
// enforces the size budgets the APM docs specify.
func apmKindFiles() []File {
	return []File{
		{Path: filepath.Join(".mdsmith", "kinds", "apm-skill.yaml"), Data: apmSkillKind},
		{Path: filepath.Join(".mdsmith", "kinds", "apm-prompt.yaml"), Data: apmPromptKind},
		{Path: filepath.Join(".mdsmith", "kinds", "apm-instruction.yaml"), Data: apmInstructionKind},
		{Path: filepath.Join(".mdsmith", "kinds", "apm-agent.yaml"), Data: apmAgentKind},
	}
}

// apmSkillKind validates .apm/skills/<name>/SKILL.md against the
// agentskills.io format. name and description are required; the APM
// docs cap SKILL.md at 500 lines / 5000 tokens.
var apmSkillKind = []byte(`# .mdsmith/kinds/apm-skill.yaml
#
# APM skill kind — scaffolded by ` + "`mdsmith init --apm`" + `.
# Validates .apm/skills/<name>/SKILL.md against the agentskills.io
# format: name (lowercase alphanumeric + hyphens, 1–64 chars) and
# description (imperative phrase, under 1024 chars) are required.
# The APM docs cap SKILL.md at 500 lines / 5000 tokens.
path-pattern: ".apm/skills/*/SKILL.md"
schema:
  frontmatter:
    name: nonEmpty
    description: nonEmpty
rules:
  max-file-length:
    max: 500
  token-budget:
    max: 5000
    mode: heuristic
    tokens-per-word: 1.33
    tokenizer: builtin
`)

// apmPromptKind validates .apm/prompts/<name>.prompt.md files.
// The APM compiler keeps exactly five frontmatter keys; description is
// required. Prompt bodies reference ${input:name} parameters, so the
// apm-input-token placeholder is opted in so content rules treat them
// as opaque rather than flagging them as prose violations.
var apmPromptKind = []byte(`# .mdsmith/kinds/apm-prompt.yaml
#
# APM prompt kind — scaffolded by ` + "`mdsmith init --apm`" + `.
# Validates .apm/prompts/<name>.prompt.md files. The APM compiler
# keeps exactly five frontmatter keys; description is required and
# the rest are optional. Prompt bodies use ${input:NAME} parameters;
# the apm-input-token placeholder prevents false positives on those
# tokens from content rules.
path-pattern: ".apm/prompts/*.prompt.md"
schema:
  frontmatter:
    description: nonEmpty
    "input?": string
    "allowed-tools?": string
    "model?": string
    "argument-hint?": string
rules:
  paragraph-readability:
    placeholders: [apm-input-token]
  paragraph-structure:
    placeholders: [apm-input-token]
  heading-increment:
    placeholders: [apm-input-token]
  first-line-heading:
    placeholders: [apm-input-token]
  no-emphasis-as-heading:
    placeholders: [apm-input-token]
  cross-file-reference-integrity:
    placeholders: [apm-input-token]
`)

// apmInstructionKind validates .apm/instructions/<name>.instructions.md
// files. description and applyTo are the contractual frontmatter keys;
// an instruction without applyTo compiles into the root context file
// rather than a scoped per-file rule.
var apmInstructionKind = []byte(`# .mdsmith/kinds/apm-instruction.yaml
#
# APM instruction kind — scaffolded by ` + "`mdsmith init --apm`" + `.
# Validates .apm/instructions/<name>.instructions.md files.
# description is required; applyTo (a glob or comma-separated globs
# binding the instruction to files) is optional but without it the
# instruction compiles into the root context file instead of a scoped
# rule.
path-pattern: ".apm/instructions/*.instructions.md"
schema:
  frontmatter:
    description: nonEmpty
    "applyTo?": string
`)

// apmAgentKind validates .apm/agents/<name>.agent.md files.
// name and description are required; the APM docs cap agents at 300
// lines.
var apmAgentKind = []byte(`# .mdsmith/kinds/apm-agent.yaml
#
# APM agent kind — scaffolded by ` + "`mdsmith init --apm`" + `.
# Validates .apm/agents/<name>.agent.md files. name and description
# are required frontmatter. The APM docs keep agents under 300 lines;
# the max-file-length rule enforces that cap.
path-pattern: ".apm/agents/*.agent.md"
schema:
  frontmatter:
    name: nonEmpty
    description: nonEmpty
    "model?": string
    "color?": string
rules:
  max-file-length:
    max: 300
`)
