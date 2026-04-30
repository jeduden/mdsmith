package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

// discoverFilesWithGeneratedContent scans the repository for markdown
// files containing generated section directives (catalog, include, toc).
// Returns a list of file paths relative to repoRoot, or falls back to
// sensible defaults if discovery fails or finds no files.
func discoverFilesWithGeneratedContent(repoRoot string, maxBytes int64) []string {
	var filesWithDirectives []string

	// Get directive names from registered rules.
	directiveNames := make(map[string]bool)
	for _, r := range rule.All() {
		if d, ok := r.(gensection.Directive); ok {
			directiveNames[d.Name()] = true
		}
	}

	// Walk the repository looking for markdown files.
	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}

		// Skip non-files, hidden directories, and non-markdown files.
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(info.Name(), ".md") && !strings.HasSuffix(info.Name(), ".markdown") {
			return nil
		}

		// Read file and check for directives.
		content, err := lint.ReadFileLimited(path, maxBytes)
		if err != nil {
			return nil // Skip files we can't read
		}

		// Check if file contains any directive markers.
		hasDirective := false
		for name := range directiveNames {
			marker := []byte("<?" + name)
			if bytes.Contains(content, marker) {
				hasDirective = true
				break
			}
		}

		if hasDirective {
			// Convert to relative path from repo root.
			relPath, err := filepath.Rel(repoRoot, path)
			if err == nil {
				filesWithDirectives = append(filesWithDirectives, relPath)
			}
		}

		return nil
	})

	// If discovery failed or found nothing, return sensible defaults.
	if err != nil || len(filesWithDirectives) == 0 {
		return []string{"PLAN.md", "README.md"}
	}

	return filesWithDirectives
}
