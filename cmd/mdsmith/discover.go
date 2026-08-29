package main

import "github.com/jeduden/mdsmith/internal/directivefiles"

// discoverFilesWithGeneratedContent is a thin shim around
// directivefiles.DiscoverFilesForInstall so the install commands keep
// their previous "fall back to PLAN.md / README.md when no
// directives are found" behavior. The shared implementation lives in
// internal/directivefiles so the CLI and the MDS048 rule cannot
// drift on directive matching itself; only the install-time fallback
// diverges.
func discoverFilesWithGeneratedContent(repoRoot string, maxBytes int64) []string {
	return directivefiles.DiscoverFilesForInstall(repoRoot, maxBytes)
}
