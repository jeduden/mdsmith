package lint

import (
	"io"
	"io/fs"
)

// RootFS is the fs.FS OpenRootFS returns: a view of one directory that
// holds an open handle until Close. A caller that walks the tree once
// closes it when the walk ends; every read after Close fails.
type RootFS interface {
	fs.FS
	io.Closer
}

// closingFS gives fsys a Close and forwards the optional fs interfaces
// fsys implements (os.Root's FS and os.DirFS implement all five), so
// fs.ReadFile, fs.ReadDir, fs.Stat, fs.WalkDir, fs.ReadLink, and
// fs.Lstat keep their fast paths through the wrapper. It is used by
// pointer, so two fs.FS values holding one compare without panicking on
// the func field.
type closingFS struct {
	fsys  fs.FS
	close func() error
}

func (c *closingFS) Open(name string) (fs.File, error) { return c.fsys.Open(name) }

// Close releases the handle behind fsys.
func (c *closingFS) Close() error { return c.close() }

func (c *closingFS) ReadFile(name string) ([]byte, error) { return fs.ReadFile(c.fsys, name) }

func (c *closingFS) ReadDir(name string) ([]fs.DirEntry, error) { return fs.ReadDir(c.fsys, name) }

func (c *closingFS) Stat(name string) (fs.FileInfo, error) { return fs.Stat(c.fsys, name) }

func (c *closingFS) ReadLink(name string) (string, error) { return fs.ReadLink(c.fsys, name) }

func (c *closingFS) Lstat(name string) (fs.FileInfo, error) { return fs.Lstat(c.fsys, name) }
