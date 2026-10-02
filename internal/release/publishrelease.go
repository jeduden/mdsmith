package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultPublishReleaseRetryAttempts is how many times
	// PublishRelease retries the by-tag lookup before concluding the
	// draft release the `release` job just created is not visible yet.
	DefaultPublishReleaseRetryAttempts = 3
	// DefaultPublishReleaseRetryDelay is the pause between by-tag
	// lookup retries while GitHub finishes materializing the release.
	DefaultPublishReleaseRetryDelay = 2 * time.Second
)

// PublishReleaseOptions describes the GitHub release to flip from
// draft to published and the HTTP settings to reach the API.
type PublishReleaseOptions struct {
	Repository string
	Tag        string
	Token      string

	APIBaseURL    string
	RetryAttempts int
	RetryDelay    time.Duration
	Client        *http.Client
	Sleep         func(time.Duration)
}

// PublishRelease flips the draft GitHub release for opts.Tag to a
// published release. The release is created by the `release` job's
// softprops/action-gh-release step as a draft so that every asset
// uploads while the release is still mutable; this call publishes it
// as the final, atomic step so the result is an immutable release.
//
// Idempotent: a release that is already published is left untouched.
func PublishRelease(opts PublishReleaseOptions) error {
	if opts.Repository == "" {
		return errors.New("publish-release requires repository")
	}
	if opts.Tag == "" {
		return errors.New("publish-release requires tag")
	}
	if opts.Token == "" {
		return errors.New("publish-release requires token")
	}

	attempts := opts.RetryAttempts
	if attempts < 1 {
		attempts = DefaultPublishReleaseRetryAttempts
	}
	delay := opts.RetryDelay
	if delay <= 0 {
		delay = DefaultPublishReleaseRetryDelay
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	repo := GitHubRepoOptions{APIBaseURL: opts.APIBaseURL, Client: opts.Client}
	client := repo.client()
	apiBase := repo.apiBase()

	var (
		rel   releaseRef
		found bool
	)
	for attempt := 1; attempt <= attempts; attempt++ {
		var err error
		rel, found, err = lookupReleaseRef(client, apiBase, opts.Repository, opts.Tag, opts.Token)
		if err != nil {
			return err
		}
		if found {
			break
		}
		if attempt < attempts {
			sleep(delay)
		}
	}
	if !found {
		return fmt.Errorf("no GitHub release found for tag %s in %s", opts.Tag, opts.Repository)
	}
	if !rel.Draft {
		return nil
	}
	return patchReleaseDraftFalse(client, apiBase, opts.Repository, rel.ID, opts.Token)
}

type releaseRef struct {
	ID      int64  `json:"id"`
	Draft   bool   `json:"draft"`
	TagName string `json:"tag_name"`
}

// lookupReleaseRef finds the release whose tag_name matches tag by
// walking the list-releases endpoint. The by-tag endpoint
// (GET /releases/tags/{tag}) deliberately omits draft releases, and
// the `release` job creates the release as a draft so its assets stay
// mutable until the final publish — so the draft is only reachable
// through the list endpoint. Pagination is followed via the Link
// header because drafts are not guaranteed to land on the first page.
func lookupReleaseRef(client *http.Client, apiBase, repository, tag, token string) (releaseRef, bool, error) {
	next := apiBase + "/repos/" + repository + "/releases?per_page=100"
	for next != "" {
		rel, found, link, err := lookupReleasePage(client, next, tag, token)
		if err != nil {
			return releaseRef{}, false, err
		}
		if found {
			return rel, true, nil
		}
		next = link
	}
	return releaseRef{}, false, nil
}

func lookupReleasePage(client *http.Client, u, tag, token string) (releaseRef, bool, string, error) {
	req, err := newGitHubRequest(http.MethodGet, u, nil, token)
	if err != nil {
		return releaseRef{}, false, "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return releaseRef{}, false, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return releaseRef{}, false, "", unexpectedStatus("lookup", u, resp)
	}

	var rels []releaseRef
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return releaseRef{}, false, "", fmt.Errorf("parse %s: %w", u, err)
	}
	for _, rel := range rels {
		if rel.TagName == tag {
			return rel, true, "", nil
		}
	}
	return releaseRef{}, false, nextPageURL(resp.Header.Get("Link")), nil
}

// nextPageURL extracts the rel="next" target from a GitHub Link
// header, or "" when there is no further page.
func nextPageURL(link string) string {
	for _, part := range strings.Split(link, ",") {
		segs := strings.Split(part, ";")
		if len(segs) < 2 {
			continue
		}
		isNext := false
		for _, attr := range segs[1:] {
			if strings.TrimSpace(attr) == `rel="next"` {
				isNext = true
				break
			}
		}
		if !isNext {
			continue
		}
		u := strings.TrimSpace(segs[0])
		u = strings.TrimPrefix(u, "<")
		u = strings.TrimSuffix(u, ">")
		return u
	}
	return ""
}

func patchReleaseDraftFalse(client *http.Client, apiBase, repository string, id int64, token string) error {
	u := fmt.Sprintf("%s/repos/%s/releases/%d", apiBase, repository, id)
	req, err := newGitHubRequest(http.MethodPatch, u, strings.NewReader(`{"draft":false}`), token)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return unexpectedStatus("publish", u, resp)
	}
	return nil
}

func newGitHubRequest(method, u string, body io.Reader, token string) (*http.Request, error) {
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func unexpectedStatus(op, u string, resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return err
	}
	return &releaseLookupError{
		Op:         op,
		URL:        u,
		StatusCode: resp.StatusCode,
		Body:       string(body),
	}
}

type releaseLookupError struct {
	// Op labels the failing operation in the error message, e.g.
	// "lookup" for the by-tag GET or "publish" for the draft PATCH.
	Op         string
	URL        string
	StatusCode int
	Body       string
}

func (e *releaseLookupError) Error() string {
	msg := fmt.Sprintf("%s %s: unexpected GitHub API status %d", e.Op, e.URL, e.StatusCode)
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return msg
	}
	return msg + ": " + body
}
