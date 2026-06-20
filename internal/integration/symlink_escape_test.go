package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/catalog"
	"github.com/jeduden/mdsmith/internal/rules/include"
	"github.com/jeduden/mdsmith/internal/rules/tablefmt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// configureTestFile wires the per-run filesystem references onto a lint.File,
// mirroring what engine/runner.go's configureFile does when the file path is
// workspace-relative.
//
//   - f.Path is set to the workspace-relative path (e.g. "host.md").
//   - f.FS is set to an os.DirFS rooted at root (so f.FS != nil, which the
//     include/catalog rules check before operating).
//   - f.RootDir and f.RootFS are set via f.SetRootDir(root).
//
// root must be an absolute path to the project root on disk.
func configureTestFile(f *lint.File, relPath, root string) {
	f.Path = relPath
	f.FS = lint.OpenRootFS(root)
	f.SetRootDir(root)
}

// TestIncludeSymlinkEscapeRefused is the S001 acceptance test.
//
// A within-workspace symlink whose target is outside the project root
// (docs/secret.md -> /etc/passwd) must NOT have its content embedded by
// the include rule's Fix.  The file system seam (f.RootFS) must deny the
// open so that no bytes from /etc/passwd reach the generated section.
func TestIncludeSymlinkEscapeRefused(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	require.NoError(t, os.MkdirAll(docsDir, 0o755))

	// Create a symlink inside the workspace that points outside.
	symlinkPath := filepath.Join(docsDir, "secret.md")
	require.NoError(t, os.Symlink("/etc/passwd", symlinkPath))

	// Host file with an include directive targeting the symlink.
	hostSrc := "# Host\n\n<?include\nfile: docs/secret.md\n?>\n<?/include?>\n"
	hostPath := filepath.Join(root, "host.md")
	require.NoError(t, os.WriteFile(hostPath, []byte(hostSrc), 0o644))

	// Verify the vulnerability exists with os.DirFS: the symlink IS readable.
	_, errDirFS := fs.ReadFile(os.DirFS(root), "docs/secret.md")
	require.NoError(t, errDirFS,
		"test invariant: os.DirFS must be able to read the symlink target for the test to be meaningful")

	// Build a lint.File that mirrors how the engine configures it with
	// a workspace-relative path. The include rule uses f.RootFS to read.
	f, err := lint.NewFile(hostPath, []byte(hostSrc))
	require.NoError(t, err)
	configureTestFile(f, "host.md", root)

	// Fix must not embed /etc/passwd content.
	r := &include.Rule{}
	fixed := r.Fix(f)
	fixedStr := string(fixed)

	// The generated section body must not contain typical /etc/passwd content.
	assert.NotContains(t, fixedStr, "root:x:",
		"include Fix must not embed content from a symlink escaping the project root")
	assert.NotContains(t, fixedStr, "/bin/bash",
		"include Fix must not embed /etc/passwd content via symlink escape")
	t.Logf("fixed output (first 500 bytes): %.500s", fixedStr)
}

// TestIncludeSymlinkInsideRootWorks verifies that a relative symlink whose
// target is inside the project root continues to work after the containment
// fix. os.OpenRoot blocks absolute symlinks unconditionally (RESOLVE_BENEATH),
// but relative symlinks that resolve to a path inside the root are allowed.
func TestIncludeSymlinkInsideRootWorks(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	require.NoError(t, os.MkdirAll(docsDir, 0o755))

	// Real file inside the root.
	realPath := filepath.Join(docsDir, "real.md")
	require.NoError(t, os.WriteFile(realPath, []byte("Hello world\n"), 0o644))

	// Relative symlink inside the root pointing to the real file.
	// Must be relative so os.OpenRoot (RESOLVE_BENEATH) follows it.
	symlinkPath := filepath.Join(docsDir, "alias.md")
	require.NoError(t, os.Symlink("real.md", symlinkPath))

	// Host file that includes via the symlink, not the real file directly.
	// This exercises that os.OpenRoot follows relative within-root symlinks.
	hostSrc := "# Host\n\n<?include\nfile: docs/alias.md\n?>\nHello world\n<?/include?>\n"
	hostPath := filepath.Join(root, "host.md")
	require.NoError(t, os.WriteFile(hostPath, []byte(hostSrc), 0o644))

	f, err := lint.NewFile(hostPath, []byte(hostSrc))
	require.NoError(t, err)
	configureTestFile(f, "host.md", root)

	r := &include.Rule{}
	diags := r.Check(f)
	assert.Emptyf(t, diags, "within-root relative-symlink include must not produce diagnostics: %v", diags)
}

// TestCatalogSymlinkEscapeRefused is the S002 acceptance test.
//
// A within-workspace symlink whose target is outside the project root
// (docs/leaked.md -> /etc/hostname) must NOT appear as a catalog row.
// The catalog rule must produce no row for that symlink target.
func TestCatalogSymlinkEscapeRefused(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	require.NoError(t, os.MkdirAll(docsDir, 0o755))

	// Symlink inside the workspace pointing outside.
	symlinkPath := filepath.Join(docsDir, "leaked.md")
	require.NoError(t, os.Symlink("/etc/hostname", symlinkPath))

	// A real .md file so the catalog has a legitimate match alongside the symlink.
	realMD := filepath.Join(docsDir, "legit.md")
	require.NoError(t, os.WriteFile(realMD, []byte("---\nsummary: legit doc\n---\n# Legit\n"), 0o644))

	// Verify the vulnerability: with os.DirFS, fs.Stat follows the symlink
	// and treats it as a regular file — so the catalog would include it.
	info, errStat := fs.Stat(os.DirFS(root), "docs/leaked.md")
	require.NoError(t, errStat,
		"test invariant: fs.Stat via os.DirFS must follow the symlink for the test to be meaningful")
	require.True(t, info.Mode().IsRegular(),
		"test invariant: /etc/hostname must be a regular file so the catalog would include the symlink")

	// Catalog with a glob that matches both legit.md and leaked.md.
	// f.FS is set to os.DirFS(root) so the catalog uses localFSResolution
	// (no source-dir), which globs through f.FS directly.
	hostSrc := "# Index\n\n<?catalog\nglob: docs/*.md\n?>\n<?/catalog?>\n"
	hostPath := filepath.Join(root, "index.md")
	require.NoError(t, os.WriteFile(hostPath, []byte(hostSrc), 0o644))

	f, err := lint.NewFile(hostPath, []byte(hostSrc))
	require.NoError(t, err)
	// For the local-FS resolution path, f.FS is the key seam.
	// Wire f.FS via lint.OpenRootFS so the glob runs through the
	// containment boundary that the engine now uses.
	f.Path = "index.md"
	f.FS = lint.OpenRootFS(root)

	r := &catalog.Rule{Pad: 1, SeparatorStyle: tablefmt.SeparatorSpaced}
	fixed := r.Fix(f)
	fixedStr := string(fixed)

	// leaked.md symlinks to /etc/hostname — must not appear as a catalog row.
	assert.NotContains(t, fixedStr, "leaked",
		"catalog Fix must not include a catalog row for a symlink escaping the project root")
	t.Logf("fixed output: %s", fixedStr)
}
