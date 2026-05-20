package schema

import (
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
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
// treated as the empty map.
func MergeRawMap(parent, child map[string]any) (map[string]any, error) {
	if parent == nil && child == nil {
		return nil, nil
	}
	if len(parent) == 0 {
		return cloneRawMap(child), nil
	}
	if len(child) == 0 {
		return cloneRawMap(parent), nil
	}
	out := cloneRawMap(parent)
	mergeRawFrontmatter(out, parent, child)
	for _, key := range []string{
		"sections", "filename", "closed",
		"cross-references", "acronyms", "index",
	} {
		if v, ok := child[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

// mergeRawFrontmatter merges parent's and child's frontmatter maps
// in place on out. Shared keys are joined with a CUE `&`
// conjunction without evaluating the result — callers that need
// load-time conflict detection run ValidateExtendedFrontmatter on
// the merged map. Keys present in one side only flow through
// verbatim.
func mergeRawFrontmatter(out, parent, child map[string]any) {
	childFM, childOK := child["frontmatter"].(map[string]any)
	parentFM, parentOK := parent["frontmatter"].(map[string]any)
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
		merged[k] = v
	}
	if !childOK {
		out["frontmatter"] = merged
		return
	}
	for k, childV := range childFM {
		parentV, hadParent := parentFM[k]
		if !hadParent {
			merged[k] = childV
			continue
		}
		parentExpr, parentExprOK := parentV.(string)
		childExpr, childExprOK := childV.(string)
		if !parentExprOK || !childExprOK || parentExpr == childExpr {
			merged[k] = childV
			continue
		}
		merged[k] = "(" + parentExpr + ") & (" + childExpr + ")"
	}
	out["frontmatter"] = merged
}

// ValidateExtendedFrontmatter CUE-checks the merged frontmatter,
// returning an UnsatisfiableKeyError naming the first key whose
// constraint cannot be satisfied. The check runs against the full
// frontmatter struct so a unified expression that legitimately
// references a sibling field resolves correctly — the same way the
// per-file validator builds and evaluates frontmatter CUE.
//
// Plan 135 surfaces these conflicts at config-load time so users
// see them on `mdsmith check` rather than as a per-file MDS020
// diagnostic.
func ValidateExtendedFrontmatter(raw map[string]any) error {
	fm, ok := raw["frontmatter"].(map[string]any)
	if !ok || len(fm) == 0 {
		return nil
	}
	sch := &Schema{Frontmatter: make(map[string]string, len(fm))}
	for k, v := range fm {
		if expr, ok := v.(string); ok {
			sch.Frontmatter[k] = expr
		}
	}
	if len(sch.Frontmatter) == 0 {
		return nil
	}
	if err := checkUnifiable(sch.FrontmatterCUE()); err == nil {
		return nil
	}
	// Walk the keys in sorted order so the offender is named
	// deterministically. Each single-key compile resolves
	// cross-field references against just that key — sufficient
	// for the conflict shapes plan 135 cares about (a unified
	// expression like `(int) & (string)` fails on its own struct,
	// no other keys involved).
	keys := make([]string, 0, len(sch.Frontmatter))
	for k := range sch.Frontmatter {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		single := &Schema{Frontmatter: map[string]string{k: sch.Frontmatter[k]}}
		if err := checkUnifiable(single.FrontmatterCUE()); err != nil {
			parent, child := splitUnifiedExpr(sch.Frontmatter[k])
			return &UnsatisfiableKeyError{
				Key:    stripOptionalSuffix(k),
				Parent: parent,
				Child:  child,
				Cause:  err,
			}
		}
	}
	// Whole-struct compile failed but no individual key did — a
	// cross-key contradiction. The wrapper at the call site
	// (ValidateKindInlineSchema) renders this with the kind name
	// so users still see where to look.
	return fmt.Errorf("schema does not compile")
}

// splitUnifiedExpr undoes the `(parent) & (child)` form
// mergeRawFrontmatter builds, returning the two component
// expressions for diagnostic display. A verbatim expression (no
// unification) returns (expr, "") so the diagnostic still names
// the failing constraint.
func splitUnifiedExpr(expr string) (parent, child string) {
	if !strings.HasPrefix(expr, "(") {
		return expr, ""
	}
	depth := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i+4 < len(expr) && expr[i+1:i+5] == ") & " {
				// shouldn't happen — but defensively fall through.
				continue
			}
			if depth == 0 {
				// Expect `) & (` next; otherwise this isn't a
				// unified expression we produced.
				if i+4 >= len(expr) || expr[i+1:i+5] != " & (" {
					return expr, ""
				}
				rest := expr[i+5:]
				if !strings.HasSuffix(rest, ")") {
					return expr, ""
				}
				return expr[1:i], rest[:len(rest)-1]
			}
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
		if err := checkUnifiable(unified); err != nil {
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

// checkUnifiable reports whether a CUE expression can be reduced
// without contradiction. It compiles the expression in a fresh CUE
// context; a bottom result (CUE's "no value satisfies" outcome) is
// returned as an error so the caller can surface the conflict at
// schema-extend time rather than later at validation time.
//
// The check is intentionally limited to syntactic / type
// contradictions: a parent of `int` unified with a child of
// `string` reduces to bottom and is rejected here. Constraint-level
// conflicts that require concrete inputs (`>5 & <3`) reduce to
// bottom only when a document supplies the value, and surface
// through the existing front-matter validator on each linted file.
func checkUnifiable(expr string) error {
	ctx := cuecontext.New()
	v := ctx.CompileString(expr)
	if err := v.Err(); err != nil {
		return err
	}
	if err := v.Validate(cue.Concrete(false)); err != nil {
		return err
	}
	return nil
}
