package rule

import "github.com/jeduden/mdsmith/internal/lint"

// Rule is a single lint rule that checks a Markdown file.
type Rule interface {
	ID() string
	Name() string
	Category() string
	Check(f *lint.File) []lint.Diagnostic
}

// FixableRule is a Rule that can also auto-fix violations.
type FixableRule interface {
	Rule
	Fix(f *lint.File) []byte
}

// Configurable is implemented by rules that have user-tunable settings.
type Configurable interface {
	ApplySettings(settings map[string]any) error
	DefaultSettings() map[string]any
}

// MergeMode declares how a list-valued rule setting combines across
// config layers (defaults → kinds → overrides).
type MergeMode int

const (
	// MergeReplace replaces an earlier list with the later list. This
	// is the default for any list setting that does not opt in to
	// concatenation.
	MergeReplace MergeMode = iota

	// MergeAppend concatenates the later list onto the earlier list.
	// Used by settings like `placeholders:` where additive composition
	// across kinds is the natural behavior.
	MergeAppend
)

// ListMerger is implemented by rules whose list-valued settings need
// non-default merge behavior. The returned map keys are setting names
// (the same keys that appear in DefaultSettings) and values declare
// each setting's merge mode. Settings not present in the map merge as
// MergeReplace.
type ListMerger interface {
	MergeModes() map[string]MergeMode
}

// Defaultable is implemented by rules that override the default enabled
// state in generated/runtime configs.
type Defaultable interface {
	EnabledByDefault() bool
}
