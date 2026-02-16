package metrics

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed MET*/README.md
var docsFS embed.FS

// DocInfo holds metadata extracted from a metric README's front matter.
type DocInfo struct {
	ID          string
	Name        string
	Description string
	Content     string
}

// ListDocs returns all embedded metrics docs sorted by ID.
func ListDocs() ([]DocInfo, error) {
	return listDocsFromFS(docsFS)
}

// LookupDoc finds a metric doc by ID (e.g. MET001) or name (e.g. bytes).
func LookupDoc(query string) (string, error) {
	return lookupDocFromFS(docsFS, query)
}

func listDocsFromFS(fsys fs.FS) ([]DocInfo, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("reading metrics directory: %w", err)
	}

	var docs []DocInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		path := entry.Name() + "/README.md"
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			continue
		}

		info, err := parseFrontMatter(string(data))
		if err != nil {
			continue
		}
		info.Content = string(data)
		docs = append(docs, info)
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].ID < docs[j].ID
	})
	return docs, nil
}

func lookupDocFromFS(fsys fs.FS, query string) (string, error) {
	docs, err := listDocsFromFS(fsys)
	if err != nil {
		return "", err
	}

	q := strings.ToUpper(strings.TrimSpace(query))
	qName := strings.ToLower(strings.TrimSpace(query))
	for _, d := range docs {
		if strings.ToUpper(d.ID) == q || d.Name == qName {
			return d.Content, nil
		}
	}

	return "", fmt.Errorf("unknown metric %q", query)
}

// parseFrontMatter extracts id, name, and description from YAML front matter.
func parseFrontMatter(content string) (DocInfo, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return DocInfo{}, fmt.Errorf("missing front matter")
	}

	var info DocInfo
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, val, ok := parseYAMLLine(line)
		if !ok {
			continue
		}
		switch key {
		case "id":
			info.ID = val
		case "name":
			info.Name = val
		case "description":
			info.Description = val
		}
	}

	if info.ID == "" {
		return DocInfo{}, fmt.Errorf("front matter missing id")
	}
	if info.Name == "" {
		return DocInfo{}, fmt.Errorf("front matter missing name")
	}
	return info, nil
}

// parseYAMLLine parses a simple "key: value" line.
func parseYAMLLine(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	return key, value, true
}
