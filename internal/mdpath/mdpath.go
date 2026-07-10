// Package mdpath provides path predicates shared across rule packages.
package mdpath

import (
	"path"
	"strings"
)

// IsMarkdownPath reports whether p has a Markdown extension (.md or
// .markdown, case-insensitively). p is expected to be a forward-slash
// path, e.g. from fs.WalkDir or a workspace-relative link target;
// path.Ext (not path/filepath.Ext) keeps extension detection independent
// of the host's separator semantics.
func IsMarkdownPath(p string) bool {
	ext := path.Ext(p)
	return strings.EqualFold(ext, ".md") || strings.EqualFold(ext, ".markdown")
}
