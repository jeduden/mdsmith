package release

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jeduden/mdsmith/internal/mdfence"
)

// GitHubRelease is the subset of a GitHub release the website's
// release-notes page reads.
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

// SiteRelease is one entry of website/data/releases.json.
type SiteRelease struct {
	Tag       string    `json:"tag"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Published time.Time `json:"published"`
	Body      string    `json:"body"`
}

// SiteReleases is the website/data/releases.json document the
// /releases/ page renders: stable releases, then candidates, each
// newest first.
type SiteReleases struct {
	Stable     []SiteRelease `json:"stable"`
	Candidates []SiteRelease `json:"candidates"`
}

// releaseHeadingShift demotes body headings below the page's own
// structure: the page is h1, the stable/candidate groups are h2,
// and each release is h3, so the notes' "## What's Changed"
// becomes h4.
const releaseHeadingShift = 2

// BuildSiteReleases splits published releases into stable ones and
// candidates by GitHub's prerelease flag, drops drafts, normalizes
// each body, and sorts both lists newest first (tag descending on a
// tie, so the output is stable). Each candidate's body loses the
// entries its next-older candidate already listed (see
// candidateDelta), so a candidate shows only its own changes.
// Normalizing then demotes and scopes the body's headings and
// defuses Hugo shortcode syntax (see defuseShortcodes).
func BuildSiteReleases(rels []GitHubRelease) SiteReleases {
	out := SiteReleases{Stable: []SiteRelease{}, Candidates: []SiteRelease{}}
	for _, r := range rels {
		if r.Draft {
			continue
		}
		name := r.Name
		if name == "" {
			name = r.TagName
		}
		sr := SiteRelease{
			Tag:       r.TagName,
			Name:      name,
			URL:       r.HTMLURL,
			Published: r.PublishedAt,
			Body:      strings.TrimSpace(strings.ReplaceAll(r.Body, "\r\n", "\n")),
		}
		if r.Prerelease {
			out.Candidates = append(out.Candidates, sr)
		} else {
			out.Stable = append(out.Stable, sr)
		}
	}
	sortNewestFirst(out.Stable)
	sortNewestFirst(out.Candidates)
	out.Candidates = candidatesAfterStable(out.Candidates, out.Stable)
	subtractPredecessors(out.Candidates)
	finalizeBodies(out.Stable)
	finalizeBodies(out.Candidates)
	return out
}

// finalizeBodies demotes and scopes each body's headings and defuses
// its Hugo shortcode syntax. It runs after subtractPredecessors,
// which compares raw bodies: the heading rewrite scopes ids per tag,
// so rewritten headings would never compare equal.
func finalizeBodies(rs []SiteRelease) {
	for i := range rs {
		body := rewriteHeadings(rs[i].Body, releaseHeadingShift, headingSlug(rs[i].Tag))
		rs[i].Body = defuseShortcodes(body)
	}
}

// subtractPredecessors rewrites each candidate's body, newest first,
// to candidateDelta against the next-older candidate's raw body. The
// oldest candidate keeps its full notes.
func subtractPredecessors(cands []SiteRelease) {
	for i := 0; i+1 < len(cands); i++ {
		prev := cands[i+1]
		cands[i].Body = candidateDelta(cands[i].Body, prev.Body, prev.Tag, cands[i].Tag)
	}
}

// candidateDelta returns body without the entries prev already
// lists, for a candidate tag whose previous candidate is prevTag.
// Candidate notes published before release-notes started each
// candidate at the previous one span every change since the last
// stable release, and published releases are immutable, so the page
// subtracts instead. Notes that already start at prevTag share no
// entries with prev and come back whole.
//
// An entry is a non-blank line that is not an ATX heading; lines are
// compared with surrounding spaces trimmed. A heading left with no
// entries is dropped. GitHub's "**Full Changelog**" line ends the
// last section, and its compare link is rebased onto prevTag. Runs
// of blank lines collapse to one.
func candidateDelta(body, prev, prevTag, tag string) string {
	seen := map[string]bool{}
	for _, l := range strings.Split(prev, "\n") {
		if t := strings.TrimSpace(l); t != "" && !isHeadingLine(l) {
			seen[t] = true
		}
	}
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	var heading string
	pending := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case t == "":
			if !pending {
				out = append(out, "")
			}
		case isHeadingLine(l):
			heading, pending = l, true
		case isFullChangelogLine(l):
			pending = false
			out = append(out, rebaseCompareLink(l, prevTag, tag))
		case seen[t]:
		default:
			if pending {
				out = append(out, heading)
				pending = false
			}
			out = append(out, l)
		}
	}
	return collapseBlankLines(out)
}

// isHeadingLine reports whether l is an ATX heading indented at most
// three spaces.
func isHeadingLine(l string) bool {
	t := strings.TrimLeft(l, " ")
	return len(l)-len(t) <= 3 && atxLevel(t) > 0
}

// isFullChangelogLine reports whether l is GitHub's generated
// "**Full Changelog**: <compare link>" line.
func isFullChangelogLine(l string) bool {
	return strings.HasPrefix(strings.TrimSpace(l), "**Full Changelog**")
}

// rebaseCompareLink points a ".../compare/<base>...<tag>" link in l
// at prevTag as its base. A line without such a link, with
// whitespace in the base, or whose "...<tag>" only prefixes a longer
// tag (rc.2 inside rc.20) comes back unchanged.
func rebaseCompareLink(l, prevTag, tag string) string {
	const marker = "/compare/"
	i := strings.Index(l, marker)
	if i < 0 {
		return l
	}
	start := i + len(marker)
	rest := l[start:]
	j := strings.Index(rest, "..."+tag)
	if j <= 0 || strings.ContainsAny(rest[:j], " \t") {
		return l
	}
	if end := j + 3 + len(tag); end < len(rest) && isTagByte(rest[end]) {
		return l
	}
	return l[:start] + prevTag + rest[j:]
}

// isTagByte reports whether c can continue a version tag.
func isTagByte(c byte) bool {
	return c == '.' || c == '-' || c == '_' || c == '+' ||
		('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// collapseBlankLines joins lines, folding each run of blank lines
// into one and dropping leading and trailing blank lines.
func collapseBlankLines(lines []string) string {
	kept := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) == "" && (len(kept) == 0 || kept[len(kept)-1] == "") {
			continue
		}
		if strings.TrimSpace(l) == "" {
			l = ""
		}
		kept = append(kept, l)
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	return strings.Join(kept, "\n")
}

// shortcodeDefuser puts a zero-width space between the braces of
// every Hugo shortcode opener.
var shortcodeDefuser = strings.NewReplacer("{{<", "{\u200b{<", "{{%", "{\u200b{%")

// defuseShortcodes keeps Hugo from reading shortcodes in a release
// body. The page renders each body through RenderString, which
// expands shortcodes, so a PR title quoting "{{< x >}}" would fail
// the site build on an unknown name. The content-file escape
// "{{</* x */>}}" (escapeHugoShortcodes) does not help: RenderString
// prints it verbatim, comment markers and all. The zero-width space
// splits the "{{<" and "{{%" delimiters Hugo's lexer matches, and
// the text still reads "{{< x >}}" in prose and in code alike.
func defuseShortcodes(body string) string {
	return shortcodeDefuser.Replace(body)
}

// candidatesAfterStable keeps the candidates that are newer than
// the stable releases. One candidate is cut per merge, and the stable
// release that follows lists all their changes, so keeping the
// candidates a stable release has already shipped would grow the
// page by one entry per merge, forever.
//
// A candidate with a v-prefixed semver tag is kept when its
// major.minor.patch is above the highest plain vX.Y.Z stable tag, as
// rc-version decides: a backport (v0.55.2) published after
// v0.56.0-rc.3 then hides nothing of the next line. Any other
// candidate, or every candidate when no stable tag is plain semver,
// is kept when it was published after the newest stable release.
// With no stable release, every candidate is kept.
func candidatesAfterStable(cands, stable []SiteRelease) []SiteRelease {
	if len(stable) == 0 {
		return cands
	}
	tags := make([]string, 0, len(stable))
	var newest time.Time
	for _, r := range stable {
		tags = append(tags, r.Tag)
		if r.Published.After(newest) {
			newest = r.Published
		}
	}
	highest, haveHighest := latestStableBelow(tags, nil)
	kept := make([]SiteRelease, 0, len(cands))
	for _, c := range cands {
		if m := versionCoreRE.FindStringSubmatch(c.Tag); haveHighest && m != nil {
			if highest.less(coreFromMatch(m)) {
				kept = append(kept, c)
			}
			continue
		}
		if c.Published.After(newest) {
			kept = append(kept, c)
		}
	}
	return kept
}

func sortNewestFirst(rs []SiteRelease) {
	sort.SliceStable(rs, func(i, j int) bool {
		if !rs[i].Published.Equal(rs[j].Published) {
			return rs[i].Published.After(rs[j].Published)
		}
		return rs[i].Tag > rs[j].Tag
	})
}

// rewriteHeadings adds shift levels to every ATX heading in body,
// capped at h6, and rewrites each setext heading (a paragraph
// underlined with "===" or "---") as an ATX heading at the shifted
// level, its lines joined with a space. When idPrefix is set, each
// heading also gets an explicit "{#<idPrefix>-<slug>}" attribute:
// every release's notes render separately and share headings such
// as "What's Changed", so automatic IDs would repeat across the
// page. A heading's own "{#id}" is scoped the same way, so no id in
// a body can collide with another release or with the page's own
// ids. ATX headings inside block quotes and list items ("> ## A",
// "- ## A") are rewritten too. Lines inside fenced code blocks,
// lines indented four or more spaces, and "#123"-style text are
// left alone. A fence opened inside a block quote or list item
// closes at that container's content indent, or ends with the
// container.
func rewriteHeadings(body string, shift int, idPrefix string) string {
	w := headingRewriter{
		lines:     strings.Split(body, "\n"),
		shift:     shift,
		ids:       headingIDs{prefix: idPrefix, seen: map[string]bool{}},
		paraStart: -1,
		canStart:  true,
	}
	for i := range w.lines {
		w.visit(i)
	}
	return strings.Join(w.lines, "\n")
}

// headingRewriter is rewriteHeadings' line-by-line state.
type headingRewriter struct {
	lines []string
	shift int
	ids   headingIDs
	// fence is the open fenced code block's opener, read by
	// internal/mdfence; its Char is 0 outside one. fenceIn lists the
	// block quotes and list items the fence was opened in,
	// outermost first; it is empty for a top-level fence.
	fence   mdfence.Fence
	fenceIn []fenceContainer
	// paraStart is the first line of the open plain paragraph (one a
	// setext underline can turn into a heading), or -1. canStart
	// reports whether the next plain line opens a new paragraph rather
	// than lazily continuing a list item or block quote.
	paraStart int
	canStart  bool
}

func (w *headingRewriter) visit(i int) {
	line := w.lines[i]
	if w.fence.Char != 0 {
		if rest, inside := stripContainers(line, w.fenceIn); inside {
			if mdfence.Close([]byte(rest), w.fence) {
				w.fence, w.fenceIn, w.canStart = mdfence.Fence{}, nil, true
			}
			return
		}
		// The line ends a block quote or list item the fence was
		// opened in, and the fence with it: fenced code has no lazy
		// continuation. The line itself is an ordinary one.
		w.fence, w.fenceIn, w.canStart = mdfence.Fence{}, nil, true
	}
	trimmed := strings.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)
	switch {
	case strings.TrimSpace(line) == "":
		w.paraStart, w.canStart = -1, true
	case indent > 3:
		// Indented code, or a continuation of the open paragraph,
		// list item, or block quote. Neither opens a paragraph nor
		// changes whether the next plain line may open one.
	default:
		w.block(i, line[:indent], trimmed)
	}
}

// fenceContainer is a block quote or list item a fenced code block
// was opened in. width is a list item's content indent: the spaces a
// line needs to stay inside the item.
type fenceContainer struct {
	quote bool
	width int
}

// fenceContainers lists the containers markers opens, outermost
// first; markers is the run containerPrefix measured. lead is the
// indent before the first marker and pad the spaces between the last
// marker and the fence; each widens the list item it borders.
func fenceContainers(lead int, markers string, pad int) []fenceContainer {
	var cs []fenceContainer
	for p := 0; p < len(markers); lead = 0 {
		if markers[p] == '>' {
			p++
			if p < len(markers) && (markers[p] == ' ' || markers[p] == '\t') {
				p++
			}
			cs = append(cs, fenceContainer{quote: true})
			continue
		}
		n := listMarkerLen(markers[p:])
		cs = append(cs, fenceContainer{width: lead + n})
		p += n
	}
	if last := &cs[len(cs)-1]; !last.quote {
		last.width += pad
	}
	return cs
}

// stripContainers removes the block-quote markers and list-item
// indents of cs from line, outermost first, and reports whether line
// stays inside every one. A blank line stays inside a list item but
// ends a block quote, as in CommonMark.
func stripContainers(line string, cs []fenceContainer) (string, bool) {
	for _, c := range cs {
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		switch {
		case c.quote:
			if indent > 3 || !strings.HasPrefix(trimmed, ">") {
				return "", false
			}
			line = trimmed[1:]
			if line != "" && (line[0] == ' ' || line[0] == '\t') {
				line = line[1:]
			}
		case strings.TrimSpace(line) == "":
			line = ""
		case indent < c.width:
			return "", false
		default:
			line = line[c.width:]
		}
	}
	return line, true
}

// block handles a non-blank line indented at most three spaces;
// indent is its leading spaces and trimmed the rest.
func (w *headingRewriter) block(i int, indent, trimmed string) {
	if f, ok := mdfence.Open([]byte(trimmed)); ok {
		w.fence, w.paraStart = f, -1
		return
	}
	if level := atxLevel(trimmed); level > 0 {
		w.lines[i] = indent + w.ids.atx(w.hashes(level), trimmed[level:])
		w.paraStart, w.canStart = -1, true
		return
	}
	if level := setextLevel(trimmed); level > 0 {
		if w.paraStart >= 0 {
			rewriteSetext(w.lines, w.paraStart, i, w.hashes(level), &w.ids)
		}
		w.paraStart, w.canStart = -1, true
		return
	}
	if isThematicBreak(trimmed) {
		w.paraStart, w.canStart = -1, true
		return
	}
	if opensNonParagraphBlock(trimmed) {
		w.containedATX(i, indent, trimmed)
		w.paraStart, w.canStart = -1, false
		return
	}
	if w.paraStart < 0 && w.canStart {
		w.paraStart = i
	}
	w.canStart = false
}

// containedATX rewrites an ATX heading that sits behind block-quote
// or list-item markers ("> ## A", "1. # A") on line i, keeping the
// markers, and enters fence mode for a fence opened behind them.
// Other lines are left alone.
func (w *headingRewriter) containedATX(i int, indent, trimmed string) {
	p := containerPrefix(trimmed)
	rest := trimmed[p:]
	inner := strings.TrimLeft(rest, " ")
	pad := len(rest) - len(inner)
	if p == 0 || pad > 3 {
		return
	}
	if f, ok := mdfence.Open([]byte(inner)); ok {
		// A fence opened behind the markers ("- ```sh"): its lines
		// are code, not headings, until the matching closer at the
		// containers' content indent, or until a container ends.
		w.fence = f
		w.fenceIn = fenceContainers(len(indent), trimmed[:p], pad)
		return
	}
	if level := atxLevel(inner); level > 0 {
		w.lines[i] = indent + trimmed[:p+pad] + w.ids.atx(w.hashes(level), inner[level:])
	}
}

