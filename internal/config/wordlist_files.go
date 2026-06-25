package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/wordlist"
)

// wordlistFilesDir is the directory under the workspace root that holds
// one YAML file per user-defined word-list. The basename (minus
// extension) is the list name. Both `*.yaml` and `*.yml` are scanned;
// subdirectories are rejected — one list per file, no nesting.
const wordlistFilesDir = ".mdsmith/wordlists"

// wordlistFileBasenameRE is the basename pattern a word-list file must
// match: a lowercase identifier usable as a YAML map key, anchored for
// OS case-folding and path safety. Inline `wordlists.<name>:` keys stay
// unvalidated — this constraint only applies to filenames.
var wordlistFileBasenameRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// discoveredWordlist pairs a parsed UserWordlist with the absolute path
// of the file it came from, for the dual-source collision check and
// provenance.
type discoveredWordlist struct {
	body       UserWordlist
	sourcePath string
}

// discoverWordlists walks `.mdsmith/wordlists/*.{yaml,yml}` at the
// workspace root and returns one entry per discovered list, keyed by
// basename. Mirrors discoverConventions: it rejects symlinks,
// subdirectories, a bad basename, and a `.yaml`/`.yml` collision, each
// error naming the offending file. A missing directory returns an empty
// map and no error.
func discoverWordlists(workspaceDir string) (map[string]discoveredWordlist, error) {
	root := filepath.Join(workspaceDir, wordlistFilesDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", wordlistFilesDir, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	result := make(map[string]discoveredWordlist, len(entries))
	seenExt := make(map[string]string, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf(
				"%s: symlinks are not allowed (found %q)", wordlistFilesDir, name)
		}
		if entry.IsDir() {
			return nil, fmt.Errorf(
				"%s: subdirectories are not allowed (found %q)", wordlistFilesDir, name)
		}
		ext := filepath.Ext(name)
		switch strings.ToLower(ext) {
		case ".yaml", ".yml":
		default:
			continue
		}
		base := name[:len(name)-len(ext)]
		if !wordlistFileBasenameRE.MatchString(base) {
			return nil, fmt.Errorf(
				"%s/%s: basename %q must match %s",
				wordlistFilesDir, name, base, wordlistFileBasenameRE.String())
		}
		if prior, ok := seenExt[base]; ok {
			return nil, fmt.Errorf(
				"%s: wordlist %q is declared by both %s and %s; keep one",
				wordlistFilesDir, base, prior, name)
		}
		seenExt[base] = name

		path := filepath.Join(root, name)
		body, err := parseWordlistFile(path)
		if err != nil {
			return nil, err
		}
		body.SourcePath = path
		result[base] = discoveredWordlist{body: body, sourcePath: path}
	}
	return result, nil
}

// parseWordlistFile reads one word-list file and decodes it via
// wordlist.Parse (strict YAML, anchors/aliases rejected) into a
// UserWordlist. The 1 MB readLimitedConfig cap matches every other
// config read.
func parseWordlistFile(path string) (UserWordlist, error) {
	data, err := readLimitedConfig(path)
	if err != nil {
		return UserWordlist{}, fmt.Errorf("reading %s: %w", path, err)
	}
	extends, entries, err := wordlist.Parse(data)
	if err != nil {
		return UserWordlist{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return UserWordlist{Extends: extends, Entries: entries}, nil
}

// mergeWordlistFiles tags every inline word-list with cfgPath for
// provenance, then discovers file-defined lists under the workspace
// root (parent of cfgPath) and merges them into cfg.Wordlists. Two
// collisions are config errors, each naming the offending file:
//
//   - a name reserved by a built-in list (ai-speak, ai-openers);
//   - a name declared both inline and in a file.
//
// Load always supplies a non-empty cfgPath.
func mergeWordlistFiles(cfg *Config, cfgPath string) error {
	for name, uw := range cfg.Wordlists {
		uw.SourcePath = cfgPath
		cfg.Wordlists[name] = uw
	}

	discovered, err := discoverWordlists(filepath.Dir(cfgPath))
	if err != nil {
		return err
	}
	if len(discovered) == 0 {
		return nil
	}

	reserved := make(map[string]bool, len(wordlist.BuiltinNames()))
	for _, name := range wordlist.BuiltinNames() {
		reserved[name] = true
	}

	if cfg.Wordlists == nil {
		cfg.Wordlists = make(map[string]UserWordlist, len(discovered))
	}
	names := make([]string, 0, len(discovered))
	for name := range discovered {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dw := discovered[name]
		if reserved[name] {
			return fmt.Errorf(
				"wordlist %q in %s: name is reserved by a built-in word-list",
				name, dw.sourcePath)
		}
		if existing, clash := cfg.Wordlists[name]; clash {
			return fmt.Errorf(
				"wordlist %q is declared both inline in %s and in %s; keep one source",
				name, existing.SourcePath, dw.sourcePath)
		}
		cfg.Wordlists[name] = dw.body
	}
	return nil
}

// toWordlistMap converts the config's user word-lists into the shape
// wordlist.Resolve consumes. Returns nil for an empty input.
func toWordlistMap(m map[string]UserWordlist) map[string]wordlist.Wordlist {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]wordlist.Wordlist, len(m))
	for name, uw := range m {
		out[name] = wordlist.Wordlist{
			Name:       name,
			Extends:    uw.Extends,
			Entries:    uw.Entries,
			SourcePath: uw.SourcePath,
		}
	}
	return out
}

