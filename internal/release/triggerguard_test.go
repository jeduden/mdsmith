package release

import (
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

func TestCheckReleaseTriggerPushRunsWithoutLookup(t *testing.T) {
	var called bool
	res, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName: "push",
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, assert.AnError
		})},
	})
	require.NoError(t, err)
	assert.Equal(t, TriggerGuardResult{ShouldRun: true}, res)
	assert.False(t, called)
}

func TestCheckReleaseTriggerNonTagCreateSkipsWithoutLookup(t *testing.T) {
	var called bool
	res, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName: "create",
		RefType:   "branch",
		RefName:   "topic",
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, assert.AnError
		})},
	})
	require.NoError(t, err)
	assert.Equal(t, TriggerGuardResult{}, res)
	assert.False(t, called)
}

func TestCheckReleaseTriggerNonVTagCreateSkipsWithoutLookup(t *testing.T) {
	var called bool
	res, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName: "create",
		RefType:   "tag",
		RefName:   "not-a-release",
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, assert.AnError
		})},
	})
	require.NoError(t, err)
	assert.Equal(t, TriggerGuardResult{}, res)
	assert.False(t, called)
}

func TestCheckReleaseTriggerDraftReleaseRunsCreatePath(t *testing.T) {
	var (
		mu      sync.Mutex
		calls   int
		sleeps  int
		authHdr []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		authHdr = append(authHdr, r.Header.Get("Authorization"))
		assert.Equal(t, "/repos/jeduden/mdsmith/releases/tags/v1.2.3", r.URL.Path)
		switch calls {
		case 1:
			http.NotFound(w, r)
		default:
			_, _ = fmt.Fprint(w, `{"draft":true}`)
		}
	}))
	t.Cleanup(srv.Close)

	res, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName:     "create",
		Repository:    "jeduden/mdsmith",
		RefType:       "tag",
		RefName:       "v1.2.3",
		Token:         "test-token",
		APIBaseURL:    srv.URL,
		RetryAttempts: 2,
		RetryDelay:    time.Millisecond,
		Sleep: func(time.Duration) {
			sleeps++
		},
	})
	require.NoError(t, err)
	assert.Equal(t, TriggerGuardResult{
		ShouldRun:            true,
		CreateReleaseIsDraft: true,
	}, res)
	assert.Equal(t, 2, calls)
	assert.Equal(t, 1, sleeps)
	assert.Equal(t, []string{"Bearer test-token", "Bearer test-token"}, authHdr)
}

func TestCheckReleaseTriggerPublishedReleaseSkipsCreatePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"draft":false}`)
	}))
	t.Cleanup(srv.Close)

	res, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName:  "create",
		Repository: "jeduden/mdsmith",
		RefType:    "tag",
		RefName:    "v1.2.3",
		Token:      "test-token",
		APIBaseURL: srv.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, TriggerGuardResult{}, res)
}

func TestCheckReleaseTriggerMissingDraftAfterRetriesSkipsCreatePath(t *testing.T) {
	var calls, sleeps int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	res, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName:     "create",
		Repository:    "jeduden/mdsmith",
		RefType:       "tag",
		RefName:       "v1.2.3",
		Token:         "test-token",
		APIBaseURL:    srv.URL,
		RetryAttempts: 2,
		RetryDelay:    time.Millisecond,
		Sleep: func(time.Duration) {
			sleeps++
		},
	})
	require.NoError(t, err)
	assert.Equal(t, TriggerGuardResult{}, res)
	assert.Equal(t, 2, calls)
	assert.Equal(t, 1, sleeps)
}

func TestCheckReleaseTriggerUnexpectedStatusErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"boom"}`)
	}))
	t.Cleanup(srv.Close)

	_, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName:  "create",
		Repository: "jeduden/mdsmith",
		RefType:    "tag",
		RefName:    "v1.2.3",
		Token:      "test-token",
		APIBaseURL: srv.URL,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected GitHub API status 500")
}

