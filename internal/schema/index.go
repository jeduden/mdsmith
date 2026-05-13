package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

func indexCacheKey(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(abs)
}

// recordIndexWriteError stores err keyed by the source file path,
// or clears the entry when err is nil.
func recordIndexWriteError(p string, err error) {
	key := indexCacheKey(p)
	indexWriteMu.Lock()
	defer indexWriteMu.Unlock()
	if err == nil {
		delete(indexWriteErr, key)
		return
	}
	indexWriteErr[key] = err
}

// lastIndexWriteError returns the last write error recorded for
// path, or nil if none.
func lastIndexWriteError(p string) error {
	key := indexCacheKey(p)
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

// WriteIndex writes the JSON index produced by BuildIndex next to
// the source file. Output paths are resolved relative to the source
// file's directory; absolute paths (including Windows drive-letter
// and leading-backslash forms), parent-traversal segments, and
// symlinks that escape the allowed root are rejected so a schema
// cannot trick fix into writing outside the project. Parent
// directories are created on demand so a nested `output:` path
// (e.g. `.mdsmith/index/runbook.json`) works on a clean checkout.
//
// The allowed root is f.RootDir when set (the project root), and
// the source file's directory otherwise. After mkdir we
// EvalSymlinks the parent directory and verify it still resolves
// inside that root, so a `sub` directory that turns out to be a
// symlink to `/etc` is caught before any bytes are written.
//
// On error WriteIndex records the failure in the package-level
// indexWriteErr cache keyed by f.Path so the next Check surfaces
// the underlying I/O error instead of repeating the generic
// "missing / out of date" message — otherwise a misconfiguration
// (e.g. `output: "."` resolving to a directory) would trap users
// in a fix loop with no signal about what is actually wrong.
// A successful write clears the entry.
func WriteIndex(f *lint.File, sch *Schema) error {
	target, data, err := resolveIndexWrite(f, sch)
	if err != nil {
		recordIndexWriteError(f.Path, err)
		return err
	}
	if data == nil {
		recordIndexWriteError(f.Path, nil)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		err = fmt.Errorf("schema.index: create parent dir: %w", err)
		recordIndexWriteError(f.Path, err)
		return err
	}
	if err := verifyIndexWithinRoot(f, target); err != nil {
		recordIndexWriteError(f.Path, err)
		return err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		recordIndexWriteError(f.Path, err)
		return err
	}
	recordIndexWriteError(f.Path, nil)
	return nil
}

// verifyIndexWithinRoot resolves the symlinks on target's parent
// directory and reports an error if the resolved path escapes the
// allowed root (f.RootDir when set, otherwise the source file's
// directory). The target file itself need not exist; only its
// parent must, and MkdirAll has just been called so it does. Hosts
// that cannot EvalSymlinks the parent (e.g. permission failures)
// fall back to a Clean-based comparison — this is best-effort
// rather than airtight, but still beats no check.
func verifyIndexWithinRoot(f *lint.File, target string) error {
	root := f.RootDir
	if root == "" {
		root = filepath.Dir(f.Path)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil // cannot enforce, do not block
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		resolvedRoot = filepath.Clean(absRoot)
	}
	absParent, err := filepath.Abs(filepath.Dir(target))
	if err != nil {
		return nil
	}
	resolvedParent, err := filepath.EvalSymlinks(absParent)
	if err != nil {
		resolvedParent = filepath.Clean(absParent)
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedParent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf(
			"schema.index.output resolves to %q which is outside the "+
				"allowed root %q (symlink escape rejected)",
			resolvedParent, resolvedRoot)
	}
	return nil
}

// resolveIndexWrite returns the absolute output path and the bytes
// that would be written for this file. data is nil when the schema
// declares no index. Path validation matches WriteIndex so both
// call sites surface the same errors.
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
	dir := filepath.Dir(f.Path)
	target := filepath.Clean(filepath.Join(dir, out))
	return target, data, nil
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

// ValidateIndex compares the on-disk index file (if any) against
// the bytes BuildIndex would emit. When the file is missing or its
// content differs, a diagnostic asks the user to run `mdsmith fix`
// so the artefact stays in sync. Comparison normalises CRLF line
// endings to LF so a Windows checkout with `core.autocrlf=true`
// does not flag a semantically-identical file as stale. Read
// errors other than "file does not exist" surface as a distinct
// diagnostic. If the last Fix tried to write this index and
// failed, the cached I/O error is reported in place of the generic
// "missing / out of date" message so users can act on the real
// cause instead of running fix again. `mdsmith check` still
// respects the read-only contract: it never touches the file.
func ValidateIndex(f *lint.File, sch *Schema, mkDiag MakeDiag) []lint.Diagnostic {
	target, want, err := resolveIndexWrite(f, sch)
	if err != nil {
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf("index: %v", err))}
	}
	if want == nil {
		// A schema that previously declared an index: but no longer
		// does should not leave stale write-error entries lying
		// around in a long-running process (notably the LSP server).
		recordIndexWriteError(f.Path, nil)
		return nil
	}
	if writeErr := lastIndexWriteError(f.Path); writeErr != nil {
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf(
				"index side-output %q write failed on the last `mdsmith fix`: %v",
				sch.Index.Output, writeErr))}
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return []lint.Diagnostic{mkDiag(f.Path, 1,
				fmt.Sprintf(
					"index side-output %q is missing; run `mdsmith fix`",
					sch.Index.Output))}
		}
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf(
				"index side-output %q cannot be read: %v",
				sch.Index.Output, readErr))}
	}
	if !indexContentEqual(got, want) {
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf(
				"index side-output %q is out of date; run `mdsmith fix`",
				sch.Index.Output))}
	}
	return nil
}

// indexContentEqual reports whether on-disk bytes a and freshly
// generated bytes b match, ignoring line-ending differences. A
// single trailing newline is also tolerated so a checkout that
// stripped or doubled a final newline is not flagged as drift.
func indexContentEqual(a, b []byte) bool {
	return bytes.Equal(normalizeIndexBytes(a), normalizeIndexBytes(b))
}

func normalizeIndexBytes(b []byte) []byte {
	// Strip CR characters so CRLF↔LF round-trips compare equal.
	out := bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	out = bytes.ReplaceAll(out, []byte("\r"), []byte("\n"))
	// Drop one trailing newline so the WriteFile-appended "\n" does
	// not diff against an in-memory build that already ended in one.
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	return out
}

// buildFlatHeadings returns every heading in document order with its
// level, plain text, slug, and 1-based line.
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
		line := 1
		if h.Lines().Len() > 0 {
			line = f.LineOfOffset(h.Lines().At(0).Start)
		}
		out = append(out, IndexHeading{
			Level: h.Level,
			Text:  text,
			Slug:  mdtext.Slugify(text),
			Line:  line,
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
		re, err := regexp.Compile(cr.Pattern)
		if err != nil {
			return nil, fmt.Errorf(
				"index cross-ref-graph: invalid pattern %q: %w",
				cr.Pattern, err)
		}
		var skipRE *regexp.Regexp
		if cr.SkipLinesMatching != "" {
			skipRE, err = regexp.Compile(cr.SkipLinesMatching)
			if err != nil {
				return nil, fmt.Errorf(
					"index cross-ref-graph: invalid skip-lines-matching %q: %w",
					cr.SkipLinesMatching, err)
			}
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
			count += len(strings.Fields(string(f.Lines[ln-1])))
		}
		out[h.Slug] = count
	}
	return out
}
