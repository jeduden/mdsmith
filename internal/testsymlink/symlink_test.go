package testsymlink

import "testing"

func TestSkipIfSymlinkUnsupported_DoesNotSkipOnSupportedHost(t *testing.T) {
	// Verify the helper runs without skipping on a host that supports symlinks.
	// If symlinks are unsupported the test is skipped rather than failed, so
	// this test is safe to run everywhere.
	SkipIfSymlinkUnsupported(t)
}
