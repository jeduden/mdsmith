package schema

import (
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue/cuecontext"
)

// MergeRawMap applies plan-135 extends semantics to two raw inline
// schema maps. The child refines the parent: frontmatter keys
// declared by both unify via CUE `&`; sections in the child wholly
// replace the parent's sections; filename, closed, cross-references,
// acronyms, and index follow the same child-wins-when-set rule.
//
// Merge is purely structural — the function builds the `(parent) &
// (child)` CUE expression for shared keys without compiling or
// evaluating it. Use ValidateExtendedFrontmatter on the result when
// load-time conflict detection is needed (parent says `int`, child
// says `string`). Separating merge from validation lets the
// per-file merge path skip CUE eval once ValidateKinds has already
// run at config load.
//
// Both inputs must be the inline-shape map produced by the YAML
// loader; mutated copies are returned so the caller may further
// merge or hand them to ParseInline without aliasing. A nil input is
// treated as the empty map. The function does no CUE evaluation,
// so it cannot fail — callers that want load-time conflict
// detection run ValidateExtendedFrontmatter on the result.
func MergeRawMap(parent, child map[string]any) map[string]any {
	if parent == nil && child == nil {
		return nil
	}
	// Even single-input merges run through the frontmatter
	// normaliser so a shortcut like `date` becomes its canonical
	// CUE expression in the output. Without that, a child kind
	// that inherits an inline schema unchanged would still leave
	// bare shortcut names in the resolved view.
	out := cloneRawMap(parent)
	if out == nil {
		out = map[string]any{}
	}
	mergeRawFrontmatter(out, parent, child)
	// Overlay every other child key — including ones MergeRawMap
	// has no special semantics for — so ParseInline still gets to
	// flag a typoed/unknown top-level key with its
	// `unknown schema key` diagnostic. Without this, a misspelled
	// `sectiosn:` would be silently dropped in the single-layer
	// normalisation path.
	for key, v := range child {
		if key == "frontmatter" {
			continue
		}
		out[key] = v
	}
	return out
}

// mergeRawFrontmatter merges parent's and child's frontmatter maps
// in place on out. Each value is normalised via the same
// frontmatterExpr pipeline ParseInline uses, so bare-name
// shortcuts (`date`, `nonEmpty`, …) are expanded to their
// canonical CUE before composition — otherwise a unified
// expression like `(date) & (…)` would carry an unresolved
// identifier into the validator and fail to compile. Shared keys
// are joined with a CUE `&` conjunction without evaluating the
// result; callers that need load-time conflict detection run
// ValidateExtendedFrontmatter on the merged map.
//
// A `frontmatter` key that's present but isn't a mapping (the user
// wrote `frontmatter: "oops"` or a list) flows through verbatim
// rather than being silently dropped — ParseInline's
// `schema.frontmatter must be a mapping` diagnostic stays reachable
// downstream. Child malformed shape wins over parent so the
// closest declaration's error surfaces.
func mergeRawFrontmatter(out, parent, child map[string]any) {
	childRaw, childHas := child["frontmatter"]
	parentRaw, parentHas := parent["frontmatter"]
	childFM, childOK := childRaw.(map[string]any)
	parentFM, parentOK := parentRaw.(map[string]any)

	// Preserve a present-but-malformed frontmatter value so
	// ParseInline can surface its specific shape error. Child wins
	// when both are present — the closer-to-the-user declaration
	// drives the diagnostic.
	switch {
	case childHas && !childOK:
		out["frontmatter"] = childRaw
		return
	case parentHas && !parentOK && !childHas:
		out["frontmatter"] = parentRaw
		return
	}
	if !childOK && !parentOK {
		return
	}
	// No pre-sized capacity: CodeQL flags `len(a)+len(b)` as a
	// possible integer overflow when the inputs come from external
	// data, and frontmatter maps are tiny in practice so growing
	// from zero is fine. The same pattern is used in compose.go's
	// composeFrontmatter for the same reason.
	merged := map[string]any{}
	for k, v := range parentFM {
		merged[k] = normalizeFrontmatterValue(v)
	}
	if !childOK {
		out["frontmatter"] = merged
		return
	}
	for k, rawChildV := range childFM {
		childExpr := normalizeFrontmatterValue(rawChildV)
		existing, hadParent := merged[k]
		if !hadParent {
			merged[k] = childExpr
			continue
		}
		parentExpr, parentOK := existing.(string)
		if !parentOK {
			// The parent value failed normalisation (e.g. a
			// non-finite float). Keep it so
			// ValidateExtendedFrontmatter names the key rather
			// than the child override silently dropping the
			// malformed constraint.
			continue
		}
		childStr, childIsStr := childExpr.(string)
		if !childIsStr || parentExpr == childStr {
			merged[k] = childExpr
			continue
		}
		merged[k] = "(" + parentExpr + ") & (" + childStr + ")"
	}
	out["frontmatter"] = merged
}