// containerPrefix returns the length of the block-quote markers
// (">" plus one optional space or tab) and list-item markers ("-",
// "*", "+", or up to nine digits and "." or ")", each followed by a
// space or tab) that open s.
func containerPrefix(s string) int {
	p := 0
	for p < len(s) {
		if s[p] == '>' {
			p++
			if p < len(s) && (s[p] == ' ' || s[p] == '\t') {
				p++
			}
			continue
		}
		n := listMarkerLen(s[p:])
		if n == 0 {
			break
		}
		p += n
	}
	return p
}

// listMarkerLen returns the length of the list-item marker and the
// one space or tab after it that open s, or 0.
func listMarkerLen(s string) int {
	n := 0
	switch {
	case s != "" && (s[0] == '-' || s[0] == '*' || s[0] == '+'):
		n = 1
	default:
		for n < len(s) && n < 10 && s[n] >= '0' && s[n] <= '9' {
			n++
		}
		if n == 0 || n > 9 || n >= len(s) || (s[n] != '.' && s[n] != ')') {
			return 0
		}
		n++
	}
	if n < len(s) && (s[n] == ' ' || s[n] == '\t') {
		return n + 1
	}
	return 0
}

// hashes returns the ATX marker for a heading of level after the
// shift, capped at h6.
func (w *headingRewriter) hashes(level int) string {
	return strings.Repeat("#", min(level+w.shift, 6))
}

