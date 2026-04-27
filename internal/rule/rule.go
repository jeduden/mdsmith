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

// ListMergeMode is how a list-typed rule setting is merged across config
// layers (defaults, kinds, overrides). A setting that is a scalar or a
// nested map is unaffected by this value.
type ListMergeMode int

const (
	// ListReplace replaces the earlier list with the later list. This is
	// the default for any list setting whose rule does not declare otherwise.
	ListReplace ListMergeMode = iota
	// ListAppend concatenates the later list onto the earlier list,
	// preserving order. Used by settings like `placeholders:` where each
	// layer contributes additional opt-ins.
	ListAppend
)

// MergeModes is implemented by Configurable rules that want to opt one or
// more list settings into a non-default merge mode (typically `append`
// instead of the default `replace`). Keys not returned use the default.
type MergeModes interface {
	ListMergeModes() map[string]ListMergeMode
}

// Defaultable is implemented by rules that override the default enabled
// state in generated/runtime configs.
type Defaultable interface {
	EnabledByDefault() bool
}
