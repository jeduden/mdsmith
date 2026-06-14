package build

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/jeduden/mdsmith/internal/rules/buildpathutil"
)

// globCapFn is the CheckGlobMatchCap implementation; tests may replace it.
var globCapFn = buildpathutil.CheckGlobMatchCap

// hashFileFn is the hashFile implementation; tests may replace it.
var hashFileFn = hashFile

// Verdict is the staleness result for one target.
type Verdict int

const (
	// Fresh means the cached ActionID matches and every output exists with
	// the recorded content hash; the recipe is skipped.
	Fresh Verdict = iota
	// Stale means the target must be rebuilt.
	Stale
)

func (v Verdict) String() string {
	if v == Fresh {
		return "FRESH"
	}
	return "STALE"
}

// StalenessInput carries everything the staleness check needs for one
// target: the resolved Target plus the recipe's command string and the
// recipe's default-inputs already expanded to project-root-relative
// paths (param tokens resolved to the relative path the param supplies).
type StalenessInput struct {
	Target        Target
	Command       string
	DefaultInputs []string
}

// StalenessResult is the verdict plus the resolved relative input set,
// returned so the caller can report it.
type StalenessResult struct {
	Verdict Verdict
	Inputs  []string
}

// resolveInputs resolves the effective input set against the project
// root. Directive entries may be globs; default-inputs stay literal. The
// result is sorted, de-duplicated, project-root-relative paths. A
// non-glob entry that does not exist, or a glob matching zero files, is
// an error.
func resolveInputs(in StalenessInput) ([]string, error) {
	root := in.Target.Root
	fsys := os.DirFS(root)
	seen := make(map[string]struct{})
	var out []string

	add := func(rel string) {
		if _, dup := seen[rel]; !dup {
			seen[rel] = struct{}{}
			out = append(out, rel)
		}
	}

	for _, entry := range in.Target.Inputs {
		if isGlob(entry) {
			matches, err := doublestar.Glob(fsys, entry)
			if err != nil {
				return nil, fmt.Errorf("inputs glob %q: %w", entry, err)
			}
			if len(matches) == 0 {
				return nil, fmt.Errorf("inputs glob %q matched no files", entry)
			}
			if err := globCapFn(len(matches)); err != nil {
				return nil, err
			}
			for _, m := range matches {
				rel, err := buildpathutil.ResolvePathInRoot(root, m, true)
				if err != nil {
					return nil, err
				}
				add(rel)
			}
			continue
		}
		rel, err := buildpathutil.ResolvePathInRoot(root, entry, true)
		if err != nil {
			return nil, err
		}
		add(rel)
	}

	for _, entry := range in.DefaultInputs {
		rel, err := buildpathutil.ResolvePathInRoot(root, entry, true)
		if err != nil {
			return nil, err
		}
		add(rel)
	}

	sort.Strings(out)
	return out, nil
}

// resolveOutputs re-checks every declared output against the project
// root (outputs may not exist yet) and returns sorted relative paths.
func resolveOutputs(in StalenessInput) ([]string, error) {
	out := make([]string, 0, len(in.Target.Outputs))
	for _, entry := range in.Target.Outputs {
		rel, err := buildpathutil.ResolvePathInRoot(in.Target.Root, entry, false)
		if err != nil {
			return nil, err
		}
		if buildpathutil.UnderMdsmithDir(rel) {
			return nil, fmt.Errorf("output %q is under .mdsmith/; refusing to overwrite mdsmith state", rel)
		}
		out = append(out, rel)
	}
	sort.Strings(out)
	return out, nil
}

// frame writes a length-framed field to h: an 8-byte big-endian length
// prefix followed by the bytes. Two-layer framing on nested entries
// prevents collisions across distinct input sets.
func frame(h hash.Hash, b []byte) {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(b)))
	h.Write(lenBuf[:])
	h.Write(b)
}

// frameString frames a single inner string (length + bytes) into a
// builder for the canonical sub-fields that are themselves framed.
func frameString(b *strings.Builder, s string) {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(s)))
	b.Write(lenBuf[:])
	b.WriteString(s)
}

// canonicalParams returns the sorted, inner-framed serialization of the
// param map: per pair len(key)|key|len(value)|value, no separators.
func canonicalParams(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		frameString(&b, k)
		frameString(&b, params[k])
	}
	return b.String()
}

// canonicalPaths returns the inner-framed serialization of an already-
// sorted path slice: per entry len(path)|path.
func canonicalPaths(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		frameString(&b, p)
	}
	return b.String()
}

