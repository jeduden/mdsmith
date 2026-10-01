package include

import (
	"path"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/mdfence"
)

// linkRe matches Markdown links [text](target) and images ![alt](target).
// It handles nested brackets in the text portion. Does not handle link
// titles (e.g. [x](url "title")) or URLs containing literal ')'.
var linkRe = regexp.MustCompile(`(!?\[(?:[^\[\]]|\[[^\]]*\])*\])\(([^)]*)\)`)

// adjustLinks rewrites relative link targets in content so they remain valid
// when the included file (includedFilePath) is rendered inside the including
// file (includingFilePath). Both paths are FS-root-relative, slash-separated.
// Links inside fenced code blocks are left unchanged.
func adjustLinks(content string, includedFilePath string, includingFilePath string) string {
	includedDir := path.Dir(includedFilePath)
	includingDir := path.Dir(includingFilePath)

	if includedDir == includingDir {
		return content
	}

	rewriteSegment := func(segment string) string {
		return linkRe.ReplaceAllStringFunc(segment, func(match string) string {
			sub := linkRe.FindStringSubmatch(match)
			if sub == nil {
				return match
			}
			linkText := sub[1] // e.g. [text] or ![alt]
			target := sub[2]   // e.g. foo.md#section

			if target == "" || shouldSkip(target) {
				return match
			}

			pathPart, suffix := splitTargetSuffix(target)

			trailingSlash := strings.HasSuffix(pathPart, "/")
			resolved := path.Join(includedDir, pathPart)
			newPath, err := relPath(includingDir, resolved)
			if err != nil {
				return match
			}
			if trailingSlash && !strings.HasSuffix(newPath, "/") {
				newPath += "/"
			}

			return linkText + "(" + newPath + suffix + ")"
		})
	}

	return rewriteSkippingCode(content, rewriteSegment)
}

// rewriteSkippingCode applies rewriteFn to non-code portions of content,
// leaving fenced code block lines unchanged. Link rewriting is applied on
// full lines so that backticks inside link text (e.g. [`name`](target))
// do not prevent matching.
func rewriteSkippingCode(content string, rewriteFn func(string) string) string {
	var b strings.Builder
	var fence mdfence.Tracker

	lines := strings.SplitAfter(content, "\n")
	for _, line := range lines {
		if stepFence(&fence, line) {
			b.WriteString(line)
			continue
		}

		b.WriteString(rewriteFn(line))
	}

	return b.String()
}

// shouldSkip returns true for targets that must not be rewritten.
func shouldSkip(target string) bool {
	if strings.HasPrefix(target, "#") {
		return true
	}
	if strings.HasPrefix(target, "/") {
		return true
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return true
	}
	if strings.HasPrefix(target, "mailto:") {
		return true
	}
	// Skip targets with unescaped whitespace — likely link titles
	// (e.g. [x](url "title")) which we cannot safely rewrite.
	if strings.ContainsAny(target, " \t") {
		return true
	}
	return false
}

// splitTargetSuffix splits a link target into the path portion and a suffix
// containing the query string and/or fragment.
// For example "foo.md?v=1#section" returns ("foo.md", "?v=1#section").
func splitTargetSuffix(target string) (string, string) {
	// Find the earliest of '?' or '#'.
	idx := len(target)
	if i := strings.Index(target, "?"); i >= 0 && i < idx {
		idx = i
	}
	if i := strings.Index(target, "#"); i >= 0 && i < idx {
		idx = i
	}
	return target[:idx], target[idx:]
}

// relPath computes a relative path from base to target using the path package.
// Both arguments must be slash-separated, clean paths.
func relPath(base, target string) (string, error) {
	// path.Clean to normalize.
	base = path.Clean(base)
	target = path.Clean(target)

	// Split into components.
	baseParts := splitParts(base)
	targetParts := splitParts(target)

	// Find common prefix length.
	common := 0
	for common < len(baseParts) && common < len(targetParts) && baseParts[common] == targetParts[common] {
		common++
	}

	// Build relative path: go up from base, then down to target.
	ups := len(baseParts) - common
	parts := make([]string, 0, ups+len(targetParts)-common)
	for i := 0; i < ups; i++ {
		parts = append(parts, "..")
	}
	parts = append(parts, targetParts[common:]...)

	if len(parts) == 0 {
		return ".", nil
	}
	return strings.Join(parts, "/"), nil
}

// splitParts splits a clean path into its components, handling "." as empty.
func splitParts(p string) []string {
	if p == "." || p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