// normalizeFrontmatterValue mirrors the per-value canonicalisation
// ParseInline applies — bare-name shortcuts expand to their CUE
// expression, scalars JSON-encode, and raw CUE strings pass
// through verbatim. A value frontmatterExpr cannot resolve (an
// unknown shortcut, an unsupported type) flows through unchanged
// so a downstream pass — ParseInline or
// ValidateExtendedFrontmatter — surfaces the same error signal
// the user would have seen without the extends merge.
func normalizeFrontmatterValue(v any) any {
	expr, err := frontmatterExpr(v)
	if err != nil {
		return v
	}
	return expr
}

// NormalizeFrontmatterValue applies the same canonicalisation
// extends merging uses — bare-name shortcuts to canonical CUE,
// scalars to JSON, raw strings verbatim — and returns the result
// as a string. Unsupported types (anything frontmatterExpr cannot
// resolve) fall back to a `%v`-formatted display so callers like
// `mdsmith kinds show` can render a value rather than an empty
// string.
func NormalizeFrontmatterValue(v any) string {
	if expr, err := frontmatterExpr(v); err == nil {
		return expr
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// ValidateExtendedFrontmatter CUE-checks the merged frontmatter,
// returning an UnsatisfiableKeyError naming the first key whose
// constraint cannot be satisfied. Each value is normalised through
// the same frontmatterExpr pipeline ParseInline uses, so a raw map
// that still carries bare-name shortcuts (`date`, `nonEmpty`) is
// validated against the same canonical CUE the parser would
// produce. Each key compiles in isolation so a unified expression
// like `(int) & (string)` surfaces on its owning key — sibling
// references inside an expression resolve against the same key,
// which is the only cross-field shape plan-135 frontmatter
// expressions need.
//
// Plan 135 surfaces these conflicts at config-load time so users
// see them on `mdsmith check` rather than as a per-file MDS020
// diagnostic.
func ValidateExtendedFrontmatter(raw map[string]any) error {
	fm, ok := raw["frontmatter"].(map[string]any)
	if !ok || len(fm) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		expr, err := frontmatterExpr(fm[k])
		if err != nil {
			return &InvalidFrontmatterError{
				Key:   stripOptionalSuffix(k),
				Value: fmt.Sprintf("%v", fm[k]),
				Cause: err,
			}
		}
		single := &Schema{Frontmatter: map[string]string{k: expr}}
		if err := checkUnifiable(single.FrontmatterCUE()); err != nil {
			parent, child := splitUnifiedExpr(expr)
			if child == "" {
				// A single-layer expression that compiles to bottom
				// (`int & string` declared verbatim, not as a
				// parent/child join). The diagnostic should not
				// imply an inheritance conflict.
				return &InvalidFrontmatterError{
					Key:   stripOptionalSuffix(k),
					Value: expr,
					Cause: err,
				}
			}
			return &UnsatisfiableKeyError{
				Key:    stripOptionalSuffix(k),
				Parent: parent,
				Child:  child,
				Cause:  err,
			}
		}
	}
	return nil
}

// splitUnifiedExpr undoes the `(parent) & (child)` form
// mergeRawFrontmatter builds, returning the two component
// expressions for diagnostic display. A verbatim expression (no
// unification) returns (expr, "") so the diagnostic still names
// the failing constraint.
func splitUnifiedExpr(expr string) (parent, child string) {
	if !strings.HasPrefix(expr, "(") || !strings.HasSuffix(expr, ")") {
		return expr, ""
	}
	depth := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth != 0 {
				continue
			}
			// The opening paren's match is at i. Expect the
			// separator `) & (` to follow; if not, the expression
			// isn't the unified shape this function produced.
			if i+5 > len(expr) || expr[i:i+5] != ") & (" {
				return expr, ""
			}
			return expr[1:i], expr[i+5 : len(expr)-1]
		}
	}
	return expr, ""
}