// computeActionIDFromSums hashes the ActionID from pre-resolved inputs,
// outputs, and already-computed per-input content sums. Separating the
// hashing from the I/O lets callers that already have the sums (e.g.
// Explain) avoid a second read pass over the input files.
func computeActionIDFromSums(in StalenessInput, inputs, outputs, sums []string) string {
	h := sha256.New()
	frame(h, []byte(in.Command))
	frame(h, []byte(canonicalParams(in.Target.Params)))
	frame(h, []byte(canonicalPaths(inputs)))

	var contents strings.Builder
	for _, sum := range sums {
		contents.WriteString(sum)
	}
	frame(h, []byte(contents.String()))

	frame(h, []byte(canonicalPaths(outputs)))

	var verBuf [8]byte
	binary.BigEndian.PutUint64(verBuf[:], uint64(CacheVersion))
	frame(h, verBuf[:])

	return "sha256-" + hex.EncodeToString(h.Sum(nil))
}

// computeActionIDFromResolved hashes the ActionID from pre-resolved inputs and
// outputs, so both ComputeActionID and RecordBuild can resolve paths once.
func computeActionIDFromResolved(in StalenessInput, inputs, outputs []string) (string, error) {
	sums := make([]string, len(inputs))
	for i, rel := range inputs {
		abs := filepath.Join(in.Target.Root, filepath.FromSlash(rel))
		sum, err := hashFileFn(abs)
		if err != nil {
			return "", err
		}
		sums[i] = sum
	}
	return computeActionIDFromSums(in, inputs, outputs, sums), nil
}

// ValidateInputs resolves the target's declared inputs and outputs without
// hashing them. It returns an error if any literal input path is missing, any
// input glob expands to zero files, or any declared input or output path
// escapes the project root. Use this before a forced rebuild (--build-force,
// --build-no-cache) to catch configuration errors cheaply, without the I/O
// cost of a full staleness check.
func ValidateInputs(in StalenessInput) error {
	if _, err := resolveInputs(in); err != nil {
		return err
	}
	_, err := resolveOutputs(in)
	return err
}

// ComputeActionID computes the sha256 ActionID over the recipe command,
// canonical params, sorted relative inputs, each input's content hash,
// sorted relative outputs, and the cache version. Every field is framed
// with an outer 8-byte big-endian length; nested keys, values, and paths
// are themselves framed. Returns "sha256-<64 lowercase hex>".
func ComputeActionID(in StalenessInput) (string, error) {
	inputs, err := resolveInputs(in)
	if err != nil {
		return "", err
	}
	outputs, err := resolveOutputs(in)
	if err != nil {
		return "", err
	}
	return computeActionIDFromResolved(in, inputs, outputs)
}

// ExplainInput is one resolved input path with its content sha, for the
// --build-explain breakdown.
type ExplainInput struct {
	Path string
	Hash string
}

// ActionExplanation is the full ActionID-input breakdown for one target: the
// recipe command, the canonical params, the resolved inputs with content
// shas, the resolved outputs, the cache version, and the resulting
// ActionID. It answers "why is this fresh?" without diving into JSON.
type ActionExplanation struct {
	Command      string
	Params       map[string]string
	Inputs       []ExplainInput
	Outputs      []string
	CacheVersion int
	ActionID     string
}

// Explain resolves a target's ActionID inputs and returns the breakdown.
func Explain(in StalenessInput) (ActionExplanation, error) {
	inputs, err := resolveInputs(in)
	if err != nil {
		return ActionExplanation{}, err
	}
	outputs, err := resolveOutputs(in)
	if err != nil {
		return ActionExplanation{}, err
	}
	sums := make([]string, len(inputs))
	exInputs := make([]ExplainInput, 0, len(inputs))
	for i, rel := range inputs {
		abs := filepath.Join(in.Target.Root, filepath.FromSlash(rel))
		sum, err := hashFileFn(abs)
		if err != nil {
			return ActionExplanation{}, err
		}
		sums[i] = sum
		exInputs = append(exInputs, ExplainInput{Path: rel, Hash: "sha256-" + sum})
	}
	return ActionExplanation{
		Command:      in.Command,
		Params:       in.Target.Params,
		Inputs:       exInputs,
		Outputs:      outputs,
		CacheVersion: CacheVersion,
		ActionID:     computeActionIDFromSums(in, inputs, outputs, sums),
	}, nil
}

// hashFile returns the lowercase-hex sha256 of a file's content. It
// streams through the shared hashFileSum primitive so large inputs never
// have to fit in memory.
func hashFile(abs string) (string, error) {
	sum, err := hashFileSum(abs)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum[:]), nil
}

