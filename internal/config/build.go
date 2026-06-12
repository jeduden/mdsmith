package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// BuildConfig is the top-level build: section.
type BuildConfig struct {
	Recipes map[string]RecipeCfg `yaml:"recipes,omitempty"`
	Hooks   HooksCfg             `yaml:"hooks,omitempty"`
	// Exec configures the hermetic execution environment recipes run
	// under (plan 2606101548). Both keys are optional; empty values mean
	// the build executor's compiled defaults apply.
	Exec ExecCfg `yaml:"exec,omitempty"`
}

// HooksCfg holds the before/after hook lists for the build pass.
type HooksCfg struct {
	Before []HookCfg `yaml:"before,omitempty"`
	After  []HookCfg `yaml:"after,omitempty"`
}

// HookCfg is a single hook entry in build.hooks.before or build.hooks.after.
type HookCfg struct {
	Command string            `yaml:"command"`
	Params  map[string]string `yaml:"params,omitempty"`
	Name    string            `yaml:"name,omitempty"`
}

// ExecCfg is the build.exec: section: the allowlisted PATH and the
// environment-variable names passed through to every recipe. An empty
// Path or EnvPassThrough means the executor's compiled defaults apply
// (PATH = /usr/bin:/bin on Unix; pass-through = [HOME, LANG, LC_ALL]).
// EnvPassThrough *replaces* the default list rather than appending to it.
type ExecCfg struct {
	Path           string   `yaml:"path,omitempty"`
	EnvPassThrough []string `yaml:"env-pass-through,omitempty"`
}

// RecipeCfg is a single user-defined recipe declaration.
type RecipeCfg struct {
	Command      string   `yaml:"command"`
	BodyTemplate string   `yaml:"body-template,omitempty"`
	Params       ParamCfg `yaml:"params,omitempty"`
	// DefaultInputs declares implicit inputs folded into every directive's
	// input set. Each entry is either a {param} token (a declared param
	// name) or a literal relative path passing the path-shape rules. A
	// param token expands to the root-joined absolute path at exec time;
	// the value hashed into the ActionID is always the relative path the
	// param supplies.
	DefaultInputs []string `yaml:"default-inputs,omitempty"`
}

// ParamCfg names the params a recipe accepts.
type ParamCfg struct {
	Required []string `yaml:"required,omitempty"`
	Optional []string `yaml:"optional,omitempty"`
}

// reservedParams are variables available in body-template but forbidden in command
// and in params declarations. {alt} is reserved because it maps to the Markdown
// image alt-text field, which the framework injects from the surrounding Markdown
// syntax; it is not a user-supplied build parameter. {output} is intentionally
// NOT reserved — a recipe command may write to an output path declared as a
// regular parameter.
var reservedParams = map[string]bool{"alt": true}

// placeholderRe matches a {name} placeholder where name is an identifier
// ([A-Za-z_][A-Za-z0-9_]*). Tokens like {a b} are intentionally not matched
// because commands are whitespace-tokenised and such a token would be split.
var placeholderRe = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// maxHookParamBytes is the maximum allowed length for a hook param value.
const maxHookParamBytes = 4 * 1024

// ValidateBuildConfig returns an error if any recipe declares a reserved param
// name or if its command references an unknown or reserved placeholder.
// Recipe names are validated in sorted order for deterministic errors.
// Hooks (before/after) are also validated.
func ValidateBuildConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Build.Recipes))
	for name := range cfg.Build.Recipes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := validateRecipe(name, cfg.Build.Recipes[name]); err != nil {
			return err
		}
	}
	for i, hook := range cfg.Build.Hooks.Before {
		if err := validateHook("before", i, hook); err != nil {
			return err
		}
	}
	for i, hook := range cfg.Build.Hooks.After {
		if err := validateHook("after", i, hook); err != nil {
			return err
		}
	}
	return validateExecConfig(cfg.Build.Exec)
}

// validateHook validates a single hook entry. listName is "before" or "after".
func validateHook(listName string, idx int, hook HookCfg) error {
	label := fmt.Sprintf("build.hooks.%s[%d]", listName, idx)
	if hook.Command == "" {
		return fmt.Errorf("%s: command must not be empty", label)
	}
	// Validate param values in sorted key order for deterministic errors.
	paramKeys := make([]string, 0, len(hook.Params))
	for k := range hook.Params {
		paramKeys = append(paramKeys, k)
	}
	sort.Strings(paramKeys)
	allowed := make(map[string]bool, len(hook.Params))
	for _, k := range paramKeys {
		allowed[k] = true
		if err := validateHookParamValue(label, k, hook.Params[k]); err != nil {
			return err
		}
	}
	// Validate command placeholders: hooks may not reference {inputs} or
	// {outputs} — those are directive-context collective placeholders.
	return validateHookCommandPlaceholders(label, hook.Command, allowed)
}

