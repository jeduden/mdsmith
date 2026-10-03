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
// tie, so the output is stable).
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
		body := strings.TrimSpace(strings.ReplaceAll(r.Body, "\r\n", "\n"))
		sr := SiteRelease{
			Tag:       r.TagName,
			Name:      name,
			URL:       r.HTMLURL,
			Published: r.PublishedAt,
			Body:      rewriteHeadings(body, releaseHeadingShift, headingSlug(r.TagName)),
		}
		if r.Prerelease {
			out.Candidates = append(out.Candidates, sr)
		} else {
			out.Stable = append(out.Stable, sr)
		}
	}
	sortNewestFirst(out.Stable)
	sortNewestFirst(out.Candidates)
	if len(out.Stable) > 0 {
		out.Candidates = candidatesAfter(out.Candidates, out.Stable[0].Published)
	}
	return out
}

// candidatesAfter keeps the candidates published after cut, the
// latest stable release. Each candidate's notes span every change
// since the previous stable release, and one is cut per merge, so
// keeping the candidates a stable release has already shipped would
// grow the page by a whole changelog per merge, forever.
func candidatesAfter(rs []SiteRelease, cut time.Time) []SiteRelease {
	kept := rs[:0]
	for _, r := range rs {
		if r.Published.After(cut) {
			kept = append(kept, r)
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
// left alone.
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
	// fenceChar and fenceLen describe the open fenced code block's
	// opening run; fenceChar is 0 outside one.
	fenceChar byte
	fenceLen  int
	// paraStart is the first line of the open plain paragraph (one a
	// setext underline can turn into a heading), or -1. canStart
	// reports whether the next plain line opens a new paragraph rather
	// than lazily continuing a list item or block quote.
	paraStart int
	canStart  bool
}

func (w *headingRewriter) visit(i int) {
	line := w.lines[i]
	if w.fenceChar != 0 {
		if w.closesFence(line) {
			w.fenceChar, w.canStart = 0, true
		}
		return
	}
	trimmed := strings.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)
	switch {
	case strings.TrimSpace(line) == "":
		w.paraStart, w.canStart = -1, true
	case indent > 3:
		if w.paraStart < 0 {
			w.canStart = true
		}
	default:
		w.block(i, line[:indent], trimmed)
	}
}

// closesFence reports whether line closes the open fenced code
// block: the same fence character, a run at least as long as the
// opener's, and nothing but whitespace after it. A fence opened
// inside a block quote closes behind the same "> " markers.
func (w *headingRewriter) closesFence(line string) bool {
	if w.isCloser(line) {
		return true
	}
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || !strings.HasPrefix(trimmed, ">") {
		return false
	}
	rest := strings.TrimLeft(trimmed, "> \t")
	return w.isCloser(rest)
}

func (w *headingRewriter) isCloser(line string) bool {
	c, n := fenceMarker([]byte(line))
	return c == w.fenceChar && n >= w.fenceLen && fenceLineEmptyAfter([]byte(line), n)
}

// block handles a non-blank line indented at most three spaces;
// indent is its leading spaces and trimmed the rest.
func (w *headingRewriter) block(i int, indent, trimmed string) {
	if c, n := fenceMarker([]byte(trimmed)); c != 0 {
		w.fenceChar, w.fenceLen, w.paraStart = c, n, -1
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
	if c, n := fenceMarker([]byte(inner)); c != 0 {
		// A fence opened behind the markers ("- ```sh"): its lines
		// are code, not headings, until the matching closer.
		w.fenceChar, w.fenceLen = c, n
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
// count stays the same.
func rewriteSetext(lines []string, start, underline int, hashes string, ids *headingIDs) {
	parts := make([]string, 0, underline-start)
	for _, l := range lines[start:underline] {
		parts = append(parts, strings.TrimSpace(l))
	}
	text, _ := ids.scoped(strings.Join(parts, " "))
	lines[start] = hashes + " " + text
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
		return text, false
	}
	id := "#" + h.claim(h.prefix+"-"+slug)
	if idAt >= 0 {
		attrs[idAt] = id
	} else {
		attrs = append([]string{id}, attrs...)
	}
	return label + " {" + strings.Join(attrs, " ") + "}", true
}

// next returns the scoped id for plain heading text, or "" when
// there is no prefix or the text has no slug.
func (h *headingIDs) next(text string) string {
	slug := headingSlug(text)
	if h.prefix == "" || slug == "" {
		return ""
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
// attribute block: "{#id}", "{.class}", or "{key=value}". Plain
// braces such as "Fix {x}" are heading text, not attributes.
func hasAttributeBlock(text string) bool {
	if !strings.HasSuffix(text, "}") {
		return false
	}
	i := strings.LastIndexByte(text, '{')
	if i < 0 {
		return false
	}
	inner := strings.TrimSpace(text[i+1 : len(text)-1])
	return strings.HasPrefix(inner, "#") || strings.HasPrefix(inner, ".") || strings.Contains(inner, "=")
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
