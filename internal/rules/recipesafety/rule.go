// Package recipesafety implements MDS040, which validates every command
// in build.recipes for shell-safety at lint time.
package recipesafety

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// shellInterpreters is the set of first-token values that indicate a shell
// interpreter.
var shellInterpreters = map[string]bool{
	"sh":            true,
	"bash":          true,
	"zsh":           true,
	"ksh":           true,
	"fish":          true,
	"/bin/sh":       true,
	"/bin/bash":     true,
	"/bin/zsh":      true,
	"/bin/ksh":      true,
	"/bin/fish":     true,
	"/usr/bin/sh":   true,
	"/usr/bin/bash": true,
}

// shellOperators are substrings that indicate shell pipeline/redirection when
// found in a static token (one that is not entirely a single placeholder).
var shellOperators = []string{
	"&&", "||", ";", "|", ">>", "2>", ">", "<", "`", "$(", "${",
}

// placeholderRe matches an entire {name} placeholder.
var placeholderRe = regexp.MustCompile(`\{([^{}]+)\}`)

// fusedRe matches two or more adjacent placeholders in one token,
// e.g. {a}{b} or {a}{b}{c}.
var fusedRe = regexp.MustCompile(`\{[^{}]+\}\{[^{}]+\}`)

// recipe holds the parsed fields needed for validation.
type recipe struct {
	Command  string
	Required []string
	Optional []string
}

// Rule implements MDS040 (recipe-safety).
type Rule struct {
	Recipes    map[string]recipe
	ConfigPath string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS040" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "recipe-safety" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// IsConfigFileRule implements rule.ConfigTarget.
func (r *Rule) IsConfigFileRule() bool { return true }

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "recipes":
			parsed, err := parseRecipesSettings(v)
			if err != nil {
				return fmt.Errorf("recipe-safety: %w", err)
			}
			r.Recipes = parsed
		case "config-path":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("recipe-safety: config-path must be a string, got %T", v)
			}
			r.ConfigPath = s
		default:
			return fmt.Errorf("recipe-safety: unknown setting %q", k)
		}
	}
	return nil
}

// parseRecipesSettings deserialises the recipes map from map[string]any.
// It handles both the serialised form from InjectBuildConfig and the
// YAML-decoded form from fixture settings:.
func parseRecipesSettings(v any) (map[string]recipe, error) {
	rawMap, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("recipes must be a map, got %T", v)
	}
	out := make(map[string]recipe, len(rawMap))
	for name, rawRecipe := range rawMap {
		rm, ok := rawRecipe.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("recipe %q must be a map, got %T", name, rawRecipe)
		}
		cmd, _ := rm["command"].(string)
		rec := recipe{Command: cmd}
		if params, ok := rm["params"].(map[string]any); ok {
			rec.Required = toStringSlice(params["required"])
			rec.Optional = toStringSlice(params["optional"])
		}
		out[name] = rec
	}
	return out, nil
}

// toStringSlice converts []any or []string to []string.
func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// Check implements rule.Rule.
//
// In production mode (ConfigPath != ""), Check only runs when f.Path
// matches ConfigPath — the runner calls it once against a synthetic
// lint.File for the config file. In fixture/test mode (ConfigPath == ""),
// it always validates the configured recipes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if len(r.Recipes) == 0 {
		return nil
	}
	if r.ConfigPath != "" && f.Path != r.ConfigPath {
		return nil
	}
	return r.validateRecipes(f.Path)
}

// validateRecipes runs all six checks on every recipe and returns diagnostics.
// Results are sorted by recipe name then message for deterministic output.
func (r *Rule) validateRecipes(filePath string) []lint.Diagnostic {
	names := make([]string, 0, len(r.Recipes))
	for name := range r.Recipes {
		names = append(names, name)
	}
	sort.Strings(names)

	var diags []lint.Diagnostic
	for _, name := range names {
		diags = append(diags, r.checkRecipe(filePath, name, r.Recipes[name])...)
	}
	return diags
}

func (r *Rule) checkRecipe(filePath, name string, rec recipe) []lint.Diagnostic {
	if rec.Command == "" {
		return []lint.Diagnostic{r.diag(filePath, lint.Error,
			fmt.Sprintf("recipe %q: command must not be empty", name))}
	}
	tokens := strings.Fields(rec.Command)
	diags := r.checkExecutable(filePath, name, tokens[0])
	diags = append(diags, r.checkTokens(filePath, name, tokens)...)
	diags = append(diags, r.checkUnusedParams(filePath, name, rec)...)
	return diags
}

func (r *Rule) checkExecutable(filePath, name, exe string) []lint.Diagnostic {
	var diags []lint.Diagnostic
	if shellInterpreters[exe] {
		diags = append(diags, r.diag(filePath, lint.Error,
			fmt.Sprintf("recipe %q: command uses shell interpreter %q — use the direct binary",
				name, exe)))
	}
	if strings.Contains(exe, "..") {
		diags = append(diags, r.diag(filePath, lint.Error,
			fmt.Sprintf("recipe %q: executable %q contains a .. path component",
				name, exe)))
	}
	return diags
}

func (r *Rule) checkTokens(filePath, name string, tokens []string) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for _, tok := range tokens {
		isSinglePlaceholder := placeholderRe.MatchString(tok) &&
			placeholderRe.FindString(tok) == tok
		if !isSinglePlaceholder {
			for _, op := range shellOperators {
				if strings.Contains(tok, op) {
					diags = append(diags, r.diag(filePath, lint.Error,
						fmt.Sprintf("recipe %q: command contains shell operator %q — use a wrapper script",
							name, op)))
					break
				}
			}
		}
		if fusedRe.MatchString(tok) {
			fused := fusedRe.FindString(tok)
			diags = append(diags, r.diag(filePath, lint.Error,
				fmt.Sprintf("recipe %q: command contains fused placeholders %q — separate with a delimiter",
					name, fused)))
		}
	}
	return diags
}

func (r *Rule) checkUnusedParams(filePath, name string, rec recipe) []lint.Diagnostic {
	declared := make(map[string]bool)
	for _, p := range rec.Required {
		declared[p] = true
	}
	for _, p := range rec.Optional {
		declared[p] = true
	}
	used := make(map[string]bool)
	for _, m := range placeholderRe.FindAllStringSubmatch(rec.Command, -1) {
		used[m[1]] = true
	}
	var unused []string
	for p := range declared {
		if !used[p] {
			unused = append(unused, p)
		}
	}
	sort.Strings(unused)
	diags := make([]lint.Diagnostic, 0, len(unused))
	for _, p := range unused {
		diags = append(diags, r.diag(filePath, lint.Warning,
			fmt.Sprintf("recipe %q: declared param %q is not referenced in command",
				name, p)))
	}
	return diags
}

func (r *Rule) diag(filePath string, severity lint.Severity, message string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     filePath,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: severity,
		Message:  message,
	}
}
