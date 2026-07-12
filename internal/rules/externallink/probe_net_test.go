//go:build !(js && wasm)

package externallink

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDo_NewRequestError exercises the http.NewRequest error branch in
// do(). An invalid method (space is not an HTTP token character) causes
// http.NewRequest to fail before any network I/O.
func TestDo_NewRequestError(t *testing.T) {
	r := newConfiguredRule(t, nil)
	r.initOnce.Do(r.init)
	_, err := r.do("BAD METHOD", "http://localhost/")
	require.Error(t, err)
}
