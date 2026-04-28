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
	"/usr/bin/zsh":  true,
	"/usr/bin/ksh":  true,
	"/usr/bin/fish": true,
}

// shellOperators are substrings that indicate shell pipeline/redirection when
// found in a static token (one that is not entirely a single placeholder).
var shellOperators = []string{
	"&&", "||", ";", "|", ">>", "2>", ">", "<", "`", "$(", "${",
}

// placeholderRe matches a {name} placeholder substring.
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
// YAML-decoded form from fixture settings.
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
		rawCommand, ok := rm["command"]
		if !ok {
			return nil, fmt.Errorf("recipe %q: missing required 'command' field", name)
		}
		cmd, ok := rawCommand.(string)
		if !ok {
			return nil, fmt.Errorf("recipe %q: command must be a string, got %T", name, rawCommand)
		}
		rec := recipe{Command: cmd}
		if rawParams, hasParams := rm["params"]; hasParams {
			params, ok := rawParams.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("recipe %q: params must be a map, got %T", name, rawParams)
			}
			req, err := toStringSlice(params["required"])
			if err != nil {
				return nil, fmt.Errorf("recipe %q: params.required: %w", name, err)
			}
			opt, err := toStringSlice(params["optional"])
			if err != nil {
				return nil, fmt.Errorf("recipe %q: params.optional: %w", name, err)
			}
			rec.Required = req
			rec.Optional = opt
		}
		out[name] = rec
	}
	return out, nil
}

// toStringSlice converts []any or []string to []string.
// It returns an error if any element in a []any slice is not a string,
// or if the input is not a string slice type.
func toStringSlice(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	switch s := v.(type) {
	case []string:
		return s, nil
	case []any:
		out := make([]string, 0, len(s))
		for i, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("element %d must be a string, got %T", i, item)
			}
			out = append(out, str)
		}
		return out, nil
	}
	return nil, fmt.Errorf("must be a string slice, got %T", v)
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
// Results are in recipe-name order; within a recipe the check order is fixed.
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
	tokens := strings.Fields(rec.Command)
	if len(tokens) == 0 {
		return []lint.Diagnostic{r.diag(filePath, lint.Error,
			fmt.Sprintf("recipe %q: command must not be empty", name))}
	}
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
	if hasDotDotSegment(exe) {
		diags = append(diags, r.diag(filePath, lint.Error,
			fmt.Sprintf("recipe %q: executable %q contains a .. path component",
				name, exe)))
	}
	return diags
}

// hasDotDotSegment reports whether the path has a segment that is exactly "..".
func hasDotDotSegment(p string) bool {
	for _, seg := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return true
		}
	}
	return false
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
