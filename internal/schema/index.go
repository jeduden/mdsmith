package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/yuin/goldmark/ast"
)

// indexWriteErrors records the last WriteIndex failure per source
// file path. When Fix swallows an I/O error from os.WriteFile (so
// `mdsmith fix` does not crash on a permissions glitch or "is a
// directory" misconfiguration), the next Check reads from this map
// and surfaces the underlying error instead of repeating the
// generic "missing/out of date" diagnostic — without it the user
// would be trapped in a fix loop with no signal about why the file
// is not being written. A successful WriteIndex clears the entry.
//
// Keys are normalised via filepath.Abs + filepath.Clean so a Check
// call with a relative `f.Path` and a Fix call with the absolute
// form agree on the same map entry. The fallback (when Abs fails,
// e.g. process cwd is unreadable) is the cleaned input; pairing
// that with the same fallback on the reader side keeps the
// behaviour symmetric.
var (
	indexWriteMu  sync.Mutex
	indexWriteErr = make(map[string]error)
)

func indexCacheKey(f *lint.File) string {
	// Anchor the key on f.RootDir + f.Path when RootDir is set
	// (engine and LSP pass workspace-relative paths), otherwise
	// fall back to filepath.Abs of the path as-is. filepath.Abs
	// only fails when the process has no working directory, which
	// is not a recoverable state; the empty fallback keeps the key
	// deterministic instead of branching on an unreachable error.
	src := f.Path
	if f.RootDir != "" && !filepath.IsAbs(f.Path) {
		src = filepath.Join(f.RootDir, f.Path)
	}
	abs, _ := filepath.Abs(src)
	if abs == "" {
		abs = src
	}
	return filepath.Clean(abs)
}

// recordIndexWriteError stores err keyed by the source file path,
// or clears the entry when err is nil.
func recordIndexWriteError(f *lint.File, err error) {
	key := indexCacheKey(f)
	indexWriteMu.Lock()
	defer indexWriteMu.Unlock()
	if err == nil {
		delete(indexWriteErr, key)
		return
	}
	indexWriteErr[key] = err
}

// lastIndexWriteError returns the last write error recorded for f,
// or nil if none.
func lastIndexWriteError(f *lint.File) error {
	key := indexCacheKey(f)
	indexWriteMu.Lock()
	defer indexWriteMu.Unlock()
	return indexWriteErr[key]
}

// IndexHeading is one entry in the flat heading list emitted by the
// "headings" include.
type IndexHeading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	Slug  string `json:"slug"`
	Line  int    `json:"line"`
}

// BuildIndex computes the JSON index document the IndexSpec asks for
// and returns its serialised bytes. The returned bytes are
// pretty-printed with two-space indentation so the file is reviewable
// when diffed. Errors from sub-builders (currently only
// `cross-ref-graph` can fail, on a bad regex) propagate so
// ValidateIndex / Fix surface them as diagnostics instead of
// silently shipping a partial index.
func BuildIndex(f *lint.File, sch *Schema) ([]byte, error) {
	if sch == nil || sch.Index == nil {
		return nil, nil
	}
	doc := map[string]any{}
	for _, key := range sch.Index.Include {
		switch key {
		case IndexIncludeStepMap:
			doc[key] = buildStepMap(f)
		case IndexIncludeCrossRefs:
			graph, err := buildCrossRefGraph(f, sch)
			if err != nil {
				return nil, err
			}
			doc[key] = graph
		case IndexIncludeWordCounts:
			doc[key] = buildWordCounts(f)
		case IndexIncludeHeadingsFlat:
			doc[key] = buildFlatHeadings(f)
		default:
			return nil, fmt.Errorf("schema.index.include: unknown entry %q", key)
		}
	}
	return json.MarshalIndent(doc, "", "  ")
}

// WriteIndex, ValidateIndex, and their OS-filesystem helpers live in
// index_native.go (native) and index_wasm.go (no-op stubs). They
// read and write the `<?index?>` sidecar file on the OS disk, which a
// WASM host (in-memory MemWorkspace, and a tinygo runtime that omits
// os.Chmod) cannot do. See the doc comments there.

