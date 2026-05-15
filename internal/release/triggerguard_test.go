package release

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
