package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// stableTagRE matches a plain release tag (v1.2.3). Tags with any
	// suffix — release candidates, the one-off v0.49.0-marketplace —
	// are not stable releases.
	stableTagRE = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)
	// rcTagRE matches a release-candidate tag (v1.2.3-rc.4).
	rcTagRE = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-rc\.([1-9]\d*)$`)
	// versionCoreRE pulls the major.minor.patch core out of any
	// v-prefixed semver, ignoring a pre-release or build suffix.
	versionCoreRE = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:[-+].*)?$`)
)

// versionCore is the numeric major.minor.patch triple of a tag.
type versionCore [3]int

func (a versionCore) less(b versionCore) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func (a versionCore) String() string {
	return fmt.Sprintf("v%d.%d.%d", a[0], a[1], a[2])
}

// coreFromMatch converts the first three submatches of one of the
// tag regexps into a versionCore. The regexps only admit digits, so
// Atoi cannot fail short of an int overflow, which a tag never hits.
func coreFromMatch(m []string) versionCore {
	var c versionCore
	for i := range c {
		c[i], _ = strconv.Atoi(m[i+1])
	}
	return c
}

// latestStableBelow returns the highest stable tag whose core is
// strictly below limit. A nil limit means no upper bound.
func latestStableBelow(tags []string, limit *versionCore) (versionCore, bool) {
	var (
		best  versionCore
		found bool
	)
	for _, tag := range tags {
		m := stableTagRE.FindStringSubmatch(tag)
		if m == nil {
			continue
		}
		c := coreFromMatch(m)
		if limit != nil && !c.less(*limit) {
			continue
		}
		if !found || best.less(c) {
			best, found = c, true
		}
	}
	return best, found
}

// LatestStableTag returns the highest plain vX.Y.Z tag in tags,
// comparing numerically. Release candidates and other suffixed tags
// are skipped.
func LatestStableTag(tags []string) (string, bool) {
	c, ok := latestStableBelow(tags, nil)
	if !ok {
		return "", false
	}
	return c.String(), true
}

// NextRCVersion returns the release-candidate version for the next
// merge to main: the minor after the latest stable tag, with an
// -rc.N suffix one above the highest existing candidate for that
// minor. With no stable tag yet the series starts at v0.1.0.
func NextRCVersion(tags []string) string {
	base := versionCore{0, 1, 0}
	if latest, ok := latestStableBelow(tags, nil); ok {
		base = versionCore{latest[0], latest[1] + 1, 0}
	}
	n := 0
	for _, tag := range tags {
		m := rcTagRE.FindStringSubmatch(tag)
		if m == nil || coreFromMatch(m) != base {
			continue
		}
		if rc, _ := strconv.Atoi(m[4]); rc > n {
			n = rc
		}
	}
	return fmt.Sprintf("%s-rc.%d", base, n+1)
}

// PreviousStableTag returns the highest stable tag whose core is
// below version's core. Both a stable release and its candidates
// get the same answer, so release notes always span from the last
// stable release rather than from the most recent candidate.
func PreviousStableTag(tags []string, version string) (string, bool, error) {
	m := versionCoreRE.FindStringSubmatch(version)
	if m == nil {
		return "", false, fmt.Errorf("version %q is not a v-prefixed semver", version)
	}
	limit := coreFromMatch(m)
	c, ok := latestStableBelow(tags, &limit)
	if !ok {
		return "", false, nil
	}
	return c.String(), true, nil
}

// GitHubRepoOptions addresses one repository on the GitHub REST API.
type GitHubRepoOptions struct {
	Repository string
	Token      string
	APIBaseURL string
	Client     *http.Client
}

func (o GitHubRepoOptions) validate() error {
	if o.Repository == "" {
		return errors.New("repository is required")
	}
	if o.Token == "" {
		return errors.New("token is required")
	}
	return nil
}

func (o GitHubRepoOptions) client() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (o GitHubRepoOptions) apiBase() string {
	if b := strings.TrimRight(o.APIBaseURL, "/"); b != "" {
		return b
	}
	return "https://api.github.com"
}

// ListTags returns every tag name in the repository, following the
// list-tags endpoint's Link-header pagination.
func ListTags(opts GitHubRepoOptions) ([]string, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	client := opts.client()
	var tags []string
	next := opts.apiBase() + "/repos/" + opts.Repository + "/tags?per_page=100"
	for next != "" {
		page, link, err := listTagsPage(client, next, opts.Token)
		if err != nil {
			return nil, err
		}
		tags = append(tags, page...)
		next = link
	}
	return tags, nil
}

func listTagsPage(client *http.Client, u, token string) ([]string, string, error) {
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
		return nil, "", unexpectedStatus("list tags", u, resp)
	}
	var page []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", u, err)
	}
	names := make([]string, 0, len(page))
	for _, t := range page {
		names = append(names, t.Name)
	}
	return names, nextPageURL(resp.Header.Get("Link")), nil
}

// ResolveRCVersion lists the repository's tags and returns the
// version the next release candidate should carry.
func ResolveRCVersion(opts GitHubRepoOptions) (string, error) {
	tags, err := ListTags(opts)
	if err != nil {
		return "", err
	}
	return NextRCVersion(tags), nil
}

// NotesOptions names the release whose notes to generate.
// Target is the commit the tag points at (or will point at, when
// the tag does not exist yet).
type NotesOptions struct {
	GitHubRepoOptions
	Tag    string
	Target string
}

// GenerateReleaseNotes asks GitHub to write the notes for opts.Tag,
// pinning previous_tag_name to the last stable tag. Left unpinned,
// GitHub may start the range at the latest release candidate, so a
// stable release would list only the merges since its last RC.
func GenerateReleaseNotes(opts NotesOptions) (string, error) {
	if opts.Tag == "" {
		return "", errors.New("release-notes requires tag")
	}
	if opts.Target == "" {
		return "", errors.New("release-notes requires target commit")
	}
	if !versionCoreRE.MatchString(opts.Tag) {
		return "", fmt.Errorf("tag %q is not a v-prefixed semver", opts.Tag)
	}
	tags, err := ListTags(opts.GitHubRepoOptions)
	if err != nil {
		return "", err
	}
	// The tag was validated above, so PreviousStableTag cannot fail.
	prev, havePrev, _ := PreviousStableTag(tags, opts.Tag)

	payload := map[string]string{
		"tag_name":         opts.Tag,
		"target_commitish": opts.Target,
	}
	if havePrev {
		payload["previous_tag_name"] = prev
	}
	return postGenerateNotes(opts.GitHubRepoOptions, payload)
}

func postGenerateNotes(opts GitHubRepoOptions, payload map[string]string) (string, error) {
	// Marshaling a map[string]string cannot fail.
	body, _ := json.Marshal(payload)
	u := opts.apiBase() + "/repos/" + opts.Repository + "/releases/generate-notes"
	// ListTags already built a request against the same base URL and
	// repository, so building this one cannot fail.
	req, _ := newGitHubRequest(http.MethodPost, u, bytes.NewReader(body), opts.Token)
	resp, err := opts.client().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", unexpectedStatus("generate notes", u, resp)
	}
	var notes struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&notes); err != nil {
		return "", fmt.Errorf("parse %s: %w", u, err)
	}
	return notes.Body, nil
}
