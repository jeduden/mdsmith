// Package build implements MDS039, which validates <?build?> directive
// parameters and keeps the body in sync with the recipe's body-template.
package build

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// recipeSchema holds the param schema and body template for a recipe.
type recipeSchema struct {
	Required     []string
	Optional     []string
	BodyTemplate string
}

// defaultBodyTemplate is the fallback body-template for recipes that omit body-template.
const defaultBodyTemplate = "[{output}]({output})"

// Rule implements MDS039 (build).
//
// engineOnce serialises lazy engine init; the rule is a registered
// singleton and concurrent LSP-side callers would otherwise race on
// the engine field.
type Rule struct {
	engineOnce sync.Once
	engine     *gensection.Engine
	recipes    map[string]recipeSchema // user-declared recipes from config
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS039" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "build" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "directive" }

// RuleID implements gensection.Directive.
func (r *Rule) RuleID() string { return "MDS039" }

// RuleName implements gensection.Directive.
func (r *Rule) RuleName() string { return "build" }

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
				return fmt.Errorf("build: %w", err)
			}
			r.recipes = parsed
		default:
			return fmt.Errorf("build: unknown setting %q", k)
		}
	}
	return nil
}

func (r *Rule) getEngine() *gensection.Engine {
	r.engineOnce.Do(func() {
		r.engine = gensection.NewEngine(r)
	})
	return r.engine
}

// Check implements rule.Rule.
// It validates each <?build?> directive and reports "generated section is out of date"
// when the rendered body differs from the expected body-template output.
// Unknown params are reported as warnings, which the gensection engine
// cannot emit alongside a stale-body check, so Check is implemented
// manually.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	pairs, diags := gensection.FindMarkerPairs(f, r.Name(), r.RuleID(), r.RuleName())
	for _, mp := range pairs {
		diags = append(diags, r.checkPair(f, mp)...)
	}
	return diags
}

// Fix implements rule.FixableRule.
// The gensection engine regenerates body content for each valid directive.
func (r *Rule) Fix(f *lint.File) []byte {
	return r.getEngine().Fix(f)
}

// Validate implements gensection.Directive.
// Returns only hard (error-severity) diagnostics so Fix can regenerate
// bodies even when unknown params are present.
func (r *Rule) Validate(
	filePath string, line int,
	params map[string]string,
	_ map[string]gensection.ColumnConfig,
) []lint.Diagnostic {
	return r.validateHard(filePath, line, params)
}

// Generate implements gensection.Directive.
func (r *Rule) Generate(
	_ *lint.File, filePath string, line int,
	params map[string]string,
	_ map[string]gensection.ColumnConfig,
) (string, []lint.Diagnostic) {
	return r.generateBody(filePath, line, params)
}

// checkPair validates a single marker pair and checks stale body.
func (r *Rule) checkPair(f *lint.File, mp gensection.MarkerPair) []lint.Diagnostic {
	dir, pDiags := gensection.ParseDirective(f.Path, mp, r.RuleID(), r.RuleName())
	if dir == nil || len(pDiags) > 0 {
		return pDiags
	}

	errDiags := r.validateHard(f.Path, mp.StartLine, dir.Params)
	if len(errDiags) > 0 {
		return errDiags
	}

	var diags []lint.Diagnostic

	recipeName := dir.Params["recipe"]
	schema, _ := r.resolveRecipe(recipeName)
	diags = append(diags, r.warnUnknownParams(f.Path, mp.StartLine, recipeName, schema, dir.Params)...)

	expected, genDiags := r.generateBody(f.Path, mp.StartLine, dir.Params)
	diags = append(diags, genDiags...)
	if len(genDiags) == 0 {
		actual := gensection.ExtractContent(f, mp)
		if actual != expected {
			diags = append(diags, gensection.MakeDiag(
				r.RuleID(), r.RuleName(), f.Path, mp.StartLine,
				"generated section is out of date",
			))
		}
	}

	return diags
}

