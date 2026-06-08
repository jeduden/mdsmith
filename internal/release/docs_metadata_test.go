package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestPublishedDocsHaveUniqueSummaries walks the docs/ tree
// excluding the maintainer-only subtrees that `SyncDocs` prunes
// (research, security, brand, development) and asserts three
// invariants that together keep the rendered <meta name=
// "description"> distinct on every URL:
//
//  1. Every Markdown file's front-matter `summary` is non-empty.
//  2. No two files share the same `summary`.
//  3. Every subdirectory has an `index.md` (or a sibling
//     `<dir>.md` overview page in the parent).
//
// Without (3), `synthesizeSectionIndex` in syncdocs.go writes a
// stub `_index.md` that carries only a title — the rendered
// page then falls through to `baseof.html`'s
// `Site.Params.description` branch and emits the same long
// Tagline as the home page's description. Several stub pages
// printing the same SEO snippet is the duplication that hit
// the live site before this fix.
//
// See docs/development/website-config.md for the design behind
// the meta-description fallback chain.
func TestPublishedDocsHaveUniqueSummaries(t *testing.T) {
	docsDir := filepath.Join(repoRoot(t), "docs")
	bySummary, missingSummary, missingSectionIndex, err := scanPublishedDocs(docsDir)
	require.NoError(t, err)

	assert.Empty(t, missingSectionIndex,
		"published doc subdirectories missing a section index (would render with the Site.Params.description fallback)")
	assert.Empty(t, missingSummary, "published docs missing a front-matter summary")

	var dups []string
	for s, paths := range bySummary {
		if len(paths) < 2 {
			continue
		}
		dups = append(dups, s+" → "+strings.Join(paths, ", "))
	}
	assert.Empty(t, dups,
		"summary duplicated across published docs (causes identical <meta name=\"description\"> on multiple URLs)")
}

// scanPublishedDocs walks docsDir and returns:
//   - bySummary: every Markdown file's summary → list of paths
//     that declared it (a single-path entry is unique; ≥2 means
//     a duplicate to flag).
//   - missingSummary: files whose front matter is absent, empty,
//     unparsable, or carries an empty `summary`.
//   - missingSectionIndex: subdirectory paths (under docs/) that
//     have neither an index.md nor a sibling `<name>.md` overview
//     page — the conditions under which Hugo's syncdocs
//     synthesizer would emit a summaryless _index.md stub.
//
// Mirrors nonPublishedDocDirs in syncdocs.go (research,
// security, brand, development); keep in lockstep with that map.
func scanPublishedDocs(docsDir string) (
	bySummary map[string][]string,
	missingSummary, missingSectionIndex []string,
	err error,
) {
	pruned := map[string]struct{}{
		"research": {}, "security": {}, "brand": {}, "development": {},
	}
	bySummary = map[string][]string{}

	walkErr := filepath.Walk(docsDir, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		rel, _ := filepath.Rel(docsDir, path)
		if info.IsDir() {
			return visitDocsDir(rel, path, pruned, &missingSectionIndex)
		}
		summary, miss, ok := readSummary(path)
		if miss != "" {
			missingSummary = append(missingSummary, miss)
			return nil
		}
		if ok {
			bySummary[summary] = append(bySummary[summary], path)
		}
		return nil
	})
	return bySummary, missingSummary, missingSectionIndex, walkErr
}

// visitDocsDir applies the section-index policy to one
// directory and returns filepath.SkipDir for pruned subtrees.
// docs/ root is exempt — its landing page is the website's own
// content/_index.md.
func visitDocsDir(rel, path string, pruned map[string]struct{}, missing *[]string) error {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) > 0 {
		if _, skip := pruned[parts[0]]; skip {
			return filepath.SkipDir
		}
	}
	if rel == "." {
		return nil
	}
	if _, err := os.Stat(filepath.Join(path, "index.md")); err == nil {
		return nil
	}
	sibling := filepath.Join(filepath.Dir(path), filepath.Base(path)+".md")
	if _, err := os.Stat(sibling); err == nil {
		return nil
	}
	*missing = append(*missing,
		"docs/"+filepath.ToSlash(rel)+
			" (no index.md and no sibling overview .md — the Hugo synthesizer would emit a summaryless stub)")
	return nil
}

// readSummary returns (summary, miss, ok) for one Markdown
// file. summary is the trimmed front-matter `summary` value
// when ok is true; otherwise miss carries the diagnostic
// string the caller should add to its "missing summaries"
// list. Skips proto.md templates and non-.md files (ok=false,
// miss="" — silently ignored).
func readSummary(path string) (summary, miss string, ok bool) {
	if filepath.Ext(path) != ".md" || filepath.Base(path) == "proto.md" {
		return "", "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", path + " (read error: " + err.Error() + ")", false
	}
	fm := extractFrontmatter(data)
	if fm == nil {
		return "", path + " (no front matter)", false
	}
	var meta struct {
		Summary string `yaml:"summary"`
	}
	if err := yaml.Unmarshal(fm, &meta); err != nil {
		return "", path + " (front matter parse error: " + err.Error() + ")", false
	}
	s := strings.TrimSpace(meta.Summary)
	if s == "" {
		return "", path + " (empty summary)", false
	}
	return s, "", true
}

// extractFrontmatter returns the YAML body between the leading
// `---\n` fences, or nil when the document carries no front
// matter. Mirrors the minimal logic Hugo uses; the test does not
// need the full pkg/markdown front-matter parser.
func extractFrontmatter(data []byte) []byte {
	const fence = "---\n"
	s := string(data)
	if !strings.HasPrefix(s, fence) {
		return nil
	}
	rest := s[len(fence):]
	end := strings.Index(rest, "\n"+fence[:3])
	if end < 0 {
		return nil
	}
	return []byte(rest[:end+1])
}
