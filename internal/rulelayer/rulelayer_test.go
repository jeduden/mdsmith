package rulelayer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auditSignal decodes the embedded manifest and returns an id→signal map for
// one boolean field (selected by sel). It reads auditJSON — the embedded copy
// TestEmbeddedManifestMatchesAuditOracle keeps byte-identical to the on-disk
// oracle — so the override-soundness tests share one decode path and no longer
// duplicate the os.ReadFile/filepath.Join/Unmarshal boilerplate.
func auditSignal(t *testing.T, sel func(auditSignalEntry) bool) map[string]bool {
	t.Helper()
	var entries []auditSignalEntry
	require.NoError(t, json.Unmarshal(auditJSON, &entries))
	m := make(map[string]bool, len(entries))
	for _, e := range entries {
		m[e.ID] = sel(e)
	}
	return m
}

// auditSignalEntry is the subset of the rule-walk manifest the override and
// promotion tests read: the rule id, its category, and the two boolean signals
// the soundness contracts pin.
type auditSignalEntry struct {
	ID           string `json:"id"`
	Category     string `json:"category"`
	NilASTSafe   bool   `json:"nil_ast_safe"`
	ReadsFileAST bool   `json:"reads_file_ast"`
}

// TestEmbeddedManifestMatchesAuditOracle keeps the embedded copy of the
// rule-walk manifest byte-identical to the audit oracle the integration
// gate enforces. If the audit re-classifies a rule, this fails until the
// embedded copy is refreshed, so the engine's parse-skip gate and the
// audit gate can never disagree about a rule's layer.
func TestEmbeddedManifestMatchesAuditOracle(t *testing.T) {
	oracle, err := os.ReadFile(filepath.Join(
		"..", "integration", "testdata", "rule_walk_audit.json"))
	require.NoError(t, err)
	assert.Equal(t, string(oracle), string(auditJSON),
		"embedded manifest drifted from the audit oracle; copy "+
			"internal/integration/testdata/rule_walk_audit.json into "+
			"internal/rulelayer/")
}

func TestIsLayer0(t *testing.T) {
	// A few representative "A-no-skipping" rules resolve to Layer 0.
	// MDS013 (blank-line-around-headings) and MDS044 (horizontal-rule-
	// style) were migrated to rule.BlockChecker (plan 2606141903): their
	// nil-AST paths serve from the Layer 0 block scan, so the audit
	// reclassified them A-no-skipping and the gate now admits them.
	for _, id := range []string{"MDS006", "MDS007", "MDS013", "MDS022", "MDS044", "MDS064"} {
		assert.True(t, IsLayer0(id), "%s should be Layer 0", id)
		assert.Equal(t, Layer0, Of(id))
	}
	// MDS047 (ambiguous-emphasis) and MDS054 (no-undefined-reference-labels)
	// are "A-no-skipping" and formerly forced to AST because they read the
	// inline code-span ranges. Those ranges are now backed on the nil-AST
	// path by the shared run-grouped inline parse (lint.InlineBlocks), so the
	// override is gone and both resolve to Layer 0.
	for _, id := range []string{"MDS047", "MDS054"} {
		assert.True(t, IsLayer0(id), "%s should be Layer 0 once code spans are backed", id)
		assert.Equal(t, Layer0, Of(id))
	}
	// MDS012 (no-bare-urls) and MDS018 (no-emphasis-as-heading) were
	// "hybrid": they walked the inline tree (Text nodes for bare URLs, a
	// lone Emphasis child for emphasis-as-heading) and produced no
	// diagnostics on a nil AST. Layer 1 re-backs both via the per-block
	// inline parse (internal/lint/inline_blocks.go + inline_emphasis.go):
	// each implements rule.BlockChecker so the engine's block-span dispatch
	// drives them on the parse-skipped File. The audit now classes them
	// "A-no-skipping" and the gate admits both.
	// MDS012 (no-bare-urls), MDS018 (no-emphasis-as-heading), MDS032
	// (no-empty-alt-text), and MDS062 (link-validity) were "hybrid": each
	// walked the inline tree and produced no (or different) diagnostics on
	// a nil AST. Layer 1 re-backs all four on the shared run-grouped inline
	// parse (internal/lint/inline_blocks.go, inline_emphasis.go): the three
	// NodeChecker rules implement rule.InlineChecker so the engine routes
	// them to their own Check on a nil-AST File, and MDS062 (a plain Check
	// rule) reads the same projection in its Check. The audit now classes
	// all four "A-no-skipping".
	for _, id := range []string{"MDS012", "MDS018", "MDS032", "MDS062"} {
		assert.True(t, IsLayer0(id), "%s should be Layer 0 once re-backed on Layer 1", id)
		assert.Equal(t, Layer0, Of(id))
	}
	// The inline-content parity rules (plan 2606171404) read heading text,
	// links, emphasis, reference definitions, or flavor-specific spans. Each
	// serves the nil-AST path from the shared run-grouped inline parse
	// (lint.InlineBlocks) — heading rules and MDS053's def/use map walk it,
	// MDS034 also reads the Layer 0 BlockQuote spans for alert blockquotes —
	// so the audit classes them "A-no-skipping" and the gate admits them.
	for _, id := range []string{"MDS005", "MDS017", "MDS034", "MDS042", "MDS049", "MDS053", "MDS063", "MDS068"} {
		assert.True(t, IsLayer0(id), "%s should be Layer 0 once re-backed on Layer 1", id)
		assert.Equal(t, Layer0, Of(id))
	}
	// The "B-prose-only" rules (plan 2606171258) are nil-AST-safe but read
	// code content the audit's code-perturbation probe scrambles: MDS041
	// inline HTML / HTML blocks, MDS050 code-block + HTML-block bodies under
	// check-code / check-html, MDS052 code-span content, MDS066 fenced-code
	// bodies. The code guard that once forced them to parse is gone and the
	// validated ClassifyLines / Layer-1 inline projections reproduce that
	// content byte-identically on the nil-AST File, so nilASTBackable promotes
	// them to Layer 0 (gated on nil_ast_safe). The corpus equivalence harness
	// and each rule's TestCheck_NilASTMatchesAST guard the promotion.
	for _, id := range []string{"MDS041", "MDS050", "MDS052", "MDS066"} {
		assert.True(t, IsLayer0(id), "%s (B-prose-only, nil-AST-safe) should be Layer 0", id)
		assert.Equal(t, Layer0, Of(id))
	}
	// AST-requiring rules are not Layer 0.
	for _, id := range []string{"MDS001", "MDS019", "MDS023"} {
		assert.False(t, IsLayer0(id), "%s should require the AST", id)
		assert.Equal(t, LayerAST, Of(id))
	}
}