// validateHard returns error-severity diagnostics for hard failures:
// missing/invalid recipe, missing/unsafe output, missing required params.
func (r *Rule) validateHard(
	filePath string, line int,
	params map[string]string,
) []lint.Diagnostic {
	recipeName, hasRecipe := params["recipe"]
	if !hasRecipe || strings.TrimSpace(recipeName) == "" {
		return []lint.Diagnostic{gensection.MakeDiag(
			r.RuleID(), r.RuleName(), filePath, line,
			`build directive missing required "recipe" parameter`,
		)}
	}

	output, hasOutput := params["output"]
	if !hasOutput || strings.TrimSpace(output) == "" {
		return []lint.Diagnostic{gensection.MakeDiag(
			r.RuleID(), r.RuleName(), filePath, line,
			`build directive missing required "output" parameter`,
		)}
	}

	if filepath.IsAbs(output) || strings.HasPrefix(output, `\`) || hasDriveLetter(output) {
		return []lint.Diagnostic{gensection.MakeDiag(
			r.RuleID(), r.RuleName(), filePath, line,
			fmt.Sprintf(`build directive "output" must be a relative path: %q`, output),
		)}
	}

	if hasDotDotSegment(output) {
		return []lint.Diagnostic{gensection.MakeDiag(
			r.RuleID(), r.RuleName(), filePath, line,
			fmt.Sprintf(`build directive "output" contains ".." path component: %q`, output),
		)}
	}

	schema, ok := r.resolveRecipe(recipeName)
	if !ok {
		return []lint.Diagnostic{gensection.MakeDiag(
			r.RuleID(), r.RuleName(), filePath, line,
			fmt.Sprintf("build directive references unknown recipe %q", recipeName),
		)}
	}

	for _, req := range schema.Required {
		if v, ok := params[req]; !ok || strings.TrimSpace(v) == "" {
			return []lint.Diagnostic{gensection.MakeDiag(
				r.RuleID(), r.RuleName(), filePath, line,
				fmt.Sprintf("build directive recipe %q: missing required parameter %q", recipeName, req),
			)}
		}
	}

	return nil
}

// warnUnknownParams returns warning diagnostics for params not in the
// recipe's required or optional lists. Results are in sorted key order.
func (r *Rule) warnUnknownParams(
	filePath string, line int,
	recipeName string, schema recipeSchema,
	params map[string]string,
) []lint.Diagnostic {
	known := map[string]bool{"recipe": true, "output": true}
	for _, p := range schema.Required {
		known[p] = true
	}
	for _, p := range schema.Optional {
		known[p] = true
	}

	var unknown []string
	for k := range params {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)

	diags := make([]lint.Diagnostic, 0, len(unknown))
	for _, k := range unknown {
		diags = append(diags, lint.Diagnostic{
			File:     filePath,
			Line:     line,
			Column:   1,
			RuleID:   r.RuleID(),
			RuleName: r.RuleName(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("build directive recipe %q: unknown parameter %q", recipeName, k),
		})
	}
	return diags
}

// generateBody renders the recipe's body-template using the directive params.
func (r *Rule) generateBody(
	_ string, _ int,
	params map[string]string,
) (string, []lint.Diagnostic) {
	recipeName := params["recipe"]
	output := params["output"]

	schema, _ := r.resolveRecipe(recipeName)
	tmpl := schema.BodyTemplate
	if tmpl == "" {
		tmpl = defaultBodyTemplate
	}

	alt := fmt.Sprintf("%s output: %s", recipeName, output)
	body := strings.NewReplacer("{alt}", alt, "{output}", output).Replace(tmpl)

	return gensection.EnsureTrailingNewline(body), nil
}

// resolveRecipe looks up a recipe by name in the user-declared recipes.
func (r *Rule) resolveRecipe(name string) (recipeSchema, bool) {
	if r.recipes != nil {
		if s, ok := r.recipes[name]; ok {
			return s, true
		}
	}
	return recipeSchema{}, false
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

// hasDriveLetter reports whether p begins with a Windows drive letter (e.g. C: or C:\).
// This check is platform-independent, unlike filepath.VolumeName which returns ""
// on non-Windows hosts even for Windows-style paths.
func hasDriveLetter(p string) bool {
	return len(p) >= 2 && p[1] == ':' &&
		((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z'))
}

// parseRecipesSettings deserialises a recipes map from map[string]any.
func parseRecipesSettings(v any) (map[string]recipeSchema, error) {
	rawMap, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("recipes must be a map, got %T", v)
	}
	out := make(map[string]recipeSchema, len(rawMap))
	for name, rawRecipe := range rawMap {
		rm, ok := rawRecipe.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("recipe %q must be a map, got %T", name, rawRecipe)
		}
		schema := recipeSchema{}
		if bt, ok := rm["body-template"]; ok {
			s, ok := bt.(string)
			if !ok {
				return nil, fmt.Errorf("recipe %q: body-template must be a string, got %T", name, bt)
			}
			schema.BodyTemplate = s
		}
		if rawParams, hasParams := rm["params"]; hasParams {
			paramsMap, ok := rawParams.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("recipe %q: params must be a map, got %T", name, rawParams)
			}
			req, err := toStringSlice(paramsMap["required"])
			if err != nil {
				return nil, fmt.Errorf("recipe %q: params.required: %w", name, err)
			}
			opt, err := toStringSlice(paramsMap["optional"])
			if err != nil {
				return nil, fmt.Errorf("recipe %q: params.optional: %w", name, err)
			}
			schema.Required = req
			schema.Optional = opt
		}
		out[name] = schema
	}
	return out, nil
}

// toStringSlice converts []any or []string to []string.
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

var _ rule.FixableRule = (*Rule)(nil)
var _ gensection.Directive = (*Rule)(nil)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Regenerate build section" }
