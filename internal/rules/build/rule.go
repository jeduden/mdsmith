// Package build implements MDS039, which validates <?build?> directive
// parameters and keeps the body in sync with the recipe's body-template.
package build

import (
	"fmt"
	"path"
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
// missing/invalid recipe, missing/empty/unsafe outputs, invalid inputs,
// missing required params. The first failure wins.
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

	if diags := r.validateOutputs(filePath, line, params); diags != nil {
		return diags
	}
	if diags := r.validateInputs(filePath, line, params); diags != nil {
		return diags
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

// validateOutputs checks the required, non-empty "outputs" list and
// validates each entry against the path-shape rules (no globs).
func (r *Rule) validateOutputs(
	filePath string, line int, params map[string]string,
) []lint.Diagnostic {
	raw, has := params["outputs"]
	outputs := splitList(raw)
	if !has || len(outputs) == 0 {
		return []lint.Diagnostic{gensection.MakeDiag(
			r.RuleID(), r.RuleName(), filePath, line,
			`build directive missing required "outputs" list`,
		)}
	}
	for _, out := range outputs {
		if msg := validatePathEntry(out, false); msg != "" {
			return []lint.Diagnostic{gensection.MakeDiag(
				r.RuleID(), r.RuleName(), filePath, line,
				fmt.Sprintf("build directive %q %s", out, msg),
			)}
		}
	}
	return nil
}

// validateInputs validates the optional "inputs" list. The list may be
// absent or empty; each entry is validated against the path-shape rules
// (doublestar globs allowed).
func (r *Rule) validateInputs(
	filePath string, line int, params map[string]string,
) []lint.Diagnostic {
	for _, in := range splitList(params["inputs"]) {
		if msg := validatePathEntry(in, true); msg != "" {
			return []lint.Diagnostic{gensection.MakeDiag(
				r.RuleID(), r.RuleName(), filePath, line,
				fmt.Sprintf("build directive %q %s", in, msg),
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
	known := map[string]bool{"recipe": true, "outputs": true, "inputs": true}
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

// generateBody renders the recipe's body-template once per outputs
// entry, in declared order, and joins the rendered lines with newlines.
// {output} and {alt} refer to the current entry in each iteration.
func (r *Rule) generateBody(
	_ string, _ int,
	params map[string]string,
) (string, []lint.Diagnostic) {
	recipeName := params["recipe"]
	outputs := splitList(params["outputs"])

	schema, _ := r.resolveRecipe(recipeName)
	tmpl := schema.BodyTemplate
	if tmpl == "" {
		tmpl = defaultBodyTemplate
	}

	rendered := make([]string, 0, len(outputs))
	for _, output := range outputs {
		alt := fmt.Sprintf("%s output: %s", recipeName, output)
		body := strings.NewReplacer("{output}", output, "{alt}", alt).Replace(tmpl)
		rendered = append(rendered, body)
	}
	body := strings.Join(rendered, "\n")

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

// reservedDeviceNames is the set of Windows reserved device names,
// rejected on every platform so a path that is harmless on Linux does
// not become a device handle when the same repo is built on Windows.
var reservedDeviceNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// splitList splits a newline-joined directive list value (the form
// gensection.ValidateStringParams produces for a YAML sequence) into
// trimmed, non-empty entries. An empty raw value yields nil.
//
// Empty and whitespace-only entries are intentionally preserved (not
// dropped) so validatePathEntry can flag them as diagnostics; only the
// trailing empty produced by a fully-empty value is removed.
func splitList(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// validatePathEntry validates one outputs: or inputs: entry against the
// path-shape allowlist. It returns "" when the entry is acceptable, or a
// short reason phrase suitable for embedding in a diagnostic. When
// allowGlob is true, doublestar glob meta-characters (*, ?, [, {) are
// permitted; outputs pass allowGlob=false and reject them.
func validatePathEntry(p string, allowGlob bool) string {
	if strings.TrimSpace(p) == "" {
		return "must not be empty"
	}
	if p != strings.TrimSpace(p) {
		return "must not have leading or trailing whitespace"
	}
	if strings.ContainsAny(p, "\x00\n\r") {
		return "must not contain NUL, newline, or carriage return"
	}
	if strings.Contains(p, `\`) {
		return "must use forward-slash separators only"
	}
	if hasDriveLetter(p) {
		return "must not be a Windows drive path"
	}
	if strings.Contains(p, ":") {
		return "must not contain a colon"
	}
	if hasReservedDeviceName(p) {
		return "must not use a reserved device name"
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") {
		return "must be a relative path"
	}
	if !allowGlob && strings.ContainsAny(p, "*?[{") {
		return "must not contain glob characters"
	}
	cleaned := path.Clean(p)
	if cleaned == ".." || cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return `must not contain ".." path components`
	}
	if UnderMdsmithDir(cleaned) {
		return "must not be under .mdsmith/"
	}
	return ""
}

// hasReservedDeviceName reports whether any slash-separated segment of p,
// with its extension stripped, is a Windows reserved device name
// (case-insensitive). "CON", "dir/NUL.txt", and "COM1.log" all match;
// "CONSOLE.md" does not.
func hasReservedDeviceName(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if dot := strings.IndexByte(seg, '.'); dot >= 0 {
			seg = seg[:dot]
		}
		if reservedDeviceNames[strings.ToUpper(seg)] {
			return true
		}
	}
	return false
}

// UnderMdsmithDir reports whether the cleaned, slash-separated path is
// the .mdsmith state directory or a file inside it. Exported so the build
// executor can apply the same reserved-path guard at exec time that this
// rule applies at lint time.
func UnderMdsmithDir(cleaned string) bool {
	return cleaned == ".mdsmith" || strings.HasPrefix(cleaned, ".mdsmith/")
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