// TestNilASTBackable is the dedicated unit test for nilASTBackable, the
// predicate buildLayerMapFrom uses to decide Layer 0 promotion (before the
// astProjectionConsumers veto). It covers all four arms: the knownNilASTSafe
// override, the "A-no-skipping" category, a nil-AST-safe "B-prose-only" rule
// (promoted), a "B-prose-only" rule the manifest marks NOT nil-AST-safe
// (withheld), and an AST-requiring category (withheld).
func TestNilASTBackable(t *testing.T) {
	t.Cleanup(func() { delete(knownNilASTSafe, "MDS950") })
	knownNilASTSafe["MDS950"] = true

	assert.True(t, nilASTBackable(auditEntry{ID: "MDS950", Category: "ast-required"}),
		"knownNilASTSafe override promotes regardless of category")
	assert.True(t, nilASTBackable(auditEntry{ID: "MDS951", Category: "A-no-skipping"}),
		"A-no-skipping is backable")
	assert.True(t, nilASTBackable(auditEntry{ID: "MDS952", Category: "B-prose-only", NilASTSafe: true}),
		"nil-AST-safe B-prose-only is backable")
	assert.False(t, nilASTBackable(auditEntry{ID: "MDS953", Category: "B-prose-only", NilASTSafe: false}),
		"B-prose-only without nil_ast_safe is not backable")
	assert.False(t, nilASTBackable(auditEntry{ID: "MDS954", Category: "ast-required", NilASTSafe: true}),
		"ast-required is not backable even when nil_ast_safe")
}

// TestBProseOnlyRulesAreNilASTSafe pins the soundness invariant behind the
// category-wide B-prose-only promotion: every rule the live manifest marks
// "B-prose-only" also reports nil_ast_safe: true, so nilASTBackable never
// promotes a B-prose-only rule whose nil-AST run diverged from the AST run
// (such a rule lands in "hybrid", not "B-prose-only"). If the audit ever
// emits a B-prose-only entry with nil_ast_safe: false, this fails and the
// promotion must be reconsidered.
func TestBProseOnlyRulesAreNilASTSafe(t *testing.T) {
	var entries []auditSignalEntry
	require.NoError(t, json.Unmarshal(auditJSON, &entries))
	saw := false
	for _, e := range entries {
		if e.Category != "B-prose-only" {
			continue
		}
		saw = true
		assert.True(t, e.NilASTSafe,
			"%s is B-prose-only; the category promotion requires nil_ast_safe: true", e.ID)
		assert.True(t, IsLayer0(e.ID), "%s (nil-AST-safe B-prose-only) must resolve to Layer 0", e.ID)
	}
	assert.True(t, saw, "manifest must contain at least one B-prose-only rule or the check is vacuous")
}

