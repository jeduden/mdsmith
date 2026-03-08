package include

import (
	"path"
	"regexp"
	"strings"
)

// linkRe matches Markdown links [text](target) and images ![alt](target).
// It handles nested brackets in the text portion. Does not handle link
// titles (e.g. [x](url "title")) or URLs containing literal ')'.
var linkRe = regexp.MustCompile(`(!?\[(?:[^\[\]]|\[[^\]]*\])*\])\(([^)]*)\)`)

// adjustLinks rewrites relative link targets in content so they remain valid
// when the included file (includedFilePath) is rendered inside the including
// file (includingFilePath). Both paths are FS-root-relative, slash-separated.
// Links inside fenced code blocks and inline code spans are left unchanged.
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
// leaving fenced code blocks and inline code spans unchanged.
func rewriteSkippingCode(content string, rewriteFn func(string) string) string {
	var b strings.Builder
	inFence := false
	var fenceChar byte
	var fenceLen int

	lines := strings.SplitAfter(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")

		if inFence {
			b.WriteString(line)
			stripped := strings.TrimRight(trimmed, " \t\n")
			if len(stripped) >= fenceLen && allSameChar(stripped, fenceChar) {
				inFence = false
			}
			continue
		}

		// Detect opening fence: capture exact run length.
		if run := countFenceRun(trimmed); run > 0 {
			inFence = true
			fenceChar = trimmed[0]
			fenceLen = run
			b.WriteString(line)
			continue
		}

		// Process line respecting inline code spans.
		b.WriteString(rewriteLineSkippingInlineCode(line, rewriteFn))
	}

	return b.String()
}

// rewriteLineSkippingInlineCode applies rewriteFn to parts of a line
// outside backtick-delimited inline code spans. Handles multi-backtick
// delimiters where the opening and closing run lengths must match.
func rewriteLineSkippingInlineCode(line string, rewriteFn func(string) string) string {
	var b strings.Builder
	inCode := false
	codeRunLen := 0
	start := 0

	for i := 0; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}

		// Count consecutive backticks.
		j := i
		for j < len(line) && line[j] == '`' {
			j++
		}
		runLen := j - i

		if inCode {
			if runLen == codeRunLen {
				b.WriteString(line[start:j])
				start = j
				inCode = false
				codeRunLen = 0
			}
		} else {
			if start < i {
				b.WriteString(rewriteFn(line[start:i]))
			}
			b.WriteString(line[i:j])
			start = j
			inCode = true
			codeRunLen = runLen
		}

		i = j
	}

	if start < len(line) {
		if inCode {
			b.WriteString(line[start:])
		} else {
			b.WriteString(rewriteFn(line[start:]))
		}
	}
	return b.String()
}

// countFenceRun returns the length of a backtick or tilde run at the
// start of trimmed (after whitespace was already stripped). Returns 0
// if no fence is detected (run < 3).
func countFenceRun(trimmed string) int {
	if len(trimmed) < 3 {
		return 0
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return 0
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == ch {
		n++
	}
	if n < 3 {
		return 0
	}
	return n
}

// allSameChar checks if s consists entirely of character ch.
func allSameChar(s string, ch byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ch {
			return false
		}
	}
	return true
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
