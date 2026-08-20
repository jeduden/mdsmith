package forbiddentext

import (
	"strconv"
	"strings"
	"sync"
)

// matcherCache memoizes compiled automata by needle list. Compiling is
// O(total needle bytes x alphabet) and allocates the transition table,
// while ApplySettings runs once per config signature per worker — so
// without this the same handful of lists (usually just the one a
// project configures) were rebuilt many times per run, which a CPU
// profile showed costing more than the matching it saved.
//
// A *matcher is immutable once built, so sharing one across rule
// instances and goroutines is safe. Entries live for the process, which
// is bounded by the number of distinct needle lists in a workspace.
var matcherCache sync.Map // string -> *matcher

// matcherCacheKey encodes needles into a string that no other list can
// produce. Every entry is length-prefixed, so no choice of separator
// byte lets one list impersonate another — joining on a separator
// would let a needle containing that byte collide with the two-needle
// list it splits into, and a `contains:` entry can hold any byte.
func matcherCacheKey(needles []string) string {
	var b strings.Builder
	size := 0
	for _, s := range needles {
		size += len(s) + 8
	}
	b.Grow(size)
	for _, s := range needles {
		b.WriteString(strconv.Itoa(len(s)))
		b.WriteByte(':')
		b.WriteString(s)
	}
	return b.String()
}

// cachedMatcher returns the compiled matcher for needles, building it
// at most once per distinct list.
func cachedMatcher(needles []string) *matcher {
	key := matcherCacheKey(needles)
	if v, ok := matcherCache.Load(key); ok {
		m, _ := v.(*matcher)
		return m
	}
	m := newMatcher(needles)
	actual, _ := matcherCache.LoadOrStore(key, m)
	got, _ := actual.(*matcher)
	return got
}

// matcher answers one question about a paragraph — "does any configured
// needle occur in this text?" — in a single pass over the text, instead
// of one full strings.Contains scan per needle.
//
// The rule's cost is dominated by compliant paragraphs, which match
// nothing: with N needles the old shape paid N end-to-end scans to
// learn that, and a merged CPU profile of `mdsmith check .` on this
// repository (which pins `convention: no-llm-tells`, ~70 needles)
// attributed 9.4% of all CPU to it, almost entirely inside
// strings.Contains. See
// docs/development/high-performance-go.md#data-structures.
//
// matcher deliberately reports only whether SOME needle matched. The
// rule still determines which ones with its original per-needle loop,
// which runs only for the rare paragraph that actually violates — so
// diagnostic order, duplicate handling, and messages come from
// unchanged code.
//
// The automaton is a fully-folded Aho-Corasick DFA: every input byte is
// one table index with no fail-link walking. Transitions are stored
// over a compact alphabet of just the bytes that appear in the needles
// (plus one shared "other" symbol), which keeps the table at
// nodes x len(alphabet) int32 rather than nodes x 256.
type matcher struct {
	// symbol maps an input byte to its alphabet index. Zero means the
	// byte appears in no needle; all such bytes share one symbol,
	// which is sound because no needle can continue through them.
	symbol [256]uint8

	// next is the flattened DFA transition table, nsym entries per
	// node: next[state*nsym+sym] is the successor state.
	next []int32

	// terminal reports whether a needle ends at a state or at any of
	// its suffix states, so a match needs no output-chain walk.
	terminal []bool

	nsym int
}

// newMatcher compiles needles into a matcher. Empty needles are
// ignored — they match everything and the rule skips them. Returns nil
// when no needle survives, which callers treat as "never matches".
func newMatcher(needles []string) *matcher {
	m := &matcher{}
	if !m.buildAlphabet(needles) {
		return nil
	}
	trie, isEnd := m.buildTrie(needles)
	m.foldFailLinks(trie, isEnd)
	return m
}

// buildAlphabet assigns each byte that appears in a needle its own
// symbol index, leaving every other byte on the shared symbol 0. It
// reports whether any usable needle was found.
func (m *matcher) buildAlphabet(needles []string) bool {
	var seen [256]bool
	any := false
	for _, s := range needles {
		if s == "" {
			continue
		}
		any = true
		for i := 0; i < len(s); i++ {
			seen[s[i]] = true
		}
	}
	if !any {
		return false
	}
	m.nsym = 1 // symbol 0 is the shared "byte in no needle" symbol
	for b := 0; b < 256; b++ {
		if seen[b] {
			m.symbol[b] = uint8(m.nsym)
			m.nsym++
		}
	}
	return true
}

// buildTrie returns the flattened goto table (-1 where no edge exists)
// and, per node, whether a needle ends there.
func (m *matcher) buildTrie(needles []string) (trie []int32, isEnd []bool) {
	addNode := func() int32 {
		id := int32(len(isEnd))
		trie = append(trie, make([]int32, m.nsym)...)
		for i := 0; i < m.nsym; i++ {
			trie[int(id)*m.nsym+i] = -1
		}
		isEnd = append(isEnd, false)
		return id
	}
	addNode() // root == 0
	for _, s := range needles {
		if s == "" {
			continue
		}
		cur := int32(0)
		for i := 0; i < len(s); i++ {
			sym := int(m.symbol[s[i]])
			nxt := trie[int(cur)*m.nsym+sym]
			if nxt < 0 {
				nxt = addNode()
				trie[int(cur)*m.nsym+sym] = nxt
			}
			cur = nxt
		}
		isEnd[cur] = true
	}
	return trie, isEnd
}

// foldFailLinks walks the trie breadth-first and folds each node's
// fail link into m.next, producing a DFA where every (state, symbol)
// pair has a direct successor and no match needs an output-chain walk.
func (m *matcher) foldFailLinks(trie []int32, isEnd []bool) {
	nodes := len(isEnd)
	m.next = make([]int32, nodes*m.nsym)
	m.terminal = make([]bool, nodes)
	fail := make([]int32, nodes)

	queue := make([]int32, 0, nodes)
	for sym := 0; sym < m.nsym; sym++ {
		child := trie[sym] // root's edge
		if child < 0 {
			m.next[sym] = 0
			continue
		}
		m.next[sym] = child
		fail[child] = 0
		queue = append(queue, child)
	}
	// The root stays non-terminal: only an empty needle could end
	// there, and those are skipped above.
	for head := 0; head < len(queue); head++ {
		u := queue[head]
		// A state matches if a needle ends here or ends at any of its
		// proper suffixes, which fail[u] already summarises.
		m.terminal[u] = isEnd[u] || m.terminal[fail[u]]
		for sym := 0; sym < m.nsym; sym++ {
			child := trie[int(u)*m.nsym+sym]
			if child < 0 {
				m.next[int(u)*m.nsym+sym] = m.next[int(fail[u])*m.nsym+sym]
				continue
			}
			m.next[int(u)*m.nsym+sym] = child
			fail[child] = m.next[int(fail[u])*m.nsym+sym]
			queue = append(queue, child)
		}
	}
}

// matches reports whether any needle occurs in text. One table lookup
// per input byte, no allocation.
func (m *matcher) matches(text string) bool {
	if m == nil {
		return false
	}
	state := int32(0)
	for i := 0; i < len(text); i++ {
		state = m.next[int(state)*m.nsym+int(m.symbol[text[i]])]
		if m.terminal[state] {
			return true
		}
	}
	return false
}