// rewriteSetext replaces the setext heading whose paragraph spans
// lines[start:underline] and whose underline is lines[underline] with
// one ATX heading line, blanking the lines it absorbs so the line
// count stays the same. The heading keeps its first line's indent,
// so one inside a list item stays inside it.
func rewriteSetext(lines []string, start, underline int, hashes string, ids *headingIDs) {
	parts := make([]string, 0, underline-start)
	for _, l := range lines[start:underline] {
		parts = append(parts, strings.TrimSpace(l))
	}
	text, _ := ids.scoped(strings.Join(parts, " "))
	first := lines[start]
	indent := first[:len(first)-len(strings.TrimLeft(first, " "))]
	lines[start] = indent + hashes + " " + text
	for j := start + 1; j <= underline; j++ {
		lines[j] = ""
	}
}

// headingIDs hands out tag-scoped heading ids, unique within one
// release body.
type headingIDs struct {
	prefix string
	seen   map[string]bool
}

// atx renders an ATX heading from its new hashes and rest, the
// original line after its opening hashes. When scoped gives the
// heading an id, the text is rebuilt around it; otherwise rest is
// kept verbatim.
func (h *headingIDs) atx(hashes, rest string) string {
	text, ok := h.scoped(atxHeadingText(rest))
	if !ok {
		return hashes + rest
	}
	return hashes + " " + text
}

