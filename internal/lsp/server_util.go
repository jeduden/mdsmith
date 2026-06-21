package lsp

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
)

// workspaceRelative converts an absolute filesystem path to a path
// relative to the workspace root. Returns the input unchanged when
// root is empty, when path is already relative, or when path lies
// outside root (which would otherwise produce an unhelpful "../"
// prefix that does not match repo-style globs).
func workspaceRelative(root, path string) string {
	if root == "" || !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	// Only treat true parent traversals as outside root. A bare
	// HasPrefix(rel, "..") would also match in-root files whose
	// names happen to start with two dots (e.g. "..foo.md"),
	// breaking glob/ignore matching for those files.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}

// dirFSForPath returns os.DirFS rooted at the directory containing
// path, or nil when path is not absolute (e.g. an in-memory test
// label). engine.Runner treats a nil SourceFS as "do not override
// the default" so this is safe in all cases.
func dirFSForPath(path string) fs.FS {
	if !filepath.IsAbs(path) {
		return nil
	}
	return os.DirFS(filepath.Dir(path))
}

func frontMatterEnabled(cfg *config.Config) bool {
	if cfg == nil || cfg.FrontMatter == nil {
		return true
	}
	return *cfg.FrontMatter
}

func isFixable(rules []rule.Rule, name string) bool {
	for _, r := range rules {
		if r.Name() != name {
			continue
		}
		_, ok := r.(rule.FixableRule)
		return ok
	}
	return false
}

// uriToPath converts a `file://` URI to a filesystem path. Non-file
// URIs return "" so the caller can skip them.
//
// Host handling:
//
//   - Empty host (`file:///path`) is the common case.
//   - "localhost" is treated as empty per RFC 8089 §3.
//   - On Windows, a non-empty/non-localhost host produces a UNC path
//     (`\\server\share\...`); on other platforms we conservatively
//     return "" because we have no way to mount a remote share.
func uriToPath(uri string) string {
	return uriToPathOnOS(uri, runtime.GOOS)
}

// uriToPathOnOS is uriToPath split out so tests can exercise the
// Windows-only branches (UNC translation, drive-letter stripping)
// from any platform.
func uriToPathOnOS(uri, goos string) string {
	if !strings.HasPrefix(uri, "file://") {
		return ""
	}
	u, err := url.Parse(uri)
	// url.Parse fails on invalid percent-encoded sequences (e.g.
	// "file://%GH/path" — %GH is not a valid escape). Return ""
	// so the caller skips the URI, matching the non-file prefix
	// guard above.
	if err != nil {
		return ""
	}
	host := u.Host
	if strings.EqualFold(host, "localhost") {
		host = ""
	}
	p := u.Path
	if host != "" {
		// UNC path on Windows: file://server/share/path -> \\server\share\path
		if goos == "windows" {
			// Require at least a share component: p must be longer than
			// "/" (i.e. the URI must be file://server/share/...). A
			// host-only URI (file://server or file://server/) yields
			// p="" or p="/", producing "\\server" — not a navigable UNC
			// path. Return "" so callers skip it rather than attempting
			// filesystem operations on a host-only stub.
			if len(p) <= 1 {
				return ""
			}
			return filepath.Clean(`\\` + host + filepath.FromSlash(p))
		}
		// Non-Windows: we cannot resolve a remote share, so refuse.
		return ""
	}
	// Windows: file:///C:/foo decodes to "/C:/foo"; strip the
	// leading slash only when the path actually starts with a
	// drive-letter pattern, so a non-Windows absolute path whose
	// third byte happens to be ':' (e.g. "/a:/tmp/file.md") is left
	// alone. The check is also gated on Windows so the fix never
	// fires on platforms that don't have drive letters.
	if goos == "windows" && hasDriveLetterPrefix(p) {
		p = p[1:]
		// A bare drive root ("X:") after stripping the leading slash
		// must return "X:\", not "X:." — filepath.Clean("X:") on
		// Windows treats "X:" as a relative drive path and appends ".".
		// Return early with the canonical root so we bypass Clean's
		// platform-dependent behaviour for this one case.
		if len(p) == 2 {
			return p + `\`
		}
	}
	return filepath.Clean(p)
}

// hasDriveLetterPrefix reports whether p starts with "/X:/" or "/X:"
// where X is an ASCII letter — the canonical Windows
// drive-letter-after-leading-slash pattern url.Parse produces for a
// `file:///C:/...` URI. The bare-root form "/X:" (len==3) is matched
// so the caller can handle it (appending a separator before Clean).
func hasDriveLetterPrefix(p string) bool {
	if len(p) < 3 || p[0] != '/' || p[2] != ':' {
		return false
	}
	c := p[1]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// pickRoot derives the workspace root from initialize params.
func pickRoot(p initializeParams) string {
	if len(p.WorkspaceFolders) > 0 {
		if path := uriToPath(p.WorkspaceFolders[0].URI); path != "" {
			return path
		}
	}
	// rootUri is `DocumentUri | null` per LSP §3.16. The pointer
	// dereference covers both the missing-key case (nil) and the
	// explicit JSON null case (also nil after Unmarshal).
	if p.RootURI != nil {
		if path := uriToPath(*p.RootURI); path != "" {
			return path
		}
	}
	return ""
}
