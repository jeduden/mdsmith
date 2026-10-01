package main

import "github.com/jeduden/mdsmith/internal/directivefiles"

// discoverFilesWithGeneratedContent is a thin shim around
// directivefiles.DiscoverFilesForInstall, which falls back to
// PLAN.md / README.md when no directives are found. Only tests call
// it: `mdsmith merge-driver install` and MDS048 derive their glob set
// from gitattributes.GlobsFromConfig, and the pre-merge-commit hook
// runs `mdsmith fix .` with no file list.
func discoverFilesWithGeneratedContent(repoRoot string, maxBytes int64) []string {
	return directivefiles.DiscoverFilesForInstall(repoRoot, maxBytes)
}