// scoped returns heading text carrying a tag-scoped id, and whether
// it assigned one. Plain text gains "{#<prefix>-<slug>}". A trailing
// attribute block keeps its classes and key-value pairs: its own
// "#id" becomes "#<prefix>-<id slug>" (the text's slug when the id
// has none), and a block with no id gains the text's scoped id
// first. With no prefix, or nothing to slug, text comes back as is.
func (h *headingIDs) scoped(text string) (string, bool) {
	if h.prefix == "" {
		return text, false
	}
	if !hasAttributeBlock(text) {
		id := h.next(text)
		if id == "" {
			return text, false
		}
		return text + " {#" + id + "}", true
	}
	i := strings.LastIndexByte(text, '{')
	label := strings.TrimSpace(text[:i])
	attrs := strings.Fields(text[i+1 : len(text)-1])
	idAt := -1
	slug := headingSlug(label)
	for k, a := range attrs {
		if strings.HasPrefix(a, "#") {
			idAt = k
			if own := headingSlug(a[1:]); own != "" {
				slug = own
			}
			break
		}
	}
	if slug == "" {
		if label == "" {
			return text, false
		}
		slug = fallbackHeadingSlug
	}
	id := "#" + h.claim(h.prefix+"-"+slug)
	if idAt >= 0 {
		attrs[idAt] = id
	} else {
		attrs = append([]string{id}, attrs...)
	}
	return label + " {" + strings.Join(attrs, " ") + "}", true
}

