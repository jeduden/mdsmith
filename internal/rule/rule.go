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

// QuickFixTitler is implemented by a FixableRule that wants a custom
// lightbulb label for its quick fix instead of the generic
// "Fix all <name> with mdsmith". The editor shows FixTitle() verbatim,
// so phrase it as the concrete edit the user gets (e.g. MDS012 returns
// "Wrap in angle brackets"). The edit itself is still whole-file scoped.
type QuickFixTitler interface {
	FixTitle() string
}

// DryRunPredictor is implemented by FixableRules whose Fix performs
// side effects (e.g. writing a sibling file, staging a git index
// entry) rather than rewriting the markdown source itself. During
// `mdsmith fix --dry-run`, Fix returns the markdown unchanged to
// honor the no-disk-writes contract, but the post-fix Check would
// still observe the unfixed drift. PredictDryRunFix lets the rule
// declare which diagnostics a real Fix would have cleared so the
// fix engine subtracts them from the remaining-diagnostic set,
// keeping the dry-run exit code matching a real run.
type DryRunPredictor interface {
	PredictDryRunFix(f *lint.File) []lint.Diagnostic
}

// GitIndexMutator is implemented by a rule whose Fix mutates the git
// index (e.g. an in-process `git add`). The merge driver excludes
// such rules: it runs inside `git merge`, which holds
// `.git/index.lock` for the whole merge, so a second index writer
// there races the lock (see cmd/mdsmith/mergedriver.go). MDS048
// (git-hook-sync) is the current implementer.
type GitIndexMutator interface {
	// MutatesGitIndex reports whether the rule's Fix writes the git
	// index.
	MutatesGitIndex() bool
}

// LineCapable is implemented by rules that can run entirely from the flat
// Layer-0 line classifier (lint.ClassifyLines) — they navigate no AST node
// tree, reading only f.Lines and the classifier-backed projections
// (lint.CollectCodeBlockLines, lint.FlatHeadingLines, and the byte-scan
// table helper). The engine's flat-Layer-0 parse-skip gate
// (Runner.FlatLayer0, plan 2606142147) skips the goldmark parse only when
// EVERY enabled markdown rule reports LineCapable() true, so a single
// non-line-capable rule keeps the whole run on the AST path. Prototype
// scope: line-length (MDS001) is the only implementer.
//
// A rule must report true only when it is line-capable under every
// settings combination it accepts: the gate consults the registered
// instance, before per-file settings are applied.
type LineCapable interface {
	LineCapable() bool
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

// MergeMode describes how a list-typed rule setting combines across
// config layers (defaults, kinds, overrides).
type MergeMode int

const (
	// MergeReplace is the default: a later layer's list replaces the
	// earlier layer's list wholesale.
	MergeReplace MergeMode = iota
	// MergeAppend concatenates a later layer's list onto the earlier
	// layer's list, preserving layer-chain order.
	MergeAppend
)

// ListMerger is implemented by Configurable rules that opt one or more
// list-typed settings out of the default MergeReplace behavior. The
// merge function calls SettingMergeMode(key) at config-resolution time
// and treats unknown keys as MergeReplace.
type ListMerger interface {
	SettingMergeMode(key string) MergeMode
}

// SettingsTranslator is implemented by Configurable rules that
// rewrite one config layer's settings map before the deep-merge
// runs. The config merge layer calls TranslateLayerSettings on
// every layer that configures the rule, so merge logic stays free
// of rule-name special cases.
//
// required-structure implements this to collapse the user-facing
// `schema:` / `inline-schema:` keys into an append-mode
// `schema-sources` list, letting multiple kinds compose their
// schemas instead of overwriting (plan 156).
type SettingsTranslator interface {
	// TranslateLayerSettings returns the settings the merge layer
	// should use for one layer. Implementations must treat the
	// input as read-only and return a new map when they change
	// anything; returning the input unchanged signals "no
	// translation applies".
	TranslateLayerSettings(settings map[string]any) map[string]any
}

// ConfigTarget is implemented by rules that validate the project
// config file (.mdsmith.yml) rather than individual Markdown files.
// The engine runner runs these rules once against a synthetic lint.File
// for the config file before per-file markdown processing; they return
// nil for all other file paths when configured in production mode.
type ConfigTarget interface {
	IsConfigFileRule() bool
}

// RepoScoped is implemented by rules whose diagnostics are anchored to
// a repository artifact rather than the linted host file. A rule is
// repo-scoped when its (File, Line, Column, RuleID, Message) diagnostic
// tuple is independent of the host file path — so two host files can
// emit the same tuple and DedupeDiagnostics collapses it.
//
// ConfigTarget rules are never repo-scoped: they run once via
// runConfigTargetRules rather than once per markdown file, so per-file
// duplicate tuples cannot occur.
//
// The engine uses this marker to skip the DedupeDiagnostics map+slice
// allocation when no enabled markdown rule is repo-scoped. Without the
// marker a blunt skip would silently drop diagnostics when repo-scoped
// rules are enabled.
type RepoScoped interface {
	RepoScopedDiagnostics() bool
}