func TestCheckReleaseTriggerRequiresRepositoryAndToken(t *testing.T) {
	_, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName: "create",
		RefType:   "tag",
		RefName:   "v1.2.3",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository")

	_, err = CheckReleaseTrigger(TriggerGuardOptions{
		EventName:  "create",
		Repository: "jeduden/mdsmith",
		RefType:    "tag",
		RefName:    "v1.2.3",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// TestCheckReleaseTriggerUsesDefaultAPIBase verifies that an
// empty APIBaseURL falls back to api.github.com (line covered:
// the apiBase == "" branch).
func TestCheckReleaseTriggerUsesDefaultAPIBase(t *testing.T) {
	var gotURL string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"draft":true}`)),
			Header:     make(http.Header),
		}, nil
	})}
	res, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName:  "create",
		Repository: "jeduden/mdsmith",
		RefType:    "tag",
		RefName:    "v1.2.3",
		Token:      "t",
		Client:     client,
	})
	require.NoError(t, err)
	assert.True(t, res.ShouldRun)
	assert.Equal(t, "https://api.github.com/repos/jeduden/mdsmith/releases/tags/v1.2.3", gotURL)
}

// TestCheckReleaseTriggerNewRequestError trips http.NewRequest by
// passing an APIBaseURL containing a control character so the URL
// parser rejects it.
func TestCheckReleaseTriggerNewRequestError(t *testing.T) {
	_, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName:  "create",
		Repository: "jeduden/mdsmith",
		RefType:    "tag",
		RefName:    "v1.2.3",
		Token:      "t",
		APIBaseURL: "http://example.com\x7f",
	})
	require.Error(t, err)
}

// TestCheckReleaseTriggerClientDoError propagates a transport
// error from client.Do.
func TestCheckReleaseTriggerClientDoError(t *testing.T) {
	sentinel := errors.New("transport boom")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, sentinel
	})}
	_, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName:  "create",
		Repository: "jeduden/mdsmith",
		RefType:    "tag",
		RefName:    "v1.2.3",
		Token:      "t",
		APIBaseURL: "https://api.example.com",
		Client:     client,
	})
	require.ErrorIs(t, err, sentinel)
}

type errReadCloser struct{ err error }

func (e errReadCloser) Read([]byte) (int, error) { return 0, e.err }
func (e errReadCloser) Close() error             { return nil }

// TestCheckReleaseTriggerReadBodyError covers the io.ReadAll
// error branch of lookupReleaseDraft.
func TestCheckReleaseTriggerReadBodyError(t *testing.T) {
	sentinel := errors.New("read boom")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       errReadCloser{err: sentinel},
			Header:     make(http.Header),
		}, nil
	})}
	_, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName:  "create",
		Repository: "jeduden/mdsmith",
		RefType:    "tag",
		RefName:    "v1.2.3",
		Token:      "t",
		APIBaseURL: "https://api.example.com",
		Client:     client,
	})
	require.ErrorIs(t, err, sentinel)
}

// TestCheckReleaseTriggerErrorBodyReadError covers the
// io.ReadAll error branch in the non-200 (diagnostic body)
// path of lookupReleaseDraft.
func TestCheckReleaseTriggerErrorBodyReadError(t *testing.T) {
	sentinel := errors.New("read boom")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       errReadCloser{err: sentinel},
			Header:     make(http.Header),
		}, nil
	})}
	_, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName:  "create",
		Repository: "jeduden/mdsmith",
		RefType:    "tag",
		RefName:    "v1.2.3",
		Token:      "t",
		APIBaseURL: "https://api.example.com",
		Client:     client,
	})
	require.ErrorIs(t, err, sentinel)
}

// TestCheckReleaseTriggerInvalidJSONErrors covers the
// json.Unmarshal error branch of lookupReleaseDraft.
func TestCheckReleaseTriggerInvalidJSONErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{not json`)
	}))
	t.Cleanup(srv.Close)

	_, err := CheckReleaseTrigger(TriggerGuardOptions{
		EventName:  "create",
		Repository: "jeduden/mdsmith",
		RefType:    "tag",
		RefName:    "v1.2.3",
		Token:      "t",
		APIBaseURL: srv.URL,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse ")
}

// TestReleaseLookupErrorEmptyBody pins the empty-body formatting
// branch of releaseLookupError.Error.
func TestReleaseLookupErrorEmptyBody(t *testing.T) {
	e := &releaseLookupError{URL: "https://x/y", StatusCode: 503}
	assert.Equal(t, "lookup https://x/y: unexpected GitHub API status 503", e.Error())
}
