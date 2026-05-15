package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPublishReleaseFlipsDraftToPublished(t *testing.T) {
	var (
		mu      sync.Mutex
		gets    int
		patched bool
		patchID string
		gotBody map[string]bool
		authHdr []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		authHdr = append(authHdr, r.Header.Get("Authorization"))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/jeduden/mdsmith/releases/tags/v1.2.3":
			gets++
			if gets == 1 {
				http.NotFound(w, r)
				return
			}
			_, _ = fmt.Fprint(w, `{"id":42,"draft":true}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/jeduden/mdsmith/releases/42":
			patched = true
			patchID = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = fmt.Fprint(w, `{"id":42,"draft":false}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)

	var sleeps int
	err := PublishRelease(PublishReleaseOptions{
		Repository:    "jeduden/mdsmith",
		Tag:           "v1.2.3",
		Token:         "test-token",
		APIBaseURL:    srv.URL,
		RetryAttempts: 3,
		RetryDelay:    time.Millisecond,
		Sleep:         func(time.Duration) { sleeps++ },
	})
	require.NoError(t, err)
	assert.Equal(t, 2, gets)
	assert.Equal(t, 1, sleeps)
	assert.True(t, patched)
	assert.Equal(t, "/repos/jeduden/mdsmith/releases/42", patchID)
	assert.Equal(t, map[string]bool{"draft": false}, gotBody)
	for _, h := range authHdr {
		assert.Equal(t, "Bearer test-token", h)
	}
}

func TestPublishReleaseAlreadyPublishedIsNoOp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("already-published release must not be patched, got %s", r.Method)
		}
		_, _ = fmt.Fprint(w, `{"id":7,"draft":false}`)
	}))
	t.Cleanup(srv.Close)

	err := PublishRelease(PublishReleaseOptions{
		Repository: "jeduden/mdsmith",
		Tag:        "v1.2.3",
		Token:      "test-token",
		APIBaseURL: srv.URL,
	})
	require.NoError(t, err)
}

func TestPublishReleaseMissingAfterRetriesErrors(t *testing.T) {
	var calls, sleeps int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	err := PublishRelease(PublishReleaseOptions{
		Repository:    "jeduden/mdsmith",
		Tag:           "v9.9.9",
		Token:         "t",
		APIBaseURL:    srv.URL,
		RetryAttempts: 2,
		RetryDelay:    time.Millisecond,
		Sleep:         func(time.Duration) { sleeps++ },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no GitHub release found for tag v9.9.9")
	assert.Equal(t, 2, calls)
	assert.Equal(t, 1, sleeps)
}

func TestPublishReleaseRequiresRepositoryTagToken(t *testing.T) {
	err := PublishRelease(PublishReleaseOptions{Tag: "v1", Token: "t"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository")

	err = PublishRelease(PublishReleaseOptions{Repository: "r", Token: "t"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag")

	err = PublishRelease(PublishReleaseOptions{Repository: "r", Tag: "v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token")
}

func TestPublishReleaseLookupUnexpectedStatusErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"boom"}`)
	}))
	t.Cleanup(srv.Close)

	err := PublishRelease(PublishReleaseOptions{
		Repository: "jeduden/mdsmith",
		Tag:        "v1.2.3",
		Token:      "t",
		APIBaseURL: srv.URL,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected GitHub API status 500")
}

func TestPublishReleasePatchUnexpectedStatusErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = fmt.Fprint(w, `{"id":3,"draft":true}`)
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = fmt.Fprint(w, `{"message":"immutable"}`)
	}))
	t.Cleanup(srv.Close)

	err := PublishRelease(PublishReleaseOptions{
		Repository: "jeduden/mdsmith",
		Tag:        "v1.2.3",
		Token:      "t",
		APIBaseURL: srv.URL,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected GitHub API status 422")
	// A PATCH (publish) failure must not be mislabeled "lookup".
	assert.Contains(t, err.Error(), "publish ")
	assert.NotContains(t, err.Error(), "lookup ")
}

func TestLookupReleaseRefRequestBuildError(t *testing.T) {
	// A control character in the URL makes http.NewRequest fail,
	// exercising newGitHubRequest's error path via the GET caller.
	_, _, err := lookupReleaseRef(&http.Client{}, "http://\x7f", "jeduden/mdsmith", "v1", "t")
	require.Error(t, err)
}

func TestPatchReleaseDraftFalseRequestBuildError(t *testing.T) {
	err := patchReleaseDraftFalse(&http.Client{}, "http://\x7f", "jeduden/mdsmith", 42, "t")
	require.Error(t, err)
}

func TestReleaseLookupErrorMessage(t *testing.T) {
	// Empty body returns just the status line (the `body == ""`
	// branch); a non-empty body appends it.
	bare := &releaseLookupError{Op: "publish", URL: "u", StatusCode: 422}
	assert.Equal(t, "publish u: unexpected GitHub API status 422", bare.Error())

	withBody := &releaseLookupError{Op: "lookup", URL: "u", StatusCode: 500, Body: "  boom  "}
	assert.Equal(t, "lookup u: unexpected GitHub API status 500: boom", withBody.Error())
}

func TestPublishReleaseUsesDefaultAPIBase(t *testing.T) {
	var gotURL string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id":1,"draft":false}`)),
			Header:     make(http.Header),
		}, nil
	})}
	err := PublishRelease(PublishReleaseOptions{
		Repository: "jeduden/mdsmith",
		Tag:        "v1.2.3",
		Token:      "t",
		Client:     client,
	})
	require.NoError(t, err)
	assert.Equal(t, "https://api.github.com/repos/jeduden/mdsmith/releases/tags/v1.2.3", gotURL)
}

func TestPublishReleaseClientDoError(t *testing.T) {
	sentinel := errors.New("transport boom")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, sentinel
	})}
	err := PublishRelease(PublishReleaseOptions{
		Repository: "jeduden/mdsmith",
		Tag:        "v1.2.3",
		Token:      "t",
		APIBaseURL: "https://api.example.com",
		Client:     client,
	})
	require.ErrorIs(t, err, sentinel)
}

func TestPublishReleasePatchTransportError(t *testing.T) {
	sentinel := errors.New("patch boom")
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPatch {
			return nil, sentinel
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id":5,"draft":true}`)),
			Header:     make(http.Header),
		}, nil
	})}
	err := PublishRelease(PublishReleaseOptions{
		Repository: "jeduden/mdsmith",
		Tag:        "v1.2.3",
		Token:      "t",
		APIBaseURL: "https://api.example.com",
		Client:     client,
	})
	require.ErrorIs(t, err, sentinel)
}

type errBody struct{}

func (errBody) Read([]byte) (int, error) { return 0, errors.New("read boom") }
func (errBody) Close() error             { return nil }

func TestPublishReleaseUnexpectedStatusBodyReadError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       errBody{},
			Header:     make(http.Header),
		}, nil
	})}
	err := PublishRelease(PublishReleaseOptions{
		Repository: "jeduden/mdsmith",
		Tag:        "v1.2.3",
		Token:      "t",
		APIBaseURL: "https://api.example.com",
		Client:     client,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read boom")
}

func TestPublishReleaseInvalidJSONErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{not json`)
	}))
	t.Cleanup(srv.Close)

	err := PublishRelease(PublishReleaseOptions{
		Repository: "jeduden/mdsmith",
		Tag:        "v1.2.3",
		Token:      "t",
		APIBaseURL: srv.URL,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse ")
}