// resolveIndexWrite returns the absolute output path and the bytes
// that would be written for this file. data is nil when the schema
// declares no index. Path validation matches WriteIndex so both
// call sites surface the same errors.
//
// The output path is resolved against the source file's on-disk
// directory: when f.RootDir is set (the engine and LSP both pass
// workspace-relative paths and an explicit RootDir), the source
// directory is computed from filepath.Join(f.RootDir, f.Path) so
// the index target lands next to the actual document instead of
// next to wherever the process CWD happens to point. Without this
// anchor, a workspace-relative f.Path would make every read/write
// drift with the user's cwd and produce spurious "missing / out
// of date" diagnostics.
func resolveIndexWrite(f *lint.File, sch *Schema) (string, []byte, error) {
	if sch == nil || sch.Index == nil {
		return "", nil, nil
	}
	out := sch.Index.Output
	if err := validateOutputPath(out); err != nil {
		return "", nil, err
	}
	data, err := BuildIndex(f, sch)
	if err != nil {
		return "", nil, err
	}
	data = append(data, '\n')
	target := filepath.Clean(filepath.Join(sourceDir(f), out))
	return target, data, nil
}

// sourceDir returns the directory the index target should be
// anchored to. When f.RootDir is set and f.Path is relative, we
// join them so the directory tracks the document's real on-disk
// location regardless of process CWD. An absolute f.Path or a
// blank RootDir falls through to filepath.Dir(f.Path), preserving
// the previous behavior for callers that don't supply RootDir.
func sourceDir(f *lint.File) string {
	if f.RootDir != "" && !filepath.IsAbs(f.Path) {
		return filepath.Dir(filepath.Join(f.RootDir, f.Path))
	}
	return filepath.Dir(f.Path)
}