// expandWordlists resolves each rule's `lists:` setting against the
// registry built from userLists, unions the resolved entries into the
// rule's WordlistTarget() setting (resolved entries first, then the
// rule's inline entries, de-duplicated), and removes the `lists:` key
// so rules never see it. A rule that is not a WordlistConsumer (or
// carries an empty `lists:`) just loses the key — validateWordlists
// reports the real error at config load, so resolution failures here
// are skipped rather than re-reported.
func expandWordlists(result map[string]RuleCfg, userLists map[string]UserWordlist) {
	if len(result) == 0 {
		return
	}
	var userMap map[string]wordlist.Wordlist
	for name, rc := range result {
		if _, ok := rc.Settings["lists"]; !ok {
			continue
		}
		newSettings := make(map[string]any, len(rc.Settings))
		for k, v := range rc.Settings {
			if k == "lists" {
				continue
			}
			newSettings[k] = v
		}
		if wc, ok := rule.ByName(name).(rule.WordlistConsumer); ok {
			if listNames, ok := anyToStrings(rc.Settings["lists"]); ok && len(listNames) > 0 {
				if userMap == nil {
					userMap = toWordlistMap(userLists)
				}
				target := wc.WordlistTarget()
				var resolved []string
				for _, ln := range listNames {
					entries, err := wordlist.Resolve(ln, userMap)
					if err != nil {
						continue
					}
					resolved = append(resolved, entries...)
				}
				existing, _ := anyToStrings(newSettings[target])
				newSettings[target] = stringsToAny(dedupStrings(append(resolved, existing...)))
			}
		}
		rc.Settings = newSettings
		result[name] = rc
	}
}

// validateWordlists walks every RuleCfg that could carry a `lists:`
// setting — top-level rules, the convention preset, kind bodies, and
// overrides — and rejects: a `lists:` that is not a list of strings; a
// `lists:` on an unknown or non-WordlistConsumer rule; and any named
// list that fails to resolve (unknown name, unknown parent, or
// `extends:` cycle). Called from loadFromBytes after applyConvention so
// the convention preset is populated.
func validateWordlists(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	userMap := toWordlistMap(cfg.Wordlists)
	check := func(scope string, rules map[string]RuleCfg) error {
		for ruleName, rc := range rules {
			raw, ok := rc.Settings["lists"]
			if !ok {
				continue
			}
			names, ok := anyToStrings(raw)
			if !ok {
				return fmt.Errorf("%s: rule %q: lists must be a list of strings", scope, ruleName)
			}
			if rule.ByName(ruleName) == nil {
				return fmt.Errorf("%s: rule %q: lists set on unknown rule", scope, ruleName)
			}
			if _, ok := rule.ByName(ruleName).(rule.WordlistConsumer); !ok {
				return fmt.Errorf("%s: rule %q does not accept lists", scope, ruleName)
			}
			for _, ln := range names {
				if _, err := wordlist.Resolve(ln, userMap); err != nil {
					return fmt.Errorf("%s: rule %q: %w", scope, ruleName, err)
				}
			}
		}
		return nil
	}
	if err := check("rules", cfg.Rules); err != nil {
		return err
	}
	if err := check("convention", cfg.ConventionPreset); err != nil {
		return err
	}
	kindNames := make([]string, 0, len(cfg.Kinds))
	for kn := range cfg.Kinds {
		kindNames = append(kindNames, kn)
	}
	sort.Strings(kindNames)
	for _, kn := range kindNames {
		if err := check("kind "+kn, cfg.Kinds[kn].Rules); err != nil {
			return err
		}
	}
	for i, o := range cfg.Overrides {
		if err := check(fmt.Sprintf("override %d", i), o.Rules); err != nil {
			return err
		}
	}
	return nil
}

// anyToStrings normalizes a settings value into a []string, accepting
// the []any YAML decode produces and a plain []string. Returns ok=false
// for a non-list or a list with a non-string element. A nil value is an
// empty list.
func anyToStrings(v any) ([]string, bool) {
	switch x := v.(type) {
	case nil:
		return nil, true
	case []string:
		return x, true
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

// stringsToAny widens a []string to the []any shape settings maps carry
// after YAML decode, so a rule's ApplySettings sees the same type
// whether the list came from YAML or from word-list expansion.
func stringsToAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// dedupStrings returns ss with later duplicates removed, preserving the
// first occurrence's position. Returns nil for an empty input.
func dedupStrings(ss []string) []string {
	if len(ss) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
