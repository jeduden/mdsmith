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

// Defaultable is implemented by rules that override the default enabled
// state in generated/runtime configs.
type Defaultable interface {
	EnabledByDefault() bool
}

// ListMergeMode controls how a list-valued setting is combined when
// later config layers (kinds or overrides) merge over earlier ones.
type ListMergeMode int

const (
	// ListReplace replaces the earlier list wholesale with the later
	// list. This is the default for any setting key the rule does not
	// explicitly mark as appendable.
	ListReplace ListMergeMode = iota
	// ListAppend concatenates the later list onto the earlier list.
	// Order is preserved across layers; duplicates are not removed.
	ListAppend
)

// ListMerger is implemented by Configurable rules that want to opt
// specific list-valued setting keys into append-merging across config
// layers. ListMergeMode is consulted by the deep-merge in
// internal/config; rules that do not implement ListMerger get the
// default replace behavior for every list key.
type ListMerger interface {
	// ListMergeMode returns the merge mode for the named setting key.
	// Unknown keys should return ListReplace.
	ListMergeMode(key string) ListMergeMode
}
