package release

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resp200 is the shorthand for a successful fakeGetter response.
func resp200(body string) struct {
	status int
	body   []byte
	err    error
} {
	return struct {
		status int
		body   []byte
		err    error
	}{200, []byte(body), nil}
}

// TestPullSiteAssets_TransportError covers the err != nil branch
// of PullSiteAssets for the demo GIF: a transport failure on it
// fails the deploy with a wrapped error rather than being silently
// swallowed.
func TestPullSiteAssets_TransportError(t *testing.T) {
	transport := errors.New("dial tcp: connection refused")

	t.Run("transport error fails the deploy", func(t *testing.T) {
		root := t.TempDir()
		g := &fakeGetter{resp: map[string]struct {
			status int
			body   []byte
			err    error
		}{
			rawAssetsBase + "demo.gif": {0, nil, transport},
		}}
		err := NewWithHTTP(osFS{}, g).PullSiteAssets(root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo.gif")
		assert.Contains(t, err.Error(), "connection refused")
	})
}

// TestPullSiteAssets_FSFaults covers the MkdirAll and WriteFile
// error branches of PullSiteAssets by pairing an all-200 getter
// with the fault-injecting FS, so a successful fetch still fails to
// land on disk.
func TestPullSiteAssets_FSFaults(t *testing.T) {
	all200 := func() *fakeGetter {
		return &fakeGetter{resp: map[string]struct {
			status int
			body   []byte
			err    error
		}{
			rawAssetsBase + "demo.gif": resp200("GIF"),
		}}
	}

	t.Run("mkdir failure", func(t *testing.T) {
		ff := newFakeFS()
		ff.failOnMkdirAllCall = 1 // first asset's parent dir
		err := NewWithHTTP(ff, all200()).PullSiteAssets(t.TempDir())
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "mkdir")
	})

	t.Run("write failure", func(t *testing.T) {
		ff := newFakeFS()
		ff.failOnWriteFileCall = 1 // first asset's write
		err := NewWithHTTP(ff, all200()).PullSiteAssets(t.TempDir())
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "write ")
	})
}
