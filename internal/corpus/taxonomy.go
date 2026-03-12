package corpus

import (
	"path/filepath"
	"strings"
)

// Classify assigns taxonomy categories to records.
func Classify(records []Record) []Record {
	classified := make([]Record, len(records))
	for i, record := range records {
		record.Category = classifyRecord(record)
		classified[i] = record
	}
	return classified
}

func classifyRecord(record Record) Category {
	path := strings.ToLower(filepath.ToSlash(record.Path))
	base := strings.ToLower(filepath.Base(path))
	heading := strings.ToLower(firstMarkdownHeading(record.RawContent))

	if isReferenceSignal(path, base, heading) {
		return CategoryReference
	}
	return CategoryOther
}

func isReferenceSignal(path string, base string, heading string) bool {
	for _, token := range []string{"/reference/", "/api/", "/spec/", "/specification/", "/man/"} {
		if strings.Contains(path, token) {
			return true
		}
	}
	for _, token := range []string{"reference", "api", "spec", "schema", "config", "changelog", "release-notes"} {
		if strings.Contains(base, token) {
			return true
		}
	}
	for _, token := range []string{"reference", "api", "specification", "changelog", "command", "options"} {
		if strings.Contains(heading, token) {
			return true
		}
	}
	return false
}

func firstMarkdownHeading(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			trimmed = strings.TrimLeft(trimmed, "#")
			return strings.TrimSpace(trimmed)
		}
	}
	return ""
}