// fallbackHeadingSlug names a heading whose text has no letter or
// digit ("## 🎉"), so it still gets a scoped id instead of an
// automatic one that repeats across releases.
const fallbackHeadingSlug = "heading"

// next returns the scoped id for plain heading text, or "" when
// there is no prefix or the text is empty. Text without a letter or
// digit slugs to fallbackHeadingSlug.
func (h *headingIDs) next(text string) string {
	if h.prefix == "" || strings.TrimSpace(text) == "" {
		return ""
	}
	slug := headingSlug(text)
	if slug == "" {
		slug = fallbackHeadingSlug
	}
	return h.claim(h.prefix + "-" + slug)
}

// claim records and returns base, or base with the first "-<n>"
// suffix no earlier heading in this body has taken.
func (h *headingIDs) claim(base string) string {
	id := base
	for n := 1; h.seen[id]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	h.seen[id] = true
	return id
}

// hasAttributeBlock reports whether heading text ends in a Goldmark
// attribute block: braces holding only "#id", ".class", and
// "key=value" tokens. Plain braces such as "Fix {x}" or "{a == b}"
// are heading text, not attributes.
func hasAttributeBlock(text string) bool {
	if !strings.HasSuffix(text, "}") {
		return false
	}
	i := strings.LastIndexByte(text, '{')
	if i < 0 {
		return false
	}
	tokens := strings.Fields(text[i+1 : len(text)-1])
	if len(tokens) == 0 {
		return false
	}
	for _, tok := range tokens {
		if !isAttributeToken(tok) {
			return false
		}
	}
	return true
}

// isAttributeToken reports whether tok is one well-formed attribute:
// "#id", ".class", or "key=value" with a name-like key and a
// non-empty value. Brace text such as "{a == b}" is heading text.
func isAttributeToken(tok string) bool {
	switch {
	case strings.ContainsAny(tok, " \t{}"):
		return false
	case len(tok) > 1 && (tok[0] == '#' || tok[0] == '.'):
		return true
	case strings.Count(tok, "=") == 0:
		return false
	}
	key, value, _ := strings.Cut(tok, "=")
	if key == "" || value == "" {
		return false
	}
	for i, r := range key {
		alpha := r == '_' || r == ':' || unicode.IsLetter(r)
		if !alpha && (i == 0 || (r != '-' && r != '.' && !unicode.IsDigit(r))) {
			return false
		}
	}
	return true
}

