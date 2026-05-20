package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// swapGenerateSBOM replaces generateSBOMFunc for the test's
// lifetime, restoring the production binding via t.Cleanup. Tests
// own the captured args so they can assert root/outPath without
// invoking the real toolkit.
func swapGenerateSBOM(t *testing.T, fn func(root, outPath string) error) {
	t.Helper()
	orig := generateSBOMFunc
	t.Cleanup(func() { generateSBOMFunc = orig })
	generateSBOMFunc = fn
}

func TestRunSBOMHappyPath(t *testing.T) {
	var gotRoot, gotOut string
	swapGenerateSBOM(t, func(root, outPath string) error {
		gotRoot = root
		gotOut = outPath
		return nil
	})

	assert.Equal(t, 0, run([]string{"sbom", "sbom.cdx.json"}))
	assert.Equal(t, "sbom.cdx.json", gotOut)
	assert.NotEmpty(t, gotRoot, "runSBOM must forward the dispatcher's root cwd")
}

func TestRunSBOMReportsError(t *testing.T) {
	swapGenerateSBOM(t, func(string, string) error {
		return errors.New("cyclonedx-gomod failed")
	})
	assert.Equal(t, 1, run([]string{"sbom", "sbom.cdx.json"}))
}

func TestRunSBOMRejectsBadArity(t *testing.T) {
	// Patch to a no-op so a mistaken pass-through never shells
	// out, but the NArg gate should fire first regardless.
	swapGenerateSBOM(t, func(string, string) error { return nil })
	for _, argv := range [][]string{
		{"sbom"},
		{"sbom", "a", "b"},
	} {
		assert.Equal(t, 2, run(argv), "%v", argv)
	}
}

func TestRunSBOMRejectsUnknownFlag(t *testing.T) {
	swapGenerateSBOM(t, func(string, string) error { return nil })
	assert.Equal(t, 2, run([]string{"sbom", "--bogus", "sbom.cdx.json"}))
}

func TestRunSBOMHelpExitsZero(t *testing.T) {
	assert.Equal(t, 0, run([]string{"sbom", "--help"}))
}
