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
	return out
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
// capped at h6. When idPrefix is set, each heading also gets an
// explicit "{#<idPrefix>-<slug>}" attribute: every release's notes
// render separately and share headings such as "What's Changed",
// so automatic IDs would repeat across the page. A heading that
// already carries an attribute block keeps it. Lines inside fenced
// code blocks, lines indented four or more spaces, and "#123"-style
// text are left alone.
func rewriteHeadings(body string, shift int, idPrefix string) string {
	lines := strings.Split(body, "\n")
	seen := map[string]int{}
	var fence string
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if indent > 3 {
			continue
		}
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, fence[:1]) == "" {
				fence = ""
			}
			continue
		}
		if f := fenceOpener(trimmed); f != "" {
			fence = f
			continue
		}
		level := atxLevel(trimmed)
		if level == 0 {
			continue
		}
		hashes := strings.Repeat("#", min(level+shift, 6))
		rest := trimmed[level:]
		lines[i] = line[:indent] + hashes + rest
		text := atxHeadingText(rest)
		slug := headingSlug(text)
		if idPrefix == "" || slug == "" || strings.HasSuffix(text, "}") {
			continue
		}
		id := idPrefix + "-" + slug
		if n := seen[id]; n > 0 {
			seen[id] = n + 1
			id = fmt.Sprintf("%s-%d", id, n)
		} else {
			seen[id] = 1
		}
		lines[i] = line[:indent] + hashes + " " + text + " {#" + id + "}"
	}
	return strings.Join(lines, "\n")
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

// fenceOpener returns the run of three or more backticks or tildes
// that opens a fenced code block on s, or "" when s opens none.
func fenceOpener(s string) string {
	if len(s) < 3 || (s[0] != '`' && s[0] != '~') {
		return ""
	}
	n := 0
	for n < len(s) && s[n] == s[0] {
		n++
	}
	if n < 3 {
		return ""
	}
	return s[:n]
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
