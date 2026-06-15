package emptysectionbody

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS030 is the per-Check ceiling for empty-section-body.
// Two optimizations reduce the alloc count:
//   - topLevelNodes pre-sizes its slice to cap 8, saving ~3 backing-array
//     growth allocations for a typical document with 8 top-level nodes.
//   - allowMarkerDirective is precomputed in the Rule struct so Check does
//     not call fmt.Sprintf on the directive string per violation.
const allocBudgetMDS030 = 3

const allocBudgetFixture = "# Document title\n" +
	"\n" +
	"A short prose paragraph for the readability and structural\n" +
	"rules to scan. It stays one paragraph long.\n" +
	"\n" +
	"## Section\n" +
	"\n" +
	"See [other](other.md) and [label][ref] for examples.\n" +
	"\n" +
	"```go\nfunc f() int { return 0 }\n```\n" +
	"\n" +
	"- one item\n" +
	"- two items\n" +
	"\n" +
	"| Col | Other |\n" +
	"|-----|-------|\n" +
	"| a   | b     |\n" +
	"\n" +
	"[ref]: https://example.com/\n"

// TestCheck_AllowMarkerDirectiveInMessage verifies that the precomputed
// allowMarkerDirective appears correctly in violation messages.
func TestCheck_AllowMarkerDirectiveInMessage(t *testing.T) {
	src := []byte("## Empty Section\n\n## Next Section\n\nSome content here.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{
		MinLevel:             2,
		MaxLevel:             6,
		AllowMarker:          "allow-empty-section",
		allowMarkerDirective: "<?allow-empty-section?>",
	}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic for empty section")
	assert.Contains(t, diags[0].Message, "<?allow-empty-section?>",
		"message should contain the precomputed directive")
}

// TestCheck_CustomAllowMarker verifies the precomputed directive is updated
// when AllowMarker is customised via ApplySettings.
func TestCheck_CustomAllowMarker(t *testing.T) {
	src := []byte("## Empty Section\n\n## Next Section\n\nSome content here.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{
		MinLevel:    2,
		MaxLevel:    6,
		AllowMarker: "allow-empty-section",
	}
	require.NoError(t, r.ApplySettings(map[string]any{"allow-marker": "my-marker"}))
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "<?my-marker?>",
		"message should use updated allowMarkerDirective")
}

func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	src := []byte(allocBudgetFixture)
	r := &Rule{
		MinLevel:             defaultMinLevel,
		MaxLevel:             defaultMaxLevel,
		AllowMarker:          defaultAllowMarker,
		allowMarkerDirective: "<?" + defaultAllowMarker + "?>",
	}
	warm, err := lint.NewFile("warm.md", src)
	require.NoError(t, err)
	_ = r.Check(warm)

	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		_, _ = lint.NewFile("parse.md", src)
	})
	full := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("check.md", src)
		require.NoError(t, err)
		_ = r.Check(f)
	})
	delta := full - parse
	if delta < 0 {
		delta = 0
	}
	t.Logf("MDS030 Check allocs/op = %.0f (budget = %d)", delta, allocBudgetMDS030)
	require.LessOrEqualf(t, delta, float64(allocBudgetMDS030),
		"MDS030 Check allocs/op = %.0f, budget = %d",
		delta, allocBudgetMDS030)
}
