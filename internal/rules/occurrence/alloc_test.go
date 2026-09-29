package occurrence

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// checkConfiguredAllocBudget is the CLAUDE.md per-Check ceiling. The
// gate below measures a configured rule on a fresh File per run, so
// the per-file prose memo is charged to Check, and on a document with
// more paragraphs than the ceiling so a per-paragraph allocation
// cannot hide under it.
const checkConfiguredAllocBudget = 10

const checkConfiguredDoc = `# Title

Alpha paragraph with a Token and ` + "`token`" + ` code.

## First

Beta paragraph mentions token twice: token.

Gamma paragraph has no match at all here.

Delta paragraph, TOKEN in capitals.

## Second

Epsilon paragraph — with an em dash — or two.

Zeta paragraph keeps going with token text.

Eta paragraph is plain prose for the scan.

Theta paragraph adds a token once more.

## Third

Iota paragraph closes one section out.

Kappa paragraph with token and token.

Lambda paragraph ends the fixture here.

Mu paragraph is the final token line.
`

// TestCheck_Configured_AllocBudget runs every scope × count mode with
// case-insensitive tokens and with a pattern. Before the prose memo,
// each paragraph paid a text extraction and a strings.ToLower, so the
// tokens path crossed the ceiling at six paragraphs.
func TestCheck_Configured_AllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}
	src := []byte(checkConfiguredDoc)
	settings := map[string]map[string]any{
		"tokens":  {"tokens": []any{"token", "paragraph"}, "max": 50},
		"pattern": {"pattern": "—", "max": 50},
		// A literal with letters takes the case-folding count path.
		"folded-literal": {"pattern": "Token", "max": 50},
	}
	for name, base := range settings {
		for _, scope := range []string{"file", "section", "paragraph"} {
			for _, count := range []string{"each", "combined"} {
				s := map[string]any{"scope": scope, "count": count}
				for k, v := range base {
					s[k] = v
				}
				r := &Rule{}
				if err := r.ApplySettings(r.DefaultSettings()); err != nil {
					t.Fatal(err)
				}
				if err := r.ApplySettings(s); err != nil {
					t.Fatal(err)
				}
				t.Run(name+"/"+scope+"/"+count, func(t *testing.T) {
					parse := testing.AllocsPerRun(100, func() {
						_, _ = lint.NewFile("a.md", src)
					})
					full := testing.AllocsPerRun(100, func() {
						f, _ := lint.NewFile("a.md", src)
						_ = r.Check(f)
					})
					allocs := full - parse
					t.Logf("allocs/op = %.1f (budget = %d)", allocs, checkConfiguredAllocBudget)
					if allocs > checkConfiguredAllocBudget {
						t.Fatalf("Check allocates %.1f/op, budget = %d",
							allocs, checkConfiguredAllocBudget)
					}
				})
			}
		}
	}
}
