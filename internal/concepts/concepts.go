// Package concepts provides embedded concept-page documentation for
// mdsmith topics that span multiple rules or subsystems.
package concepts

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed *.md
var conceptsFS embed.FS

// Lookup returns the content of the named concept page with its front
// matter stripped. Returns an error if no concept page with that name
// exists.
func Lookup(name string) (string, error) {
	data, err := fs.ReadFile(conceptsFS, name+".md")
	if err != nil {
		return "", fmt.Errorf("unknown concept %q", name)
	}
	return stripFrontMatter(string(data)), nil
}

// stripFrontMatter removes the leading YAML front matter block (--- ... ---)
// and any immediately following blank line from content.
func stripFrontMatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return content
	}
	body := content[4+end+5:]
	return strings.TrimLeft(body, "\n")
}