// validateOutputPath rejects any output: value that would not be a
// project-relative POSIX-style path. Checks: host-absolute (POSIX),
// Windows-absolute (leading "\\", drive letter), embedded
// backslashes (which would otherwise become literal '\' characters
// in the filename on non-Windows hosts), any ".." segment, and
// paths that clean to "." (empty, ".", "./", trailing slash, etc.)
// since those resolve to the source directory and would cause
// WriteIndex to fail with "is a directory". The drive-letter check
// is host-independent so the rejection is consistent across OSes —
// filepath.IsAbs on a Linux host considers `C:\foo` relative,
// which would slip past a naive IsAbs guard.
func validateOutputPath(out string) error {
	if strings.TrimSpace(out) == "" {
		return fmt.Errorf("schema.index.output must not be empty")
	}
	if filepath.IsAbs(out) ||
		strings.HasPrefix(out, `\`) ||
		hasDriveLetterPrefix(out) {
		return fmt.Errorf("schema.index.output %q must be relative", out)
	}
	if strings.ContainsRune(out, '\\') {
		return fmt.Errorf(
			"schema.index.output %q must use POSIX-style \"/\" "+
				"separators; backslashes are rejected so the same "+
				"path resolves identically across operating systems",
			out)
	}
	slash := filepath.ToSlash(out)
	for _, elem := range strings.Split(slash, "/") {
		if elem == ".." {
			return fmt.Errorf(
				"schema.index.output %q must not contain \"..\" traversal", out)
		}
	}
	// filepath.Clean reduces "./", "foo/.", "foo/", and trailing
	// separators. A cleaned value of "." (or the empty string after
	// trimming) means the user pointed output at the source
	// directory, which is not a writable target.
	cleaned := filepath.Clean(out)
	if cleaned == "." || cleaned == "" {
		return fmt.Errorf(
			"schema.index.output %q must name a file, not the "+
				"source directory", out)
	}
	if strings.HasSuffix(slash, "/") {
		return fmt.Errorf(
			"schema.index.output %q must not end with a separator", out)
	}
	return nil
}

// hasDriveLetterPrefix reports whether p begins with a Windows
// drive letter (e.g. `C:` or `C:\`). Host-independent so the same
// rejection fires on Linux CI as on a Windows developer machine.
func hasDriveLetterPrefix(p string) bool {
	return len(p) >= 2 && p[1] == ':' &&
		((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z'))
}

// indexContentEqual reports whether on-disk bytes a and freshly
// generated bytes b match, ignoring line-ending differences and
// trailing-newline drift. The latter covers checkouts that
// stripped or doubled the final newline (editor or git settings).
func indexContentEqual(a, b []byte) bool {
	return bytes.Equal(normalizeIndexBytes(a), normalizeIndexBytes(b))
}

func normalizeIndexBytes(b []byte) []byte {
	// Strip CR characters so CRLF↔LF round-trips compare equal.
	out := bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	out = bytes.ReplaceAll(out, []byte("\r"), []byte("\n"))
	// Drop every trailing newline so stripped or doubled final
	// newlines compare equal to the WriteFile-appended canonical
	// form.
	return bytes.TrimRight(out, "\n")
}

// buildFlatHeadings returns every heading in document order with
// its level, plain text, slug, and 1-based line. Line numbers go
// through the shared headingLine helper so headings whose
// Lines() slice is empty (Goldmark produces this for some ATX
// forms) still report a meaningful position via their first
// descendant text node, matching the validator's behaviour and
// keeping word-count ranges consistent with the index's line
// fields.
func buildFlatHeadings(f *lint.File) []IndexHeading {
	var out []IndexHeading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		text := mdtext.ExtractPlainText(h, f.Source)
		out = append(out, IndexHeading{
			Level: h.Level,
			Text:  text,
			Slug:  mdtext.Slugify(text),
			Line:  headingLine(h, f),
		})
		return ast.WalkContinue, nil
	})
	if out == nil {
		out = []IndexHeading{}
	}
	return out
}

// buildStepMap returns a map of section slug → list of immediate
// child slugs. The map is keyed by the parent's slug for stable JSON
// output regardless of doc order.
func buildStepMap(f *lint.File) map[string][]string {
	heads := buildFlatHeadings(f)
	out := map[string][]string{}
	// Use a stack of (slug, level) for the current path.
	type frame struct {
		slug  string
		level int
	}
	var stack []frame
	for _, h := range heads {
		for len(stack) > 0 && stack[len(stack)-1].level >= h.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			parent := stack[len(stack)-1].slug
			out[parent] = append(out[parent], h.Slug)
		}
		stack = append(stack, frame{slug: h.Slug, level: h.Level})
	}
	return out
}

// buildCrossRefGraph maps each cross-reference match found in the
// document to the slug derived from its `must-match:` template,
// without consulting the document's heading slug set. Downstream
// tools see the full reference graph regardless of whether
// ValidateCrossReferences emitted an unresolved-reference
// diagnostic for any individual entry — diagnostic emission is the
// validator's job; the index is purely descriptive.
//
// Schema-level misconfigurations (a bad `pattern:` or
// `skip-lines-matching:` regex) are propagated as errors instead of
// silently swallowed: a partial index would let `mdsmith fix`
// report success while shipping data the user did not ask for.
// Template-fill failures on individual matches are kept silent —
// they're per-occurrence and ValidateCrossReferences already
// surfaces them.
func buildCrossRefGraph(f *lint.File, sch *Schema) (map[string]string, error) {
	out := map[string]string{}
	if len(sch.CrossReferences) == 0 {
		return out, nil
	}
	texts := collectTextNodes(f)
	for _, cr := range sch.CrossReferences {
		re, err := cr.compilePattern()
		if err != nil {
			return nil, fmt.Errorf(
				"index cross-ref-graph: invalid pattern %q: %w",
				cr.Pattern, err)
		}
		skipRE, err := cr.compileSkip()
		if err != nil {
			return nil, fmt.Errorf(
				"index cross-ref-graph: invalid skip-lines-matching %q: %w",
				cr.SkipLinesMatching, err)
		}
		groupNames := re.SubexpNames()
		for _, tn := range texts {
			if skipRE != nil && lineMatches(f, tn.Line, skipRE) {
				continue
			}
			for _, m := range re.FindAllStringSubmatch(tn.Text, -1) {
				target, err := fillTemplate(cr.MustMatch, m, groupNames)
				if err != nil {
					continue
				}
				out[m[0]] = mdtext.Slugify(target)
			}
		}
	}
	return out, nil
}

// buildWordCounts maps each heading slug to the word count of the
// body text immediately under that heading — up to but excluding
// the next heading at any level. Sub-section text is attributed to
// that subsection's slug, not the parent's, so summing along the
// step-map child list gives the recursive total when callers want
// it.
func buildWordCounts(f *lint.File) map[string]int {
	heads := buildFlatHeadings(f)
	out := map[string]int{}
	for i, h := range heads {
		startLine := h.Line + 1
		endLine := len(f.Lines) + 1
		if i+1 < len(heads) {
			endLine = heads[i+1].Line
		}
		count := 0
		for ln := startLine; ln < endLine && ln-1 < len(f.Lines); ln++ {
			count += len(bytes.Fields(f.Lines[ln-1]))
		}
		out[h.Slug] = count
	}
	return out
}