// validateHookParamValue enforces the baseline constraints on a hook param value:
// no NUL byte, no newline or carriage return, no leading/trailing whitespace,
// and at most maxHookParamBytes bytes.
func validateHookParamValue(label, paramName, value string) error {
	if len(value) > maxHookParamBytes {
		return fmt.Errorf("%s: param %q value exceeds 4 KB limit", label, paramName)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s: param %q value must not contain a NUL byte", label, paramName)
	}
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("%s: param %q value must not contain a newline or carriage return", label, paramName)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s: param %q value must not have leading or trailing whitespace", label, paramName)
	}
	return nil
}

// validateHookCommandPlaceholders validates placeholders in a hook command.
// For hooks, {inputs} and {outputs} are forbidden (they are meaningless without
// a directive context), and any {param} token must appear in the allowed map.
func validateHookCommandPlaceholders(label, command string, allowed map[string]bool) error {
	for _, tok := range strings.Fields(command) {
		for _, m := range placeholderRe.FindAllStringSubmatch(tok, -1) {
			param := m[1]
			if collectivePlaceholders[param] {
				return fmt.Errorf(
					"%s: command references {%s} which is not available in hooks "+
						"(hooks have no directive context)",
					label, param,
				)
			}
			if reservedParams[param] {
				return fmt.Errorf(
					"%s: command uses reserved placeholder {%s}; "+
						"reserved placeholders are only available in body-template",
					label, param,
				)
			}
			if !allowed[param] {
				return fmt.Errorf(
					"%s: command references undeclared placeholder {%s}; "+
						"declare it in params",
					label, param,
				)
			}
		}
	}
	return nil
}

// validateExecConfig rejects an env-pass-through entry that is empty or
// contains an "=" character. An empty name cannot identify an
// environment variable; an "=" would let a config inject a value rather
// than name a variable to pass through, defeating the allowlist.
func validateExecConfig(exec ExecCfg) error {
	for i, name := range exec.EnvPassThrough {
		if name == "" {
			return fmt.Errorf("build.exec.env-pass-through[%d]: name must not be empty", i)
		}
		if strings.Contains(name, "=") {
			return fmt.Errorf(
				"build.exec.env-pass-through[%d]: name %q must not contain %q", i, name, "=",
			)
		}
		if strings.ContainsAny(name, "\x00\n\r") {
			return fmt.Errorf(
				"build.exec.env-pass-through[%d]: name %q must not contain NUL, newline, or carriage return",
				i, name,
			)
		}
	}
	return nil
}

func validateRecipe(name string, recipe RecipeCfg) error {
	if err := validateDeclaredParams(name, recipe.Params); err != nil {
		return err
	}
	if recipe.Command == "" {
		return nil
	}
	allowed := make(map[string]bool)
	for _, p := range recipe.Params.Required {
		allowed[p] = true
	}
	for _, p := range recipe.Params.Optional {
		allowed[p] = true
	}
	if err := validateCommandPlaceholders(name, recipe.Command, allowed); err != nil {
		return err
	}
	return validateDefaultInputs(name, recipe.DefaultInputs, allowed)
}