// TestBuildLayerMapFromBProseOnlyPromotesToLayer0 exercises the B-prose-only
// arm of buildLayerMapFrom directly with a synthetic entry: a "B-prose-only"
// rule reporting nil_ast_safe must resolve to Layer0.
func TestBuildLayerMapFromBProseOnlyPromotesToLayer0(t *testing.T) {
	m := buildLayerMapFrom([]byte(`[
		{"id":"MDS905","category":"B-prose-only","nil_ast_safe":true}
	]`))
	assert.Equal(t, Layer0, m["MDS905"], "nil-AST-safe B-prose-only promotes to Layer0")
}

// TestBuildLayerMapFromBProseOnlyRequiresNilASTSafe pins the nil_ast_safe gate
// on the B-prose-only arm: a B-prose-only entry the manifest marks NOT
// nil-AST-safe must stay LayerAST, so a hand-edit that mislabels an
// AST-fragile rule B-prose-only cannot silently admit it to the parse-skip
// gate. The live audit never emits this combination, so the guard is
// exercised here with a synthetic entry.
func TestBuildLayerMapFromBProseOnlyRequiresNilASTSafe(t *testing.T) {
	m := buildLayerMapFrom([]byte(`[
		{"id":"MDS906","category":"B-prose-only","nil_ast_safe":false}
	]`))
	assert.Equal(t, LayerAST, m["MDS906"],
		"a B-prose-only entry without nil_ast_safe must stay AST")
}

// TestAstProjectionConsumersAreNotLayer0 guards the parse-skip gate's
// soundness invariant: any rule the manifest marks "A-no-skipping" yet
// listed in astProjectionConsumers (because it reads an AST-only projection
// Layer N does not yet back) must be forced to AST, so the gate never
// admits it. The map is empty today — Layer 1 backs the code-span ranges
// that were its only entries — so this test is vacuous but pins the
// invariant for any future entry.
func TestAstProjectionConsumersAreNotLayer0(t *testing.T) {
	for id := range astProjectionConsumers {
		assert.False(t, IsLayer0(id),
			"%s consumes an AST-only projection; gate must not admit it", id)
		assert.Equal(t, LayerAST, Of(id))
	}
}

// TestKnownNilASTSafeAreLayer0 confirms every rule in the manual
// knownNilASTSafe override resolves to Layer 0 even though its audit
// category is not A-no-skipping. MDS069 (unique-frontmatter) is the
// canonical entry: a cross-file rule the bad-fixture probe cannot fire, so
// the audit leaves it inconclusive-not-fired, but it never reads f.AST.
func TestKnownNilASTSafeAreLayer0(t *testing.T) {
	for id := range knownNilASTSafe {
		assert.True(t, IsLayer0(id), "%s is in knownNilASTSafe; must be Layer 0", id)
		assert.Equal(t, Layer0, Of(id))
	}
	assert.True(t, knownNilASTSafe["MDS069"], "MDS069 must be in the override set")
}

// TestKnownNilASTSafeOnlyListsNonASTReaders guards the override's manual
// commitment: a rule may only be force-classified Layer 0 here when its
// audit manifest entry reports reads_file_ast: false. A rule that grows an
// f.AST read trips its static signal and this test fails until the override
// is reconsidered.
func TestKnownNilASTSafeOnlyListsNonASTReaders(t *testing.T) {
	readsAST := auditSignal(t, func(e auditSignalEntry) bool { return e.ReadsFileAST })
	for id := range knownNilASTSafe {
		_, present := readsAST[id]
		assert.True(t, present, "%s in knownNilASTSafe must appear in the audit manifest", id)
		assert.False(t, readsAST[id],
			"%s in knownNilASTSafe must have reads_file_ast: false", id)
	}
}

func TestUnknownRuleIsNotLayer0(t *testing.T) {
	assert.False(t, IsLayer0("MDS999"))
	assert.Equal(t, LayerUnknown, Of("MDS999"))
}

