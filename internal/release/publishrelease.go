package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	apiBase := strings.TrimRight(opts.APIBaseURL, "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}

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
	ID    int64 `json:"id"`
	Draft bool  `json:"draft"`
}

func lookupReleaseRef(client *http.Client, apiBase, repository, tag, token string) (releaseRef, bool, error) {
	u := apiBase + "/repos/" + repository + "/releases/tags/" + url.PathEscape(tag)
	req, err := newGitHubRequest(http.MethodGet, u, nil, token)
	if err != nil {
		return releaseRef{}, false, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return releaseRef{}, false, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		var rel releaseRef
		if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
			return releaseRef{}, false, fmt.Errorf("parse %s: %w", u, err)
		}
		return rel, true, nil
	case http.StatusNotFound:
		return releaseRef{}, false, nil
	default:
		return releaseRef{}, false, unexpectedStatus("lookup", u, resp)
	}
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
