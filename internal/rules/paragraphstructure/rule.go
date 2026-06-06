package paragraphstructure

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
)

// sentBufPool reuses the []string destination handed to
// mdtext.SplitSentencesInto so the per-Check `make([]string, 0, n)`
// the bare SplitSentences call would force is amortized across
// Check invocations. Each pooled slice grows once to fit its
// largest paragraph; subsequent reuses hit zero allocations for the
// slice. Storing *[]string (not []string) lets Put/Get round-trip
// the slice header without boxing it into the pool's any.
var sentBufPool = sync.Pool{
	New: func() any {
		s := make([]string, 0, 16)
		return &s
	},
}

func init() {
	rule.Register(&Rule{MaxSentences: 6, MaxWords: 40})
}

// Rule checks that paragraphs do not exceed sentence and word limits.
type Rule struct {
	MaxSentences int
	MaxWords     int
	Placeholders []string // placeholder tokens to treat as opaque
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS024" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "paragraph-structure" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable. MDS024 is opt-in
// because exact sentence counting and per-sentence word counting
// require the trained Punkt segmenter
// (github.com/neurosnap/sentences), which the neutral CPU profile
// recorded in plan 187 attributes ~20% of mdsmith's wall time on
// prose-heavy input. Punkt's cost is the trained model's regex
// execution (english.MultiPunctWordAnnotation.tokenAnnotation
// runs reAbbr and the token-type matchers with backtracking on
// every period-ending token), and no pure-Go Punkt-equivalent
// faster segmenter exists — plan 187 records the negative with a
// reusable equivalence harness. Users who want the diagnostic
// enable it explicitly; the default check path stops paying the
// ~20%.
//
// NOTE: This is a behaviour change. Before this rule implemented
// rule.Defaultable, MDS024 ran on every default check. Existing
// .mdsmith.yml configs that did not pin paragraph-structure will
// no longer emit prose-structure diagnostics until they opt in
// via `rules: { paragraph-structure: true }`.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	// Iterate the per-File memoized non-table paragraph collection so
	// the AST walk is shared with the other paragraph-walking rules
	// (MDS023 paragraph-readability, MDS057 required-text-patterns,
	// MDS058 required-mentions) instead of being re-run here. Plan 196
	// made the per-paragraph text lazy, so the walk no longer carries
	// the ExtractPlainText cost; MDS024 still materialises the text
	// per paragraph via ExtractText because every paragraph reaches
	// the segmenter.
	//
	// Note: MDS024 stays on the bare collector even though it
	// ALWAYS materialises text. The
	// [astutil.CollectSectionParagraphsWithText] variant would
	// share the materialisation with MDS057/MDS058 when those
	// opt-in rules run, but it also adds a per-Check slice copy
	// + interface boxing for the memo store — enough to put this
	// rule over its 9-alloc/op budget (plan 193). The bare
	// collector keeps MDS024 inside the budget; the extra
	// extraction cost when MDS057/MDS058 are co-enabled is paid
	// per-paragraph in the bare ExtractText calls.
	var diags []lint.Diagnostic //nolint:prealloc // pre-sizing would exceed the 9-alloc/op budget (plan 193)
	for _, p := range astutil.CollectSectionParagraphs(f) {
		diags = append(diags, r.checkParagraph(p.ExtractText(f.Source), p.Line, f.Path)...)
	}
	return diags
}