// TestBuildLayerMapFromPanicsOnMalformedManifest drives the decode-failure
// branch: malformed manifest JSON is a build-time contract violation, so the
// builder panics rather than returning a degraded table.
func TestBuildLayerMapFromPanicsOnMalformedManifest(t *testing.T) {
	assert.Panics(t, func() {
		buildLayerMapFrom([]byte("{not json"))
	})
}

// TestBuildLayerMapFromClassifies confirms the standard category arms of
// buildLayerMapFrom: an "A-no-skipping" rule maps to Layer0, a nil-AST-safe
// "B-prose-only" rule maps to Layer0, and any other category maps to LayerAST
// (absent a knownNilASTSafe override).
func TestBuildLayerMapFromClassifies(t *testing.T) {
	m := buildLayerMapFrom([]byte(`[
		{"id":"MDS900","category":"A-no-skipping","nil_ast_safe":true},
		{"id":"MDS901","category":"ast-required","nil_ast_safe":false},
		{"id":"MDS907","category":"B-prose-only","nil_ast_safe":true}
	]`))
	assert.Equal(t, Layer0, m["MDS900"])
	assert.Equal(t, LayerAST, m["MDS901"])
	assert.Equal(t, Layer0, m["MDS907"])
}

// TestBuildLayerMapFromKnownNilASTSafePromotesToLayer0 exercises the
// knownNilASTSafe branch of buildLayerMapFrom directly with a synthetic
// entry, independently of the live layerByID table. A non-A-no-skipping
// rule listed in knownNilASTSafe must resolve to Layer0.
func TestBuildLayerMapFromKnownNilASTSafePromotesToLayer0(t *testing.T) {
	knownNilASTSafe["MDS904"] = true
	t.Cleanup(func() { delete(knownNilASTSafe, "MDS904") })
	m := buildLayerMapFrom([]byte(`[
		{"id":"MDS904","category":"inconclusive-not-fired"}
	]`))
	assert.Equal(t, Layer0, m["MDS904"], "knownNilASTSafe promotes non-A-no-skipping rule to Layer0")
}

// TestAstProjectionConsumerOverrideForcesAST pins the override arm of the
// category switch independently of the (currently empty) production map: a
// rule whose category is "A-no-skipping" but which is listed in
// astProjectionConsumers must still resolve to LayerAST. It mutates the
// package map for the duration of one buildLayerMapFrom call so the test
// does not depend on a live override entry.
func TestAstProjectionConsumerOverrideForcesAST(t *testing.T) {
	astProjectionConsumers["MDS902"] = true
	t.Cleanup(func() { delete(astProjectionConsumers, "MDS902") })
	m := buildLayerMapFrom([]byte(`[
		{"id":"MDS902","category":"A-no-skipping"}
	]`))
	assert.Equal(t, LayerAST, m["MDS902"], "override forces an A-no-skipping rule to AST")
}

// TestAstProjectionConsumerOverridesKnownNilASTSafe pins that astProjectionConsumers
// wins over knownNilASTSafe: a rule in both must still resolve to LayerAST because
// it reads a projection the nil-AST path does not back, regardless of whether its
// Check never directly dereferences f.AST.
func TestAstProjectionConsumerOverridesKnownNilASTSafe(t *testing.T) {
	astProjectionConsumers["MDS903"] = true
	knownNilASTSafe["MDS903"] = true
	t.Cleanup(func() {
		delete(astProjectionConsumers, "MDS903")
		delete(knownNilASTSafe, "MDS903")
	})
	m := buildLayerMapFrom([]byte(`[
		{"id":"MDS903","category":"inconclusive-not-fired"}
	]`))
	assert.Equal(t, LayerAST, m["MDS903"], "astProjectionConsumers must override knownNilASTSafe")
}

// TestAstProjectionConsumerOverridesBProseOnly pins that astProjectionConsumers
// wins over the B-prose-only promotion: a rule in astProjectionConsumers must
// still resolve to LayerAST because it reads an AST-only projection the Layer 0
// / Layer 1 path does not back, regardless of its nil-AST-safe B-prose-only
// category. This guards the soundness escape hatch — buildLayerMapFrom gates
// the whole Layer 0 promotion behind !astProjectionConsumers[e.ID].
func TestAstProjectionConsumerOverridesBProseOnly(t *testing.T) {
	astProjectionConsumers["MDS904"] = true
	t.Cleanup(func() { delete(astProjectionConsumers, "MDS904") })
	m := buildLayerMapFrom([]byte(`[
		{"id":"MDS904","category":"B-prose-only","nil_ast_safe":true}
	]`))
	assert.Equal(t, LayerAST, m["MDS904"], "astProjectionConsumers must override the B-prose-only promotion")
}