// defaultInputTokenRe matches an entry that is exactly a single {name}
// param token. A literal path entry never matches.
var defaultInputTokenRe = regexp.MustCompile(`^\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

// validateDefaultInputs checks each default-inputs entry. A {param} token
// must name a declared, non-reserved param; any other entry must be a
// literal relative path passing the path-shape rules.
func validateDefaultInputs(recipeName string, entries []string, allowed map[string]bool) error {
	for _, entry := range entries {
		if m := defaultInputTokenRe.FindStringSubmatch(entry); m != nil {
			param := m[1]
			if reservedParams[param] || collectivePlaceholders[param] {
				return fmt.Errorf(
					"build.recipes.%s: default-inputs uses reserved token {%s}",
					recipeName, param,
				)
			}
			if !allowed[param] {
				return fmt.Errorf(
					"build.recipes.%s: default-inputs references undeclared param {%s}",
					recipeName, param,
				)
			}
			continue
		}
		if reason := defaultInputPathShape(entry); reason != "" {
			return fmt.Errorf(
				"build.recipes.%s: default-inputs entry %q %s",
				recipeName, entry, reason,
			)
		}
	}
	return nil
}

// defaultInputPathShape returns "" when entry is an acceptable literal
// relative path, or a short reason phrase otherwise. The rules mirror the
// directive path-shape allowlist: no absolute paths, no NUL/newline, no
// backslashes, no ".." escape.
func defaultInputPathShape(entry string) string {
	if strings.TrimSpace(entry) == "" {
		return "must not be empty"
	}
	if entry != strings.TrimSpace(entry) {
		return "must not have leading or trailing whitespace"
	}
	if strings.ContainsAny(entry, "\x00\n\r") {
		return "must not contain NUL, newline, or carriage return"
	}
	if strings.Contains(entry, `\`) {
		return "must use forward-slash separators only"
	}
	if strings.HasPrefix(entry, "/") || strings.HasPrefix(entry, "~") {
		return "must be a relative path"
	}
	cleaned := strings.TrimRight(entry, "/")
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") ||
		strings.Contains(cleaned, "/../") || strings.HasSuffix(cleaned, "/..") {
		return `must not contain ".." path components`
	}
	return ""
}

func validateDeclaredParams(recipeName string, params ParamCfg) error {
	for _, p := range params.Required {
		if reservedParams[p] {
			return fmt.Errorf(
				"build.recipes.%s: params.required contains reserved name %q; "+
					"reserved names are only available in body-template",
				recipeName, p,
			)
		}
	}
	for _, p := range params.Optional {
		if reservedParams[p] {
			return fmt.Errorf(
				"build.recipes.%s: params.optional contains reserved name %q; "+
					"reserved names are only available in body-template",
				recipeName, p,
			)
		}
	}
	return nil
}

// collectivePlaceholders are the argv list placeholders the build
// executor expands from the directive's outputs:/inputs: lists. They
// may appear in a command without being declared as params, but each
// must stand alone as its own whitespace-delimited token: expanding a
// list inside a token fragment (e.g. -o{outputs}) has no well-defined
// meaning.
var collectivePlaceholders = map[string]bool{"inputs": true, "outputs": true}

func validateCommandPlaceholders(recipeName, command string, allowed map[string]bool) error {
	for _, tok := range strings.Fields(command) {
		isStandalone := tok == "{inputs}" || tok == "{outputs}"
		for _, m := range placeholderRe.FindAllStringSubmatch(tok, -1) {
			param := m[1]
			if collectivePlaceholders[param] {
				if isStandalone {
					continue
				}
				return fmt.Errorf(
					"build.recipes.%s: command embeds collective placeholder {%s} in token %q; "+
						"it must stand alone as its own argument",
					recipeName, param, tok,
				)
			}
			if reservedParams[param] {
				return fmt.Errorf(
					"build.recipes.%s: command uses reserved placeholder {%s}; "+
						"reserved placeholders are only available in body-template",
					recipeName, param,
				)
			}
			if !allowed[param] {
				return fmt.Errorf(
					"build.recipes.%s: command references undeclared placeholder {%s}; "+
						"declare it in params.required or params.optional",
					recipeName, param,
				)
			}
		}
	}
	return nil
}

// InjectBuildConfig copies cfg.Build.Recipes and cfg.Build.Hooks into the
// recipe-safety and build rule settings. It is called after config loading in
// main so rules receive their inputs through the normal ApplySettings path.
// cfgPath is the path to the loaded .mdsmith.yml; it is set in the config-path
// setting so MDS040 can report diagnostics against the right file.
func InjectBuildConfig(cfg *Config, cfgPath string) {
	if cfg == nil {
		return
	}
	recipes := serializeRecipes(cfg.Build.Recipes)

	// Inject into recipe-safety (MDS040) with config-path and hooks.
	if rc, ok := cfg.Rules["recipe-safety"]; ok && rc.Enabled {
		if rc.Settings == nil {
			rc.Settings = make(map[string]any)
		}
		rc.Settings["recipes"] = recipes
		rc.Settings["hooks-before"] = serializeHooks(cfg.Build.Hooks.Before)
		rc.Settings["hooks-after"] = serializeHooks(cfg.Build.Hooks.After)
		if cfgPath != "" {
			rc.Settings["config-path"] = cfgPath
		}
		cfg.Rules["recipe-safety"] = rc
	}

	// Inject into build directive (MDS039).
	if rc, ok := cfg.Rules["build"]; ok && rc.Enabled {
		if rc.Settings == nil {
			rc.Settings = make(map[string]any)
		}
		rc.Settings["recipes"] = recipes
		cfg.Rules["build"] = rc
	}
}

// serializeHooks converts a []HookCfg to []any for transport through the
// generic ApplySettings mechanism.
func serializeHooks(hooks []HookCfg) []any {
	out := make([]any, len(hooks))
	for i, h := range hooks {
		m := map[string]any{"command": h.Command}
		if h.Name != "" {
			m["name"] = h.Name
		}
		if len(h.Params) > 0 {
			params := make(map[string]any, len(h.Params))
			for k, v := range h.Params {
				params[k] = v
			}
			m["params"] = params
		}
		out[i] = m
	}
	return out
}

// serializeRecipes converts RecipeCfg map to map[string]any for transport
// through the generic ApplySettings mechanism.
func serializeRecipes(recipes map[string]RecipeCfg) map[string]any {
	out := make(map[string]any, len(recipes))
	for name, r := range recipes {
		m := map[string]any{"command": r.Command}
		if r.BodyTemplate != "" {
			m["body-template"] = r.BodyTemplate
		}
		params := map[string]any{}
		if len(r.Params.Required) > 0 {
			s := make([]any, len(r.Params.Required))
			for i, v := range r.Params.Required {
				s[i] = v
			}
			params["required"] = s
		}
		if len(r.Params.Optional) > 0 {
			s := make([]any, len(r.Params.Optional))
			for i, v := range r.Params.Optional {
				s[i] = v
			}
			params["optional"] = s
		}
		if len(params) > 0 {
			m["params"] = params
		}
		if len(r.DefaultInputs) > 0 {
			s := make([]any, len(r.DefaultInputs))
			for i, v := range r.DefaultInputs {
				s[i] = v
			}
			m["default-inputs"] = s
		}
		out[name] = m
	}
	return out
}