// checkParagraph evaluates one paragraph against the sentence-count
// and per-sentence word limits. text is the raw extracted plain
// text; line is its 1-based source line; both come from the shared
// collector. Placeholder masking stays per-rule so the shared text
// is not coupled to one rule's config.
func (r *Rule) checkParagraph(text string, line int, filePath string) []lint.Diagnostic {
	if len(r.Placeholders) > 0 {
		text = placeholders.MaskBodyTokens(text, r.Placeholders)
	}
	// Fast path: skip the Punkt tokenizer when this paragraph
	// provably cannot violate either limit. Punkt places a boundary
	// only at '.'/'!'/'?' and yields >=1 sentence, so
	// (terminal-punct + 1) is an upper bound on its sentence count;
	// and every sentence's word count is <= the whole paragraph's.
	// mdtext.SplitSentences dominated allocations (~2 GB over a
	// 600-file corpus, plan 175 profiling); most real paragraphs
	// clear this guard with zero allocation.
	if sentUB, words := cheapBounds(text); sentUB <= r.MaxSentences && words <= r.MaxWords {
		return nil
	}

	// Pool the segmenter's result slice: bare mdtext.SplitSentences
	// would `make([]string, 0, n)` on every Check, costing 1 alloc
	// per call on the budget gate. The pool amortizes it. The slice
	// must not escape this function — every call reaches the Put
	// below before returning.
	bufPtr := sentBufPool.Get().(*[]string)
	*bufPtr = mdtext.SplitSentencesInto((*bufPtr)[:0], text)
	sentences := *bufPtr
	var diags []lint.Diagnostic

	if len(sentences) > r.MaxSentences {
		// Hand-built string instead of fmt.Sprintf: Sprintf builds
		// a format-string scratch buffer and boxes each arg into
		// `any` before formatting (~3 allocs per call). The
		// concat + strconv.Itoa form below is 1–3 allocs depending
		// on the int values — Itoa caches the decimal form for
		// values 0–99 (returns a substring of strconv's precomputed
		// `smallsString` table, no alloc), and larger values
		// allocate one string each. The runtime.concatstrings
		// lowering of the `+` chain produces one final string. For
		// MDS024's typical inputs (max-sentences ~6, counted ~7–30)
		// both Itoa calls hit the cache, so the message build
		// costs one allocation; for larger paragraphs it grows.
		diags = append(diags, lint.Diagnostic{
			File:     filePath,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message: "paragraph has too many sentences (" +
				strconv.Itoa(len(sentences)) + " > " +
				strconv.Itoa(r.MaxSentences) + ")",
		})
	}

	for _, sent := range sentences {
		wc := mdtext.CountWords(sent)
		if wc > r.MaxWords {
			diags = append(diags, lint.Diagnostic{
				File:     filePath,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message: "sentence too long (" +
					strconv.Itoa(wc) + " > " +
					strconv.Itoa(r.MaxWords) + " words): " +
					sentencePreview(sent, 10),
			})
		}
	}
	// Return the sentence buffer to the pool. Clear first.
	// `sentences[:0]` shrinks len to zero but leaves cap (and the
	// backing array) untouched — the GC scans the slice's full
	// allocated size, not just its current len, so string headers
	// at indices ≥ len still pin their referent text alive through
	// the pool's reachable holder. Without the clear, a 10 KB
	// paragraph's text would stay in memory until the pool's
	// next-cycle sweep evicts the entry. Verified: putting
	// `["MARK"]` then `[:0]` then Put, immediately reading
	// `pool.Get()[:cap]` still returns `["MARK"]` at index 0.
	// See plan 193 follow-up.
	clear(sentences)
	*bufPtr = sentences[:0]
	sentBufPool.Put(bufPtr)
	return diags
}

// cheapBounds returns, in one allocation-free pass, an upper bound
// on the Punkt sentence count (terminal-punct + 1) and the exact
// word count (whitespace-delimited, matching strings.Fields). Both
// are conservative for the rule's checks: Punkt never splits without
// a terminal-punct rune and always yields >=1 sentence, and no
// single sentence has more words than the whole paragraph.
//
// The terminal-punct set covers every rune the English Punkt
// pipeline (and internal/punkt's vendored fork) actually flags as a
// sentence break: ASCII `.!?` via HasSentEndChars + HasPeriodFinal,
// and CJK `。` via HasPeriodFinal's full-width branch. The CJK
// `！` and `？` runes are word boundaries (IsCjkPunct) but not
// sentence boundaries — the English pipeline's hasSentEndChars set
// is ASCII-only — so a paragraph that uses only `！`/`？` between
// sentences segments as ONE sentence upstream. Counting `！`/`？`
// here would let the guard fail to short-circuit safe input, not
// cause a false positive — but it would also be misleading; the
// set is kept aligned with actual SentBreak boundaries so the
// guard stays a tight upper bound.
func cheapBounds(s string) (sentUB, words int) {
	punct := 0
	inWord := false
	for _, r := range s {
		switch r {
		case '.', '!', '?', '。':
			punct++
		}
		if mdtext.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			words++
		}
	}
	return punct + 1, words
}

// sentencePreview returns a quoted preview of the sentence, truncated
// to approximately limit words with "..." appended if truncated.
func sentencePreview(sent string, limit int) string {
	words := strings.Fields(strings.TrimSpace(sent))
	if len(words) <= limit {
		return fmt.Sprintf("%q", strings.Join(words, " "))
	}
	return fmt.Sprintf("%q", strings.Join(words[:limit], " ")+" ...")
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "max-sentences":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-structure: max-sentences must be an integer, got %T", v,
				)
			}
			r.MaxSentences = n
		case "max-words-per-sentence":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-structure: max-words-per-sentence must be an integer, got %T", v,
				)
			}
			r.MaxWords = n
		case "placeholders":
			toks, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-structure: placeholders must be a list of strings, got %T", v,
				)
			}
			if err := placeholders.Validate(toks); err != nil {
				return fmt.Errorf("paragraph-structure: %w", err)
			}
			r.Placeholders = toks
		default:
			return fmt.Errorf("paragraph-structure: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max-sentences":          6,
		"max-words-per-sentence": 40,
		"placeholders":           []string{},
	}
}

// SettingMergeMode implements rule.ListMerger.
func (r *Rule) SettingMergeMode(key string) rule.MergeMode {
	if key == "placeholders" {
		return rule.MergeAppend
	}
	return rule.MergeReplace
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