// CheckStaleness returns the staleness verdict for one target against the
// cache. Steps, in order: resolve inputs (missing → error); any missing
// output → stale; compute ActionID; cache miss or different ActionID →
// stale; any output content hash mismatch → stale; otherwise fresh.
func CheckStaleness(in StalenessInput, cache *Cache) (StalenessResult, error) {
	inputs, err := resolveInputs(in)
	if err != nil {
		return StalenessResult{}, err
	}
	outputs, err := resolveOutputs(in)
	if err != nil {
		return StalenessResult{}, err
	}

	res := StalenessResult{Verdict: Stale, Inputs: inputs}

	// Step 2: any output missing on disk → stale.
	for _, rel := range outputs {
		abs := filepath.Join(in.Target.Root, filepath.FromSlash(rel))
		if _, err := os.Stat(abs); err != nil {
			return res, nil
		}
	}

	// Step 3-4: ActionID lookup.
	actionID, err := ComputeActionID(in)
	if err != nil {
		return StalenessResult{}, err
	}
	entry, ok := cache.Lookup(outputs)
	if !ok || entry.ActionID != actionID {
		return res, nil
	}

	// Step 5: output content hashes must match the cache.
	stored := make(map[string]string, len(entry.Outputs))
	for _, o := range entry.Outputs {
		stored[o.Path] = o.Hash
	}
	for _, rel := range outputs {
		abs := filepath.Join(in.Target.Root, filepath.FromSlash(rel))
		sum, err := hashFile(abs)
		if err != nil {
			return StalenessResult{}, err
		}
		if stored[rel] != "sha256-"+sum {
			return res, nil
		}
	}

	res.Verdict = Fresh
	return res, nil
}

// RecordBuild builds the cache entry for a freshly-built target: the
// ActionID, the sorted relative inputs, and each declared output hashed
// from disk. Call after a successful Build. built-at is left empty; the
// caller stamps it.
func RecordBuild(in StalenessInput) (CacheEntry, error) {
	inputs, err := resolveInputs(in)
	if err != nil {
		return CacheEntry{}, err
	}
	outputs, err := resolveOutputs(in)
	if err != nil {
		return CacheEntry{}, err
	}
	actionID, err := computeActionIDFromResolved(in, inputs, outputs)
	if err != nil {
		return CacheEntry{}, err
	}
	outHashes := make([]OutputHash, 0, len(outputs))
	for _, rel := range outputs {
		abs := filepath.Join(in.Target.Root, filepath.FromSlash(rel))
		sum, err := hashFile(abs)
		if err != nil {
			return CacheEntry{}, err
		}
		outHashes = append(outHashes, OutputHash{Path: rel, Hash: "sha256-" + sum})
	}
	return CacheEntry{
		Outputs:  outHashes,
		Inputs:   inputs,
		ActionID: actionID,
		Recipe:   in.Target.Recipe,
	}, nil
}

// OverlapTarget identifies one directive's declared outputs and its
// source location for the overlap report.
type OverlapTarget struct {
	File    string
	Line    int
	Outputs []string
}

// DetectOutputOverlap reports an error when two directives declare
// overlapping outputs — an exact path collision or a directory-prefix
// collision (book/ vs book/index.html). The error names both source
// locations. Targets are compared pairwise in source order.
func DetectOutputOverlap(targets []OverlapTarget) error {
	type owned struct {
		file string
		line int
		raw  string
	}
	var claimed []owned
	for _, t := range targets {
		for _, o := range t.Outputs {
			norm := normalizeOutput(o)
			for _, c := range claimed {
				if outputsOverlap(norm, normalizeOutput(c.raw)) {
					return fmt.Errorf(
						"build outputs overlap: %q (%s:%d) and %q (%s:%d) write to the same location",
						c.raw, c.file, c.line, o, t.File, t.Line,
					)
				}
			}
			claimed = append(claimed, owned{file: t.File, line: t.Line, raw: o})
		}
	}
	return nil
}

// normalizeOutput cleans an output path for comparison: forward slashes,
// path.Clean, trailing slash stripped.
func normalizeOutput(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "."
	}
	return path.Clean(p)
}

// outputsOverlap reports whether two normalized output paths collide:
// equal, or one is a directory prefix of the other.
func outputsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	return isPrefixDir(a, b) || isPrefixDir(b, a)
}

// isPrefixDir reports whether dir is a directory prefix of p (p lives
// under dir). "book" is a prefix dir of "book/index.html"; "book" is not
// a prefix dir of "bookmark".
func isPrefixDir(dir, p string) bool {
	return strings.HasPrefix(p, dir+"/")
}
