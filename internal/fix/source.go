package fix

import (
	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
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
	MaxInputBytes    int64
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
	if maxBytes <= 0 {
		maxBytes = lint.DefaultMaxInputBytes
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
	}
	lf, dirFS, fmKinds, err := f.prepareFile(opts.Path, opts.Source)
	if err != nil {
		return nil, err
	}
	effective := f.effectiveWithCategories(opts.Path, fmKinds)
	fixable, _ := f.fixableRules(effective)
	if only != nil {
		set := make(map[string]struct{}, len(only))
		for _, n := range only {
			set[n] = struct{}{}
		}
		filtered := fixable[:0]
		for _, r := range fixable {
			if _, ok := set[r.Name()]; ok {
				filtered = append(filtered, r)
			}
		}
		fixable = filtered
	}
	lf.GeneratedRanges = gensection.FindAllGeneratedRanges(lf)
	var errs []error
	fixed := f.applyFixPasses(opts.Path, lf.Source, fixable, lf, dirFS, &errs)
	if len(errs) > 0 {
		return nil, errs[0]
	}
	return lf.FullSource(fixed), nil
}
