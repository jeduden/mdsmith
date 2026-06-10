package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderRow is a test helper: compile and render a row expression against a
// front-matter map in one call.
func renderRow(t *testing.T, expr string, scope map[string]any) (string, error) {
	t.Helper()
	tpl, err := CompileRow(expr)
	require.NoError(t, err)
	return tpl.Render(scope)
}

func TestCompileRow_RejectsEmpty(t *testing.T) {
	_, err := CompileRow("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty row expression")
}

func TestCompileRow_RejectsSyntaxError(t *testing.T) {
	_, err := CompileRow("strings.Join([for x in")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid row expression")
}

func TestRender_StringLiteral(t *testing.T) {
	got, err := renderRow(t, `"literal"`, nil)
	require.NoError(t, err)
	assert.Equal(t, "literal", got)
}

func TestRender_ScalarInterpolation(t *testing.T) {
	got, err := renderRow(t, `"\(id) - \(name)"`, map[string]any{
		"id":   "MDS001",
		"name": "line-length",
	})
	require.NoError(t, err)
	assert.Equal(t, "MDS001 - line-length", got)
}

func TestRender_InterpolationOfNumber(t *testing.T) {
	got, err := renderRow(t, `"n=\(count)"`, map[string]any{"count": 42})
	require.NoError(t, err)
	assert.Equal(t, "n=42", got)
}

func TestRender_InterpolationOfBool(t *testing.T) {
	got, err := renderRow(t, `"on=\(flag)"`, map[string]any{"flag": true})
	require.NoError(t, err)
	assert.Equal(t, "on=true", got)
}

func TestRender_InterpolationOfFloat(t *testing.T) {
	got, err := renderRow(t, `"w=\(weight)"`, map[string]any{"weight": 1.5})
	require.NoError(t, err)
	assert.Equal(t, "w=1.5", got)
}

func TestRender_InterpolationOfWholeFloatKeepsPoint(t *testing.T) {
	got, err := renderRow(t, `"w=\(weight)"`, map[string]any{"weight": 2.0})
	require.NoError(t, err)
	assert.Equal(t, "w=2.0", got)
}

func TestRender_NestedInterpolation(t *testing.T) {
	got, err := renderRow(t, `"\("\(id)")"`, map[string]any{"id": "X"})
	require.NoError(t, err)
	assert.Equal(t, "X", got)
}

func TestRender_InterpolationEscapes(t *testing.T) {
	got, err := renderRow(t, `"a\tb \(id)"`, map[string]any{"id": "Z"})
	require.NoError(t, err)
	assert.Equal(t, "a\tb Z", got)
}

func TestRender_InterpolationOfNullIsError(t *testing.T) {
	_, err := renderRow(t, `"\(x)"`, map[string]any{"x": nil})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid interpolation")
}

