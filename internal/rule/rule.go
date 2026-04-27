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

// SettingsMergeMode is the merge mode for a single setting key. It mirrors
// config.MergeMode but lives in the rule package so rules can declare
// their merge intent without depending on the config package.
type SettingsMergeMode int

const (
	// MergeReplace — later layer's value replaces the earlier one
	// wholesale. This is the default for every setting.
	MergeReplace SettingsMergeMode = iota
	// MergeAppend — list-typed settings concatenate across layers.
	MergeAppend
)

// SettingsMerger is implemented by Configurable rules that want any of
// their list settings to merge by append rather than replace across the
// kind/override layer chain. Returning a nil or empty map means every
// setting follows the default replace semantics.
type SettingsMerger interface {
	SettingsMergeModes() map[string]SettingsMergeMode
}
