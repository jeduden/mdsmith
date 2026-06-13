// Package bytelimit guards against oversized inputs. It reads a file —
// from disk or an fs.FS — up to a byte cap, returning an error when the
// file exceeds it.
package bytelimit

import (
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
)

// DefaultMaxInputBytes is the default file-size cap (2 MB, binary).
const DefaultMaxInputBytes int64 = 2 * 1024 * 1024

// ReadFileLimited reads path from disk, returning an error if the file
// exceeds max bytes. When max <= 0 or max == math.MaxInt64 no limit is
// applied (unlimited mode). MaxInt64 is treated as unlimited because the
// +1 sentinel used internally would overflow.
func ReadFileLimited(path string, max int64) ([]byte, error) {
	if max <= 0 || max == math.MaxInt64 {
		return os.ReadFile(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // best-effort close on read-only file

	return readLimited(f, path, max)
}

// ReadFileLimitedInto reads path from disk into the caller-owned buffer
// *buf, growing it only when its capacity is short, and returns the
// filled slice (resliced from *buf). It applies the same max+1 overflow
// check as ReadFileLimited: a file larger than max returns an error.
// When max <= 0 or max == math.MaxInt64 no limit is applied; in that
// mode the read still fills *buf so the caller's buffer is reused.
//
// The buffer lets a caller pool one allocation across many reads — the
// engine's lintFile draws a *[]byte from a sync.Pool, passes it here,
// and returns it after the parsed File dies. *buf is updated to the
// (possibly grown) backing array so the next call can reuse the larger
// capacity. The returned slice aliases *buf; callers must not return
// the buffer to a pool while that slice (or anything aliasing it, like
// a File's Source/Lines) is still live.
func ReadFileLimitedInto(path string, buf *[]byte, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // best-effort close on read-only file

	if max <= 0 || max == math.MaxInt64 {
		return readAllInto(f, buf, math.MaxInt64, statSize(f))
	}
	return readLimitedInto(f, buf, max)
}

// ReadFSFileLimited reads name from fsys, returning an error if the file
// exceeds max bytes. When max <= 0 or max == math.MaxInt64 no limit is
// applied (unlimited mode).
func ReadFSFileLimited(fsys fs.FS, name string, max int64) ([]byte, error) {
	if max <= 0 || max == math.MaxInt64 {
		return fs.ReadFile(fsys, name)
	}

	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // best-effort close on read-only file

	return readLimited(f, name, max)
}

// readLimited reads from r up to max+1 bytes. If the read returns more
// than max bytes the file is too large. The +1 sentinel distinguishes
// "exactly at limit" from "truncated".
//
// When the underlying reader is a file, we stat it first to report the
// actual file size in the error message. For non-file readers (or when
// stat fails), we report the truncated read length.
func readLimited(r io.Reader, name string, max int64) ([]byte, error) {
	actualSize := statSize(r)

	// Pre-size the read buffer from the stat size (like os.ReadFile) so
	// the common in-cap read is a single allocation rather than
	// io.ReadAll's repeated grow-and-copy. Read through LimitReader(max+1)
	// regardless so a file that grew past the cap since the stat is still
	// flagged as too large.
	var buf []byte
	data, err := readAllInto(io.LimitReader(r, max+1), &buf, max, actualSize)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fileTooLargeErr(actualSize, int64(len(data)), max)
	}
	return data, nil
}

// fileTooLargeErr builds the standard "file too large" error. It reports
// actualSize when it is known (≥ 0); otherwise it falls back to dataLen
// (the number of bytes actually read through the LimitReader sentinel).
func fileTooLargeErr(actualSize, dataLen, max int64) error {
	reported := actualSize
	if reported < 0 {
		reported = dataLen
	}
	return fmt.Errorf("file too large (%d bytes, max %d)", reported, max)
}

// statSize returns r's file size, or -1 when it cannot be determined.
func statSize(r io.Reader) int64 {
	if st, ok := r.(interface{ Stat() (os.FileInfo, error) }); ok {
		if info, err := st.Stat(); err == nil {
			return info.Size()
		}
	}
	return -1
}

// readLimitedInto mirrors readLimited but fills the caller-owned buffer
// *buf instead of allocating a fresh slice. The max+1 sentinel read and
// the too-large error are identical to readLimited.
func readLimitedInto(r io.Reader, buf *[]byte, max int64) ([]byte, error) {
	actualSize := statSize(r)
	data, err := readAllInto(io.LimitReader(r, max+1), buf, max, actualSize)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fileTooLargeErr(actualSize, int64(len(data)), max)
	}
	return data, nil
}

// readAllInto reads r to EOF into the caller-owned buffer *buf. It
// starts from (*buf)[:0], seeds capacity from sizeHint when the buffer
// is too small, grows only when capacity runs short, and writes the
// grown backing array back through buf so the next call reuses it.
// The returned slice aliases *buf.
func readAllInto(r io.Reader, buf *[]byte, max, sizeHint int64) ([]byte, error) {
	data := (*buf)[:0]
	if sizeHint >= 0 && sizeHint <= max {
		if h := sizeHint + 1; int64(int(h)) == h && cap(data) < int(h) { // +1 for EOF read; guard int overflow
			data = make([]byte, 0, int(h))
		}
	}
	if cap(data) == 0 {
		data = make([]byte, 0, 512)
	}
	for {
		if len(data) >= cap(data) {
			data = append(data, 0)[:len(data)] // grow, preserve len
		}
		n, err := r.Read(data[len(data):cap(data)])
		data = data[:len(data)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			*buf = data
			return data, err
		}
	}
}