func TestRender_InterpolationOfListIsError(t *testing.T) {
	_, err := renderRow(t, `"\(x)"`, map[string]any{"x": []any{1, 2}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid interpolation")
}

func TestRender_StringConcatenation(t *testing.T) {
	got, err := renderRow(t, `id + " " + name`, map[string]any{
		"id": "A", "name": "b",
	})
	require.NoError(t, err)
	assert.Equal(t, "A b", got)
}

func TestRender_NumberAdditionIsNotAString(t *testing.T) {
	_, err := renderRow(t, `1 + 1`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "concrete string")
}

func TestRender_MixedAddIsError(t *testing.T) {
	_, err := renderRow(t, `"a" + 1`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid operation")
}

func TestRender_ConditionalTernary(t *testing.T) {
	onCase, err := renderRow(t, `[if def {"on"}, if !def {"off"}][0]`, map[string]any{"def": true})
	require.NoError(t, err)
	assert.Equal(t, "on", onCase)
	offCase, err := renderRow(t, `[if def {"on"}, if !def {"off"}][0]`, map[string]any{"def": false})
	require.NoError(t, err)
	assert.Equal(t, "off", offCase)
}

func TestRender_ListComprehensionAndJoin(t *testing.T) {
	expr := `strings.Join([for m in markdownlint {"\(m.id) \(m.name)"}], ", ")`
	got, err := renderRow(t, expr, map[string]any{
		"markdownlint": []any{
			map[string]any{"id": "MD018", "name": "no-missing-space-atx"},
			map[string]any{"id": "MD019", "name": "no-multiple-space-atx"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "MD018 no-missing-space-atx, MD019 no-multiple-space-atx", got)
}

func TestRender_ForOverEmptyList(t *testing.T) {
	got, err := renderRow(t, `strings.Join([for x in items {"\(x)"}], ",")`, map[string]any{
		"items": []any{},
	})
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

func TestRender_EmptyListComparison(t *testing.T) {
	got, err := renderRow(t, `[if items == [] {"—"}, if items != [] {"full"}][0]`, map[string]any{
		"items": []any{},
	})
	require.NoError(t, err)
	assert.Equal(t, "—", got)
}

func TestRender_NonEmptyListComparison(t *testing.T) {
	got, err := renderRow(t, `[if items == [] {"—"}, if items != [] {"full"}][0]`, map[string]any{
		"items": []any{"a"},
	})
	require.NoError(t, err)
	assert.Equal(t, "full", got)
}

func TestRender_FMStructAccess(t *testing.T) {
	got, err := renderRow(t, `"\(fm.id)"`, map[string]any{"id": "MDS001"})
	require.NoError(t, err)
	assert.Equal(t, "MDS001", got)
}

func TestRender_FMQuotedKeyAccess(t *testing.T) {
	got, err := renderRow(t, `fm["my-key"]`, map[string]any{"my-key": "value"})
	require.NoError(t, err)
	assert.Equal(t, "value", got)
}

func TestRender_FMListIndex(t *testing.T) {
	got, err := renderRow(t, `"\(fm.markdownlint[0].id) \(fm.markdownlint[0].name)"`, map[string]any{
		"markdownlint": []any{
			map[string]any{"id": "MD013", "name": "line-length"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "MD013 line-length", got)
}

func TestRender_StringsJoinLiterals(t *testing.T) {
	got, err := renderRow(t, `strings.Join(["a", "b", "c"], "-")`, nil)
	require.NoError(t, err)
	assert.Equal(t, "a-b-c", got)
}

func TestRender_LenOfList(t *testing.T) {
	got, err := renderRow(t, `"\(len(items))"`, map[string]any{"items": []any{1, 2, 3}})
	require.NoError(t, err)
	assert.Equal(t, "3", got)
}

func TestRender_LenOfString(t *testing.T) {
	got, err := renderRow(t, `"\(len(id))"`, map[string]any{"id": "abc"})
	require.NoError(t, err)
	assert.Equal(t, "3", got)
}

func TestRender_NilScope(t *testing.T) {
	got, err := renderRow(t, `"literal"`, nil)
	require.NoError(t, err)
	assert.Equal(t, "literal", got)
}

func TestRender_UnknownFieldIsError(t *testing.T) {
	_, err := renderRow(t, `"\(missing)"`, map[string]any{"id": "X"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_NonStringResultIsError(t *testing.T) {
	_, err := renderRow(t, `42`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "concrete string")
}

func TestRender_SelectorOnAbsentFieldIsError(t *testing.T) {
	_, err := renderRow(t, `"\(m.absent)"`, map[string]any{
		"m": map[string]any{"id": "X"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_SelectorOnNonStructIsError(t *testing.T) {
	_, err := renderRow(t, `"\(id.sub)"`, map[string]any{"id": "X"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot select")
}

func TestRender_ListIndexOutOfRangeIsError(t *testing.T) {
	_, err := renderRow(t, `items[5]`, map[string]any{"items": []any{"a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestRender_BuiltinArityError(t *testing.T) {
	_, err := renderRow(t, `strings.Join(["a"])`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two arguments")
}

// bigRowExpr is the canonical coverage-matrix row expression from
// docs/research/markdownlint-coverage/README.md: the richest real row-expr,
// combining string interpolation, `+` concatenation, the ternary idiom,
// nested for-comprehensions, strings.Join, empty-list guards, and quoted-key
// fm access. It is the acceptance corpus's heaviest single expression.
const bigRowExpr = `
  "| [\(id)](../../../internal/rules/" +
  "\(id)-\(name)/README.md) \(name)" +
  [if status != "ready" {" (not-ready)"},
   if status == "ready" {""}][0] +
  " | " +
  [if markdownlint == [] {"—"},
   if markdownlint != [] {
     strings.Join([for m in markdownlint {
       "\(m.id) " +
       [if m.default {"✅"}, if !m.default {"⚪"}][0] +
       [if m.id != m.name {" \(m.name)"},
        if m.id == m.name {""}][0] +
       [if m.partial {" (partial)"},
        if !m.partial {""}][0]
     }], ", ")
   }][0] +
  " | " +
  [if rumdl == [] {"—"},
   if rumdl != [] {
     strings.Join([for m in rumdl {
       "\(m.id) " +
       [if m.default {"✅"}, if !m.default {"⚪"}][0] +
       [if m.id != m.name {" \(m.name)"},
        if m.id == m.name {""}][0] +
       [if m.partial {" (partial)"},
        if !m.partial {""}][0]
     }], ", ")
   }][0] +
  " |"`

// peerEntry builds one peer-linter list entry the big row-expr iterates.
func peerEntry(id, name string, def, partial bool) map[string]any {
	return map[string]any{"id": id, "name": name, "default": def, "partial": partial}
}

func TestRender_BigCoverageRowExpr(t *testing.T) {
	scope := map[string]any{
		"id":     "MDS064",
		"name":   "atx-heading-whitespace",
		"status": "ready",
		"markdownlint": []any{
			peerEntry("MD018", "no-missing-space-atx", true, false),
			peerEntry("MD020", "no-missing-space-closed-atx", true, true),
		},
		"rumdl": []any{},
	}
	got, err := renderRow(t, bigRowExpr, scope)
	require.NoError(t, err)
	// markdownlint: id==name false so the name is appended; MD020 partial.
	// rumdl: empty list renders the em dash.
	want := "| [MDS064](../../../internal/rules/MDS064-atx-heading-whitespace/README.md) " +
		"atx-heading-whitespace | " +
		"MD018 ✅ no-missing-space-atx, MD020 ✅ no-missing-space-closed-atx (partial) | — |"
	assert.Equal(t, want, got)
}

func TestRender_BigCoverageRowExpr_NotReadyStatus(t *testing.T) {
	scope := map[string]any{
		"id":           "MDS029",
		"name":         "conciseness-scoring",
		"status":       "draft",
		"markdownlint": []any{},
		"rumdl":        []any{},
	}
	got, err := renderRow(t, bigRowExpr, scope)
	require.NoError(t, err)
	want := "| [MDS029](../../../internal/rules/MDS029-conciseness-scoring/README.md) " +
		"conciseness-scoring (not-ready) | — | — |"
	assert.Equal(t, want, got)
}

// --- item 3: rowEqual struct and type-strict list element equality ---

func TestRender_StructEqualityIsFieldWise(t *testing.T) {
	// CUE: {k:1} == {k:1} is true (struct equality compares field-wise).
	got, err := renderRow(t, `[if x == y {"T"}, if x != y {"F"}][0]`, map[string]any{
		"x": map[string]any{"k": 1},
		"y": map[string]any{"k": 1},
	})
	require.NoError(t, err)
	assert.Equal(t, "T", got)
}

func TestRender_StructInequalityWhenFieldDiffers(t *testing.T) {
	got, err := renderRow(t, `[if x == y {"T"}, if x != y {"F"}][0]`, map[string]any{
		"x": map[string]any{"k": 1},
		"y": map[string]any{"k": 2},
	})
	require.NoError(t, err)
	assert.Equal(t, "F", got)
}

func TestRender_ListElementEqualityIsTypeStrict(t *testing.T) {
	// CUE: [2] == [2.0] is FALSE — list element equality is kind-strict, even
	// though the scalar 2 == 2.0 is true.
	got, err := renderRow(t, `[if x == y {"T"}, if x != y {"F"}][0]`, map[string]any{
		"x": []any{2},
		"y": []any{2.0},
	})
	require.NoError(t, err)
	assert.Equal(t, "F", got)
}

func TestRender_ScalarNumericEqualityCrossesKinds(t *testing.T) {
	// CUE: top-level 2 == 2.0 is true (numeric-aware).
	got, err := renderRow(t, `[if x == y {"T"}, if x != y {"F"}][0]`, map[string]any{
		"x": 2, "y": 2.0,
	})
	require.NoError(t, err)
	assert.Equal(t, "T", got)
}

// --- item 4: len is a byte count, not a rune count ---

func TestRender_LenOfStringIsByteCount(t *testing.T) {
	// CUE's len(string) is the BYTE count: "café" is 5 bytes (é is 2 bytes).
	got, err := renderRow(t, `"\(len(s))"`, map[string]any{"s": "café"})
	require.NoError(t, err)
	assert.Equal(t, "5", got)
}

func TestRender_LenOfMultibyteEmoji(t *testing.T) {
	got, err := renderRow(t, `"\(len(s))"`, map[string]any{"s": "😀"})
	require.NoError(t, err)
	assert.Equal(t, "4", got)
}