// cloneRawMap returns a shallow copy of m. The inline schema parser
// only inspects keys; the nested values it consumes (lists, maps,
// scalars) are not mutated downstream. A shallow copy is therefore
// sufficient to keep callers' inputs intact.
func cloneRawMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Extend produces a schema that inherits from parent and is refined by
// child. The semantics are single-parent inheritance with child-wins
// override and CUE-unification refinement (plan 135):
//
//   - Frontmatter keys unify. For a key both inputs declare, the
//     effective expression joins them with `&` so the value must
//     satisfy both. A key only the parent declares survives; a key
//     only the child declares is appended.
//   - Sections in the child wholly replace the parent's sections.
//     Heading templates compose by sequence, not by constraint, so a
//     child whose Sections is non-empty drops the parent's tree. A
//     child without sections inherits the parent's tree verbatim.
//   - Filename: child wins if non-empty, else parent's. Override is
//     wholesale — the child is free to pick a different filename
//     pattern (a draft-* variant of an RFC kind, for example) without
//     conflicting with the parent.
//   - Closed: the child's value wins when the child carries a
//     non-empty Sections list (its `closed:` describes its own
//     section tree); otherwise the parent's value flows through.
//   - CrossReferences, Acronyms, Index: child wins if set; else
//     parent's.
//
// A frontmatter key the child declares as a CUE expression that
// cannot unify with the parent's expression is detected statically
// via a CUE eval and surfaces as an unsatisfiable-key error. The
// caller wraps that error with the extends-chain context (plan
// 135's diagnostic shape).
//
// Extend returns nil when given two nil inputs. When parent is nil
// it returns the child unchanged; when child is nil it returns the
// parent unchanged.
func Extend(parent, child *Schema) (*Schema, error) {
	if parent == nil && child == nil {
		return nil, nil
	}
	if parent == nil {
		return child, nil
	}
	if child == nil {
		return parent, nil
	}

	out := &Schema{
		Source:    child.Source,
		RootLevel: extendRootLevel(parent, child),
	}

	if err := extendFrontmatter(out, parent, child); err != nil {
		return nil, err
	}
	extendFilename(out, parent, child)
	extendSections(out, parent, child)
	extendCrossRefs(out, parent, child)
	extendAcronyms(out, parent, child)
	extendIndex(out, parent, child)

	return out, nil
}

// extendRootLevel picks the child's root level when the child
// declares its own section tree (RootLevel is then meaningful);
// otherwise the parent's. A frontmatter-only child has RootLevel
// unset, which EffectiveRootLevel reports as 2 — falling through
// to the parent keeps the inherited section tree's level intact.
func extendRootLevel(parent, child *Schema) int {
	if len(child.Sections) > 0 {
		return child.EffectiveRootLevel()
	}
	return parent.EffectiveRootLevel()
}

// extendFrontmatter unifies parent's and child's frontmatter
// constraints. For shared keys, the child's expression refines the
// parent's via CUE `&`; the result is a single CUE expression the
// schema engine validates against the document. A key whose
// expressions cannot unify (parent says `int`, child says `string`)
// surfaces as an unsatisfiable-key error so the caller can wrap it
// with the extends-chain context.
func extendFrontmatter(out, parent, child *Schema) error {
	if len(parent.Frontmatter) == 0 && len(child.Frontmatter) == 0 {
		return nil
	}
	// Allocate without a size hint: CodeQL flags `len(a)+len(b)`
	// as a possible overflow when both sides are untrusted; the
	// maps are tiny in practice (a handful of frontmatter keys),
	// so the grow-from-zero cost is negligible.
	out.Frontmatter = map[string]string{}
	out.FrontmatterLines = mergeFrontmatterLines(
		parent.FrontmatterLines, child.FrontmatterLines)
	out.FrontmatterMeta = mergeFrontmatterMeta(
		parent.FrontmatterMeta, child.FrontmatterMeta)

	for k, expr := range parent.Frontmatter {
		out.Frontmatter[k] = expr
	}
	for k, expr := range child.Frontmatter {
		parentExpr, ok := parent.Frontmatter[k]
		if !ok || parentExpr == expr {
			out.Frontmatter[k] = expr
			continue
		}
		unified := "(" + parentExpr + ") & (" + expr + ")"
		// Compile in struct context (`close({ <k>: <unified> })`)
		// so an expression that legitimately references the field
		// by name — e.g. a constraint like `len(<k>) > 0` — sees
		// the same scope ParseInline gives it.
		single := &Schema{Frontmatter: map[string]string{k: unified}}
		if err := checkUnifiable(single.FrontmatterCUE()); err != nil {
			return &UnsatisfiableKeyError{
				Key:    stripOptionalSuffix(k),
				Parent: parentExpr,
				Child:  expr,
				Cause:  err,
			}
		}
		out.Frontmatter[k] = unified
	}
	return nil
}

// mergeFrontmatterMeta merges parent and child deprecation metadata
// for plan 136. Child-declared metadata wins on key collisions so a
// kind extending its parent can re-deprecate a field with a fresher
// message. Undoing a parent's deprecation by setting
// `deprecated: false` on the child is not supported in this plan:
// ExtractFieldMeta only classifies a mapping as metadata when the
// literal `deprecated: true` discriminator is present, so a child
// meta block with `deprecated: false` flows through to the CUE
// struct path and never produces a FrontmatterMeta entry the
// merger could see. Removing the field from the schema entirely
// (plan 136 acceptance criterion 6) is the supported migration
// end-state.
func mergeFrontmatterMeta(parent, child map[string]FieldMeta) map[string]FieldMeta {
	if len(parent) == 0 && len(child) == 0 {
		return nil
	}
	out := map[string]FieldMeta{}
	for k, v := range parent {
		out[k] = v
	}
	for k, v := range child {
		out[k] = v
	}
	return out
}

