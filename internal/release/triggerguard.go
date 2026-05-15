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
	// DefaultReleaseTriggerRetryAttempts is the number of times the
	// create-event trigger guard retries a 404 draft-release lookup
	// before concluding the matching draft does not exist yet.
	DefaultReleaseTriggerRetryAttempts = 3
	// DefaultReleaseTriggerRetryDelay is the pause between 404
	// retries while GitHub finishes materializing the draft release.
	DefaultReleaseTriggerRetryDelay = 2 * time.Second
)

// TriggerGuardOptions describes the workflow event fields
// and HTTP settings the create-event trigger guard needs.
type TriggerGuardOptions struct {
	EventName  string
	Repository string
	RefName    string
	RefType    string
	Token      string

	APIBaseURL    string
	RetryAttempts int
	RetryDelay    time.Duration
	Client        *http.Client
	Sleep         func(time.Duration)
}

// TriggerGuardResult is the decision the workflow writes to
// GITHUB_OUTPUT for downstream jobs.
type TriggerGuardResult struct {
	ShouldRun            bool
	CreateReleaseIsDraft bool
}

// CheckReleaseTrigger decides whether the release workflow should
// continue for the current event. Push events always proceed. A
// create event only proceeds when it created a v* tag whose
// matching GitHub Release already exists and is still draft.
func CheckReleaseTrigger(opts TriggerGuardOptions) (TriggerGuardResult, error) {
	if opts.EventName != "create" {
		return TriggerGuardResult{ShouldRun: true}, nil
	}
	if opts.RefType != "tag" || !strings.HasPrefix(opts.RefName, "v") {
		return TriggerGuardResult{}, nil
	}
	if opts.Repository == "" {
		return TriggerGuardResult{}, errors.New("release trigger guard requires repository")
	}
	if opts.Token == "" {
		return TriggerGuardResult{}, errors.New("release trigger guard requires token")
	}

	attempts := opts.RetryAttempts
	if attempts < 1 {
		attempts = DefaultReleaseTriggerRetryAttempts
	}
	delay := opts.RetryDelay
	if delay <= 0 {
		delay = DefaultReleaseTriggerRetryDelay
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

	for attempt := 1; attempt <= attempts; attempt++ {
		found, draft, err := lookupReleaseDraft(client, apiBase, opts.Repository, opts.RefName, opts.Token)
		if err != nil {
			return TriggerGuardResult{}, err
		}
		if found {
			return TriggerGuardResult{
				ShouldRun:            draft,
				CreateReleaseIsDraft: draft,
			}, nil
		}
		if attempt < attempts {
			sleep(delay)
		}
	}
	return TriggerGuardResult{}, nil
}

type releaseLookupPayload struct {
	Draft bool `json:"draft"`
}

type releaseLookupError struct {
	URL        string
	StatusCode int
	Body       string
}

func (e *releaseLookupError) Error() string {
	msg := fmt.Sprintf("lookup %s: unexpected GitHub API status %d", e.URL, e.StatusCode)
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return msg
	}
	return msg + ": " + body
}

func lookupReleaseDraft(client *http.Client, apiBase, repository, tag, token string) (bool, bool, error) {
	u := apiBase + "/repos/" + repository + "/releases/tags/" + url.PathEscape(tag)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return false, false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return false, false, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		// Stream-decode only the `draft` field straight from the
		// response body. Reading into a fixed buffer first would
		// truncate (and fail to parse) releases whose `body` is
		// larger than the cap, even though we never need it.
		var payload releaseLookupPayload
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return false, false, fmt.Errorf("parse %s: %w", u, err)
		}
		return true, payload.Draft, nil
	case http.StatusNotFound:
		return false, false, nil
	default:
		// Bound the error body: it is only recorded for diagnostics.
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if err != nil {
			return false, false, err
		}
		return false, false, &releaseLookupError{
			URL:        u,
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}
}