// setextLevel returns 1 for a setext h1 underline ("==="), 2 for an
// h2 underline ("---"), or 0, on a line with its indent removed.
func setextLevel(s string) int {
	s = strings.TrimRight(s, " \t")
	switch {
	case s == "":
		return 0
	case strings.Trim(s, "=") == "":
		return 1
	case strings.Trim(s, "-") == "":
		return 2
	}
	return 0
}

// isThematicBreak reports whether s (indent removed, not blank) is a
// thematic break: three or more of one of "*", "-", or "_", with only
// spaces or tabs between them.
func isThematicBreak(s string) bool {
	c := s[0]
	if c != '*' && c != '-' && c != '_' {
		return false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case c:
			n++
		case ' ', '\t':
		default:
			return false
		}
	}
	return n >= 3
}

// opensNonParagraphBlock reports whether s (indent removed) opens a
// list item, block quote, HTML block, or table row: a block a setext
// underline cannot turn into a heading.
func opensNonParagraphBlock(s string) bool {
	switch s[0] {
	case '>', '<', '|':
		return true
	case '-', '*', '+':
		return len(s) == 1 || s[1] == ' ' || s[1] == '\t'
	}
	n := 0
	for n < len(s) && n < 9 && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	if n == 0 || n >= len(s) || (s[n] != '.' && s[n] != ')') {
		return false
	}
	return n+1 == len(s) || s[n+1] == ' ' || s[n+1] == '\t'
}

// atxHeadingText returns an ATX heading's inline text: rest is the line
// after its opening hashes, and the optional closing hash sequence
// is dropped.
func atxHeadingText(rest string) string {
	text := strings.TrimSpace(rest)
	trimmed := strings.TrimRight(text, "#")
	if trimmed == "" || strings.HasSuffix(trimmed, " ") || strings.HasSuffix(trimmed, "\t") {
		text = strings.TrimSpace(trimmed)
	}
	return text
}

// headingSlug lowercases s, keeps letters and digits, turns runs of
// spaces, tabs, hyphens, underscores, and dots into one hyphen, and
// drops every other character, so "What's Changed" becomes
// "whats-changed" and "v0.56.0-rc.2" becomes "v0-56-0-rc-2".
func headingSlug(s string) string {
	var b strings.Builder
	sep := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if sep && b.Len() > 0 {
				b.WriteByte('-')
			}
			sep = false
			b.WriteRune(r)
		case r == ' ' || r == '\t' || r == '-' || r == '_' || r == '.':
			sep = true
		}
	}
	return b.String()
}

// atxLevel returns the heading level of an ATX heading line with its
// leading indent removed, or 0 when s is not a heading.
func atxLevel(s string) int {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return 0
	}
	if n < len(s) && s[n] != ' ' && s[n] != '\t' {
		return 0
	}
	return n
}

// ListReleases returns every release in the repository, following
// the list-releases endpoint's Link-header pagination.
func ListReleases(opts GitHubRepoOptions) ([]GitHubRelease, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	client := opts.client()
	var all []GitHubRelease
	next := opts.apiBase() + "/repos/" + opts.Repository + "/releases?per_page=100"
	for next != "" {
		page, link, err := listReleasesPage(client, next, opts.Token)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		next = link
	}
	return all, nil
}

func listReleasesPage(client *http.Client, u, token string) ([]GitHubRelease, string, error) {
	req, err := newGitHubRequest(http.MethodGet, u, nil, token)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", unexpectedStatus("list releases", u, resp)
	}
	var page []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", u, err)
	}
	return page, nextPageURL(resp.Header.Get("Link")), nil
}

// SyncReleases lists the repository's releases and writes them to
// outPath as the website's releases data file.
func SyncReleases(opts GitHubRepoOptions, outPath string) error {
	rels, err := ListReleases(opts)
	if err != nil {
		return err
	}
	// SiteReleases holds only strings and times, so marshaling
	// cannot fail.
	data, _ := json.MarshalIndent(BuildSiteReleases(rels), "", "  ")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, append(data, '\n'), 0o644)
}