// mergeFrontmatterLines builds a per-key source-line map giving
// child-declared lines precedence; parent-only keys keep their
// recorded lines so a parent-side validation error points at the
// parent schema even after extension.
func mergeFrontmatterLines(parent, child map[string]int) map[string]int {
	if len(parent) == 0 && len(child) == 0 {
		return nil
	}
	// Same overflow concern as extendFrontmatter — skip the size
	// hint; the per-key line maps are tiny in practice.
	out := map[string]int{}
	for k, v := range parent {
		out[k] = v
	}
	for k, v := range child {
		out[k] = v
	}
	return out
}

// stripOptionalSuffix removes the trailing "?" optional-field marker
// from a frontmatter key for diagnostic display. The internal map
// preserves the marker so callers can still tell required from
// optional, but the user-facing key is the bare name.
func stripOptionalSuffix(key string) string {
	return strings.TrimSuffix(key, "?")
}

// extendFilename picks the child's filename pattern when set, else
// the parent's. The child can override the parent wholesale —
// inheritance is about composing constraints, but a kind's filename
// is the kind's identity and a child variant routinely wants its
// own pattern (a draft-* RFC, a ratified-* RFC).
func extendFilename(out, parent, child *Schema) {
	if child.Filename != "" {
		out.Filename = child.Filename
		return
	}
	out.Filename = parent.Filename
}

// extendSections copies the child's sections wholesale when it
// declares any; otherwise the parent's tree flows through. Plan 135
// explicitly rejects sequence-level unification of heading templates
// so the simpler rule wins.
func extendSections(out, parent, child *Schema) {
	if len(child.Sections) > 0 {
		out.Sections = append([]Scope(nil), child.Sections...)
		out.Closed = child.Closed
		return
	}
	out.Sections = append([]Scope(nil), parent.Sections...)
	out.Closed = parent.Closed
}

func extendCrossRefs(out, parent, child *Schema) {
	if len(child.CrossReferences) > 0 {
		out.CrossReferences = append([]CrossRef(nil), child.CrossReferences...)
		return
	}
	out.CrossReferences = append([]CrossRef(nil), parent.CrossReferences...)
}

func extendAcronyms(out, parent, child *Schema) {
	if child.Acronyms != nil {
		out.Acronyms = child.Acronyms
		return
	}
	out.Acronyms = parent.Acronyms
}

func extendIndex(out, parent, child *Schema) {
	if child.Index != nil {
		out.Index = child.Index
		return
	}
	out.Index = parent.Index
}

// UnsatisfiableKeyError reports a frontmatter key whose child
// expression cannot unify with the parent's. The caller wraps it
// with the kind names so the rendered diagnostic carries the full
// extends-chain context (plan 135 / plan 147 shape).
type UnsatisfiableKeyError struct {
	Key    string
	Parent string
	Child  string
	Cause  error
}

// Error implements error.
func (e *UnsatisfiableKeyError) Error() string {
	return fmt.Sprintf(
		"%s: schema cannot unify with parent (parent: %s, child: %s): %v",
		e.Key, e.Parent, e.Child, e.Cause)
}

// Unwrap exposes the underlying CUE error so callers can introspect
// it when needed.
func (e *UnsatisfiableKeyError) Unwrap() error { return e.Cause }

// InvalidFrontmatterError reports a frontmatter value the parser
// could not turn into a CUE expression — an unknown shortcut
// name, an unsupported value type. Unlike UnsatisfiableKeyError
// this is not a parent/child unification conflict; the value
// alone is invalid, so the diagnostic stays focused on the key
// rather than implying an inheritance mismatch.
type InvalidFrontmatterError struct {
	Key   string
	Value string
	Cause error
}

// Error implements error.
func (e *InvalidFrontmatterError) Error() string {
	return fmt.Sprintf("frontmatter key %q (%s): %v",
		e.Key, e.Value, e.Cause)
}

// Unwrap exposes the underlying parse error.
func (e *InvalidFrontmatterError) Unwrap() error { return e.Cause }

// checkUnifiable reports whether a CUE expression can be reduced
// without contradiction. It compiles the expression in a fresh CUE
// context; the compiled value's Err() is non-nil whenever the
// expression reduces to bottom (CUE's "no value satisfies"
// outcome), so a simple Err()-check covers every conflict shape
// the plan cares about — `int & string`, conflicting bounds,
// closed-struct violations, unresolved references.
func checkUnifiable(expr string) error {
	v := cuecontext.New().CompileString(expr)
	return v.Err()
}
