package refactor

import (
	"bytes"
	"errors"
	"path"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdpath"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

// ErrTraversalPath is returned when a move source or destination
// escapes the workspace (an absolute path or one that resolves outside
// the root). Nothing is moved or written.
var ErrTraversalPath = errors.New("path escapes the workspace")

// ErrSameFile is returned when a move's source and destination
// normalize to the same workspace-relative path.
var ErrSameFile = errors.New("source and destination are the same file")

// DestinationExistsError reports that a move's destination already
// exists. The move aborts with no edit written, so an existing file is
// never clobbered.
type DestinationExistsError struct{ Dst string }

func (e DestinationExistsError) Error() string {
	return "destination already exists: " + e.Dst
}

// SourceNotFoundError reports that a move's source file is not readable
// in the workspace.
type SourceNotFoundError struct{ Src string }

func (e SourceNotFoundError) Error() string {
	return "source file not found: " + e.Src
}

// Move computes a Plan that relocates the workspace file src to dst and
// rewrites every reference so no link breaks in either direction. The
// returned Plan carries a FileOp{From: src, To: dst} the host executes
// after applying the edits (a git mv when tracked, a plain rename
// otherwise); the engine never touches the filesystem.
//
// The Plan rewrites, keyed per output target:
//
//   - incoming file-link paths — `[t](src)` / `[t](src#frag)` in other
//     files, path token rewritten to resolve to dst, the fragment kept;
//   - ref-def destinations — `[label]: src` lines, path token rewritten;
//   - wikilink stems — `[[old-stem]]` → `[[new-stem]]`, but only when
//     the basename stem changes; a move that keeps the basename leaves
//     wikilinks alone because a stem still resolves (a documented
//     asymmetry with path links);
//   - outbound relative links inside src, recomputed so each still
//     resolves from dst's directory.
//
// Spelling is preserved: an explicit `./x` keeps its prefix. Absolute
// URLs, mailto, root-anchored `/x`, and any other out-of-workspace
// path are never touched — they do not resolve to a workspace file, so
// a move has nothing to rewrite.
//
// A traversal path returns ErrTraversalPath; an equal src/dst returns
// ErrSameFile; a missing source returns SourceNotFoundError; an
// existing destination returns DestinationExistsError. Each aborts with
// a zero Plan and no edit.
//
// `<?include?>`, `<?build?>`, and `<?catalog?>` directive paths are not
// yet rewritten — a move that a directive path targets still needs a
// manual fix; this is a tracked follow-up.
func Move(ws Workspace, src, dst string) (Plan, error) {
	src = index.NormalizePath(src)
	dst = index.NormalizePath(dst)
	if !workspaceRelative(src) || !workspaceRelative(dst) {
		return Plan{}, ErrTraversalPath
	}
	if src == dst {
		return Plan{}, ErrSameFile
	}
	srcKey, srcSource, ok := ws.Resolve(src)
	if !ok {
		return Plan{}, SourceNotFoundError{Src: src}
	}
	if _, _, exists := ws.Resolve(dst); exists {
		return Plan{}, DestinationExistsError{Dst: dst}
	}

	changes := map[string][]Edit{}
	appendIncomingPathEdits(changes, ws, src, dst)
	appendRefDefPathEdits(changes, ws, src, dst)
	appendWikilinkStemEdits(changes, ws, src, dst)
	appendOutboundEdits(changes, srcKey, src, dst, srcSource)
	stableSortEdits(changes)
	return Plan{Edits: changes, FileOp: &FileOp{From: src, To: dst}}, nil
}

// workspaceRelative reports whether p is a safe workspace-relative path
// — not absolute, not a `..` traversal. p is assumed already
// NormalizePath-cleaned (forward slashes, no leading `./`).
func workspaceRelative(p string) bool {
	if p == "" {
		return false
	}
	t := filepath.ToSlash(p)
	if path.IsAbs(t) {
		return false
	}
	cleaned := path.Clean(t)
	return cleaned != ".." && !strings.HasPrefix(cleaned, "../")
}

// recomputeToken returns the path token a reference in refFile must
// carry to point at target, given the token's original spelling. An
// explicit `./x` keeps its prefix unless the recomputed path climbs out
// of the directory (a `../` result already reads as relative);
// everything else is bare-relative. Only workspace-relative links reach
// here — an absolute or root-anchored `/x` never resolves to a
// workspace file (see linkgraph.ResolveRelTarget), so a move leaves
// those tokens untouched upstream rather than recomputing them.
func recomputeToken(refFile, oldTok, target string) string {
	rel := relFrom(path.Dir(refFile), target)
	if strings.HasPrefix(oldTok, "./") && !strings.HasPrefix(rel, "../") {
		return "./" + rel
	}
	return rel
}

// relFrom returns the forward-slash path from fromDir to target, both
// workspace-relative. It falls back to target on the rare error path
// (paths on different volumes), which cannot happen for two
// workspace-relative inputs.
func relFrom(fromDir, target string) string {
	r, err := filepath.Rel(fromDir, target)
	if err != nil {
		return target
	}
	return filepath.ToSlash(r)
}

// appendIncomingPathEdits rewrites every incoming file-link path that
// resolves to src so it resolves to dst instead, keeping any fragment.
// A self-link inside src is left to the outbound pass so the same token
// is never edited twice.
func appendIncomingPathEdits(changes map[string][]Edit, ws Workspace, src, dst string) {
	for _, e := range ws.IncomingPathEdges(src) {
		// Directive paths (include/build) are a tracked follow-up; only
		// regular file links are rewritten here.
		if e.Kind != index.EdgeFileLink {
			continue
		}
		if index.NormalizePath(e.SourceFile) == src {
			continue
		}
		key, source, ok := ws.Resolve(e.SourceFile)
		if !ok {
			continue
		}
		lines := splitLines(source)
		if e.SourceLine < 1 || e.SourceLine > len(lines) {
			continue
		}
		row := lines[e.SourceLine-1]
		ps, pe, ok := linkPathBytesResolving(row, e.SourceCol-1, e.SourceFile, src)
		if !ok {
			continue
		}
		if edit, ok := pathEdit(row, e.SourceLine-1, ps, pe, e.SourceFile, dst); ok {
			changes[key] = append(changes[key], edit)
		}
	}
}

// appendOutboundEdits recomputes every relative link inside the moved
// file so it still resolves from dst's directory. Edits key under the
// moved file's own key: the host applies them before the file relocates.
func appendOutboundEdits(changes map[string][]Edit, srcKey, src, dst string, source []byte) {
	body, fmOffset := bodyAndFMOffset(source)
	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	lf := &lint.File{
		Path:       src,
		Source:     body,
		Lines:      bytes.Split(body, []byte("\n")),
		AST:        root,
		LineOffset: fmOffset,
	}
	fileLines := splitLines(source)
	for _, l := range linkgraph.ExtractLinks(lf) {
		if l.Target.LocalAnchor {
			continue
		}
		tgt := linkgraph.ResolveRelTarget(src, l.Target.Path)
		if tgt == "" {
			// External, absolute, or out-of-workspace — leave untouched.
			continue
		}
		// fileLine is always in range: l.Line is a body line and
		// fileLines covers the body plus its fmOffset prefix, so unlike
		// the index-fed incoming pass there is no stale coordinate here.
		fileLine := l.Line + fmOffset
		row := fileLines[fileLine-1]
		ps, pe, ok := linkPathBytesResolving(row, l.Column-1, src, tgt)
		if !ok {
			continue
		}
		// The reference lives in the moved file, so its new spelling is
		// computed as if from dst's directory.
		if edit, ok := pathEdit(row, fileLine-1, ps, pe, dst, tgt); ok {
			changes[srcKey] = append(changes[srcKey], edit)
		}
	}
}

// pathEdit builds the Edit that replaces row[ps:pe] (a path token) with
// the token recomputed for refFile pointing at target, or ok=false when
// the recompute is a no-op.
func pathEdit(row []byte, line, ps, pe int, refFile, target string) (Edit, bool) {
	oldTok := string(row[ps:pe])
	newTok := recomputeToken(refFile, oldTok, target)
	if newTok == oldTok {
		return Edit{}, false
	}
	return Edit{
		Range: Range{
			Start: Position{Line: line, Character: mdtext.UTF16FromByteOffset(row, ps)},
			End:   Position{Line: line, Character: mdtext.UTF16FromByteOffset(row, pe)},
		},
		NewText: newTok,
	}, true
}

// linkPathBytesResolving returns the byte range of the path portion
// (before any `#`) of the first inline-link destination at or after
// textStart on row whose path resolves to want. It advances past
// destinations that resolve elsewhere, so an image-in-link like
// [![alt](img.png)](want.md) rewrites the outer path, not the inner
// image. Angle-bracketed `<dest>` forms are unwrapped.
func linkPathBytesResolving(row []byte, textStart int, refFile, want string) (int, int, bool) {
	searchFrom := textStart
	if searchFrom < 0 {
		searchFrom = 0
	}
	for {
		open, closeIdx, ok := destBounds(row, searchFrom)
		if !ok {
			return 0, 0, false
		}
		start, end := open, closeIdx
		if start < end && row[start] == '<' {
			start++
			for j := start; j < end; j++ {
				if row[j] == '>' {
					end = j
					break
				}
			}
		}
		for i := start; i < end; i++ {
			if row[i] == '#' {
				end = i
				break
			}
		}
		if start < end && linkgraph.ResolveRelTarget(refFile, string(row[start:end])) == want {
			return start, end, true
		}
		searchFrom = closeIdx + 1
	}
}

// appendRefDefPathEdits rewrites `[label]: src` reference-definition
// destinations across the workspace so the path resolves to dst. It
// mirrors the heading ref-def pass: every file is scanned through
// validRefDefMatches so def-shaped lines inside code blocks are left
// alone.
func appendRefDefPathEdits(changes map[string][]Edit, ws Workspace, src, dst string) {
	src = index.NormalizePath(src)
	for _, rel := range ws.Files() {
		key, source, ok := ws.Resolve(rel)
		if !ok {
			continue
		}
		body, fmOffset := bodyAndFMOffset(source)
		fileLines := splitLines(source)
		for _, m := range validRefDefMatches(body) {
			edit, ok := refDefPathEditForMatch(body, fileLines, fmOffset, m.matchIdx, rel, src, dst)
			if ok {
				changes[key] = append(changes[key], edit)
			}
		}
	}
}

// refDefPathEditForMatch turns one `[label]: url` match into an Edit on
// the URL's path portion when that path resolves to src, or ok=false
// otherwise. The fragment (if any) is preserved.
func refDefPathEditForMatch(
	body []byte, fileLines [][]byte, fmOffset int, m []int,
	defFile, src, dst string,
) (Edit, bool) {
	bodyLine := lineOfBodyOffset(body, m[2])
	fileLine := bodyLine + fmOffset
	if fileLine-1 >= len(fileLines) {
		return Edit{}, false
	}
	row := fileLines[fileLine-1]
	colonOff := refDefColonOffset(row)
	if colonOff < 0 {
		return Edit{}, false
	}
	destStart, destEnd := refDefDestRange(row, colonOff+1)
	if destStart >= destEnd {
		return Edit{}, false
	}
	pathEnd := destEnd
	for i := destStart; i < destEnd; i++ {
		if row[i] == '#' {
			pathEnd = i
			break
		}
	}
	if destStart >= pathEnd {
		return Edit{}, false
	}
	oldTok := string(row[destStart:pathEnd])
	if linkgraph.ResolveRelTarget(defFile, oldTok) != src {
		return Edit{}, false
	}
	// src != dst, so the recomputed path always differs from oldTok — no
	// no-op guard is needed here (unlike the outbound pass, whose links
	// can point at unrelated targets).
	newTok := recomputeToken(defFile, oldTok, dst)
	return Edit{
		Range: Range{
			Start: Position{Line: fileLine - 1, Character: mdtext.UTF16FromByteOffset(row, destStart)},
			End:   Position{Line: fileLine - 1, Character: mdtext.UTF16FromByteOffset(row, pathEnd)},
		},
		NewText: newTok,
	}, true
}

// appendWikilinkStemEdits rewrites `[[old-stem]]` links to the new
// basename stem, but only when the move changes the basename. A move
// that keeps the basename leaves wikilinks alone: a stem still resolves
// to the file at its new path.
func appendWikilinkStemEdits(changes map[string][]Edit, ws Workspace, src, dst string) {
	oldStem := fileStem(src)
	newStem := fileStem(dst)
	if oldStem == newStem {
		return
	}
	newSpelling := dstStemSpelling(dst)
	for _, e := range ws.IncomingWikilinkEdges(oldStem) {
		key, source, ok := ws.Resolve(e.SourceFile)
		if !ok {
			continue
		}
		lines := splitLines(source)
		if e.SourceLine < 1 || e.SourceLine > len(lines) {
			continue
		}
		row := lines[e.SourceLine-1]
		start, end, ok := wikilinkStemBytes(row, e.SourceCol-1)
		if !ok {
			continue
		}
		changes[key] = append(changes[key], Edit{
			Range: Range{
				Start: Position{Line: e.SourceLine - 1, Character: mdtext.UTF16FromByteOffset(row, start)},
				End:   Position{Line: e.SourceLine - 1, Character: mdtext.UTF16FromByteOffset(row, end)},
			},
			NewText: newSpelling,
		})
	}
}

// wikilinkStemBytes returns the byte range of the basename-stem token
// inside a `[[target#anchor|alias]]` link starting at bracketStart.
// Any folder prefix, anchor, and alias are preserved: only the stem
// after the last `/` and before `#` / `|` / `]]` is returned.
func wikilinkStemBytes(row []byte, bracketStart int) (int, int, bool) {
	i := bracketStart
	if i < 0 || i+1 >= len(row) || row[i] != '[' || row[i+1] != '[' {
		return 0, 0, false
	}
	start := i + 2
	end := start
	for end < len(row) {
		c := row[end]
		if c == '#' || c == '|' || c == ']' {
			break
		}
		end++
	}
	stemStart := start
	for j := start; j < end; j++ {
		if row[j] == '/' {
			stemStart = j + 1
		}
	}
	if stemStart >= end {
		return 0, 0, false
	}
	return stemStart, end, true
}

// fileStem returns the lowercased basename stem a file is addressed by
// as a wikilink target (matching linkgraph.WikilinkStem's lookup key).
func fileStem(p string) string {
	if stem, ok := linkgraph.WikilinkStem(path.Base(p)); ok {
		return stem
	}
	return strings.ToLower(path.Base(p))
}

// dstStemSpelling returns the basename stem of dst with its original
// casing, so a rewritten wikilink reads naturally (`[[Service]]`, not a
// lowercased match key). A Markdown extension is stripped; any other
// name is kept whole.
func dstStemSpelling(dst string) string {
	base := path.Base(dst)
	ext := path.Ext(base)
	if mdpath.HasMarkdownExt(ext) {
		return strings.TrimSuffix(base, ext)
	}
	return base
}
