package main

import "github.com/jeduden/mdsmith/internal/directivefiles"

// discoverFilesWithGeneratedContent is a thin shim around
// directivefiles.DiscoverFilesForInstall, which falls back to
// PLAN.md / README.md when no directives are found. Only tests call
// it; the install commands and MDS048 now derive their file lists
// from gitattributes.GlobsFromConfig.
func discoverFilesWithGeneratedContent(repoRoot string, maxBytes int64) []string {
	return directivefiles.DiscoverFilesForInstall(repoRoot, maxBytes)
}
