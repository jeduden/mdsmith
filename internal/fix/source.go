package fix

import (
	"errors"
	"fmt"
	"io/fs"
	"math"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/setutil"
)

// SourceOptions configures an in-memory fix run. The fix functions
// (Source, SourceWithRules) do not touch disk; they reuse the
// same prep/apply machinery the path-based Fixer uses on a buffer.
type SourceOptions struct {
	Config           *config.Config
	Rules            []rule.Rule
	Path             string
	Source           []byte
	RootDir          string
	StripFrontMatter bool
	// MaxInputBytes caps Source size before any fix runs. Semantics
	// match bytelimit.ReadFileLimited: <= 0 or math.MaxInt64 means
	// unlimited; > 0 means reject buffers larger than that many
	// bytes. Callers that want the default 2 MB cap must pass
	// bytelimit.DefaultMaxInputBytes explicitly.
	MaxInputBytes int64
	// SourceFS, when non-nil, is the filesystem the fixable rules
	// (include/catalog/cross-file) see for the buffer. Callers that
	// pass a workspace-relative Path (for config glob matching) MUST
	// also supply a SourceFS rooted at the document's real
	// directory; otherwise the dirFS derived from the relative path
	// would be resolved against the process CWD, breaking
	// neighbour-file lookups when the editor is launched from
	// elsewhere.
	SourceFS fs.FS
}

// Source applies every fixable rule allowed by the effective
// config and returns the resulting bytes. The returned bytes equal
// the input when no rule produced an edit.
func Source(opts SourceOptions) ([]byte, error) {
	return fixSourceImpl(opts, nil)
}

// SourceWithRules is like Source but only the named rules are
// applied. An empty names slice produces no fixes.
func SourceWithRules(opts SourceOptions, names []string) ([]byte, error) {
	if len(names) == 0 {
		return opts.Source, nil
	}
	return fixSourceImpl(opts, names)
}

func fixSourceImpl(opts SourceOptions, only []string) ([]byte, error) {
	maxBytes := opts.MaxInputBytes
	// Mirror the on-disk cap that bytelimit.ReadFileLimited applies
	// during `mdsmith fix`. The same convention is used here so
	// callers who already resolved the project's max-input-size
	// (CLI, LSP) can pass the value through unchanged:
	//   - max <= 0           → unlimited (matches `max-input-size: 0`).
	//   - max == math.MaxInt64 → unlimited.
	//   - max > 0            → cap the buffer at that many bytes.
	// When the caller wants the default 2 MB cap, they must pass
	// bytelimit.DefaultMaxInputBytes explicitly (resolveMaxInputBytes
	// in cmd/mdsmith/main.go shows the canonical resolution).
	if maxBytes > 0 && maxBytes != math.MaxInt64 &&
		int64(len(opts.Source)) > maxBytes {
		// Match the on-disk Fixer's error shape — Fixer.Fix wraps
		// bytelimit.ReadFileLimited's "file too large" via
		// `reading %q: %w`, so editor / log output stays uniform
		// regardless of whether the source came from disk or an
		// in-memory caller (LSP, stdin, …).
		return nil, fmt.Errorf("reading %q: file too large (%d bytes, max %d)",
			opts.Path, len(opts.Source), maxBytes)
	}
	cfg := opts.Config
	if cfg == nil {
		// prepareFile dereferences Fixer.Config (via
		// config.ValidateFrontMatterKinds), so a nil Config would
		// panic. Treat absent config as defaults so callers can pass
		// a zero-value Options.
		cfg = config.Merge(config.Defaults(), nil)
	}
	f := &Fixer{
		Config:           cfg,
		Rules:            opts.Rules,
		StripFrontMatter: opts.StripFrontMatter,
		RootDir:          opts.RootDir,
		MaxInputBytes:    maxBytes,
		SourceFS:         opts.SourceFS,
	}
	lf, dirFS, fmKinds, fmFields, err := f.prepareFile(opts.Path, opts.Source)
	if err != nil {
		return nil, err
	}
	effective := f.effectiveWithCategories(opts.Path, fmKinds, fmFields)
	// Surface configuration errors (invalid rule settings, etc.)
	// instead of silently producing a fix that omits the affected
	// rules. Callers (LSP / `mdsmith fix`) decide how to render the
	// failure.
	fixable, settingsErrs := f.fixableRules(effective)
	if len(settingsErrs) > 0 {
		return nil, errors.Join(settingsErrs...)
	}
	if only != nil {
		set := setutil.FromStrings(only)
		filtered := fixable[:0]
		for _, r := range fixable {
			if _, ok := set[r.Name()]; ok {
				filtered = append(filtered, r)
			}
		}
		fixable = filtered
	}
	lf.GeneratedRanges = gensection.FindAllGeneratedRanges(lf)
	// applyFixPasses' error sink is unreachable today: the only
	// path that appends is `lint.NewFile`'s error return, and
	// NewFile is currently infallible. We pass a discarded sink
	// so the call site mirrors fix.go without carrying a dead
	// error pass-through.
	var sink []error
	fixed := f.applyFixPasses(opts.Path, lf.Source, fixable, lf, dirFS, &sink)
	_ = sink
	return lf.FullSource(fixed), nil
}
