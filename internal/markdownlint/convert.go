package markdownlint

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rules"
)

// Conversion is the result of converting a markdownlint config: the
// mdsmith rule entries whose behavior differs from mdsmith's defaults,
// plus notes about everything that did not translate. Rules the input
// never mentions are absent — they keep their mdsmith defaults.
type Conversion struct {
	Rules map[string]config.RuleCfg
	Notes []string
}

// target names one markdownlint rule covered by an mdsmith rule, read
// from the rule README front matter. upstreamDefault is markdownlint's
// own default-enabled state for that rule.
type target struct {
	rule            string
	mdID            string
	mdName          string
	upstreamDefault bool
}

// mappingIndex holds the markdownlint-to-mdsmith rule mapping in the
// three orientations the converter needs.
type mappingIndex struct {
	byID    map[string][]target
	byAlias map[string][]target
	ruleMDs map[string][]target
}

// listRules is a test seam for the embedded rule-metadata read;
// production always points at rules.ListRules.
var listRules = rules.ListRules

// buildIndex reads the embedded rule READMEs and indexes their
// `markdownlint:` front-matter mappings by MD id, by markdownlint rule
// name, and by mdsmith rule.
func buildIndex() (*mappingIndex, error) {
	all, err := listRules()
	if err != nil {
		return nil, fmt.Errorf("loading rule metadata: %w", err)
	}
	idx := &mappingIndex{
		byID:    map[string][]target{},
		byAlias: map[string][]target{},
		ruleMDs: map[string][]target{},
	}
	for _, info := range all {
		for _, m := range info.Markdownlint {
			t := target{
				rule:            info.Name,
				mdID:            strings.ToUpper(m.ID),
				mdName:          m.Name,
				upstreamDefault: m.Default,
			}
			idx.byID[t.mdID] = append(idx.byID[t.mdID], t)
			idx.byAlias[strings.ToLower(m.Name)] = append(idx.byAlias[strings.ToLower(m.Name)], t)
			idx.ruleMDs[t.rule] = append(idx.ruleMDs[t.rule], t)
		}
	}
	return idx, nil
}

// mdlTags lists markdownlint's rule-group tag names, so a tag toggle
// gets a precise note instead of a generic "unknown key" one.
var mdlTags = map[string]bool{
	"accessibility": true, "atx": true, "atx_closed": true,
	"blank_lines": true, "blockquote": true, "bullet": true,
	"code": true, "emphasis": true, "hard_tab": true, "headings": true,
	"hr": true, "html": true, "images": true, "indentation": true,
	"language": true, "line_length": true, "links": true, "ol": true,
	"spaces": true, "spelling": true, "table": true, "ul": true,
	"url": true, "whitespace": true,
}

// Convert maps a parsed markdownlint config onto mdsmith rule entries.
// Explicit rule keys translate through the mapping index and the option
// table; `default: false` additionally disables every unmentioned
// mdsmith rule that has a markdownlint analog and is on by default in
// mdsmith. Whatever cannot be translated is reported in Notes.
func Convert(raw map[string]any) (*Conversion, error) {
	idx, err := buildIndex()
	if err != nil {
		return nil, err
	}
	st := &convertState{
		idx:        idx,
		defaults:   config.DumpDefaults().Rules,
		states:     map[string]*ruleState{},
		disabledMD: map[string]bool{},
	}

	defaultOn := true
	if v, ok := raw["default"]; ok {
		if b, okB := v.(bool); okB {
			defaultOn = b
		} else {
			st.notef(`"default" must be true or false; ignored`)
		}
	}

	for _, key := range sortedKeys(raw) {
		switch key {
		case "default", "$schema":
			continue
		case "extends":
			st.notef(`"extends" is not followed; convert the extended config separately and merge by hand`)
			continue
		}
		st.applyKey(key, raw[key])
	}

	conv := &Conversion{Rules: map[string]config.RuleCfg{}}
	st.finalize(conv, defaultOn)
	return conv, nil
}

// ruleState accumulates what the input said about one mdsmith rule
// across all the markdownlint keys that map to it.
type ruleState struct {
	explicitOn bool
	offIDs     []string
	settings   map[string]any
}

// ensureSettings returns the settings map, allocating it on first use.
func (rs *ruleState) ensureSettings() map[string]any {
	if rs.settings == nil {
		rs.settings = map[string]any{}
	}
	return rs.settings
}

// convertState carries the working data of one Convert run.
type convertState struct {
	idx        *mappingIndex
	defaults   map[string]config.RuleCfg
	states     map[string]*ruleState
	disabledMD map[string]bool
	notes      []string
}

// notef appends a formatted note.
func (s *convertState) notef(format string, args ...any) {
	s.notes = append(s.notes, fmt.Sprintf(format, args...))
}

// state returns the accumulator for an mdsmith rule, creating it on
// first use.
func (s *convertState) state(rule string) *ruleState {
	rs, ok := s.states[rule]
	if !ok {
		rs = &ruleState{}
		s.states[rule] = rs
	}
	return rs
}

// resolve maps a markdownlint config key (MD id or rule-name alias,
// case-insensitive) to its mdsmith targets. Nil means unknown.
func (s *convertState) resolve(key string) []target {
	if ts, ok := s.idx.byID[strings.ToUpper(key)]; ok {
		return ts
	}
	return s.idx.byAlias[strings.ToLower(key)]
}

// applyKey folds one markdownlint config entry into the per-rule
// accumulators, or records a note when the key or value shape has no
// translation.
func (s *convertState) applyKey(key string, val any) {
	targets := s.resolve(key)
	if targets == nil {
		if mdlTags[strings.ToLower(key)] {
			s.notef(`"%s" is a markdownlint tag; tag toggles are not converted`+
				` — configure the individual mdsmith rules instead`, key)
		} else {
			s.notef(`"%s" has no mdsmith equivalent`, key)
		}
		return
	}
	switch v := val.(type) {
	case bool:
		for _, t := range targets {
			rs := s.state(t.rule)
			if v {
				rs.explicitOn = true
			} else {
				rs.offIDs = append(rs.offIDs, t.mdID)
				s.disabledMD[t.mdID] = true
			}
		}
	case map[string]any:
		for _, t := range targets {
			rs := s.state(t.rule)
			rs.explicitOn = true
			s.translateOptions(t, v, rs)
		}
	default:
		s.notef(`"%s" must be a boolean or an options mapping; ignored`, key)
	}
}

// translateOptions converts one markdownlint rule's options through the
// option table, noting every option that has no mdsmith setting.
func (s *convertState) translateOptions(t target, opts map[string]any, rs *ruleState) {
	table := optionTable[t.mdID]
	for _, name := range sortedKeys(opts) {
		spec, ok := table[name]
		if !ok {
			s.notef(`%s option "%s": no mdsmith equivalent`, t.mdID, name)
			continue
		}
		if reason := spec(opts[name], rs.ensureSettings()); reason != "" {
			s.notef(`%s option "%s": %s`, t.mdID, name, reason)
		}
	}
}

// finalize turns the accumulated per-rule states into emitted rule
// entries, applies the `default: false` sweep, and reports the rules
// markdownlint checks by default that mdsmith leaves opt-in.
func (s *convertState) finalize(conv *Conversion, defaultOn bool) {
	for _, name := range sortedKeys(s.states) {
		rs := s.states[name]
		on := s.effectiveOn(name, rs, defaultOn)
		mdsmithDefault := s.defaults[name].Enabled
		switch {
		case on && len(rs.settings) > 0:
			conv.Rules[name] = config.RuleCfg{Enabled: true, Settings: rs.settings}
		case on && !mdsmithDefault:
			conv.Rules[name] = config.RuleCfg{Enabled: true}
		case !on && mdsmithDefault:
			conv.Rules[name] = config.RuleCfg{Enabled: false}
		}
		if on && len(rs.offIDs) > 0 {
			sort.Strings(rs.offIDs)
			s.notef(`%s is disabled, but mdsmith's %s also covers %s and stays`+
				` enabled; disable it entirely with "%s: false"`,
				strings.Join(rs.offIDs, ", "), name,
				strings.Join(s.otherMDs(name, rs.offIDs), ", "), name)
		}
	}

	if !defaultOn {
		for _, name := range sortedKeys(s.idx.ruleMDs) {
			if _, seen := s.states[name]; seen {
				continue
			}
			if s.defaults[name].Enabled {
				conv.Rules[name] = config.RuleCfg{Enabled: false}
			}
		}
		s.notef(`markdownlint "default: false" disables every mdsmith rule with` +
			` a markdownlint analog; mdsmith-only rules keep their defaults`)
	} else if gaps := s.optInGaps(); len(gaps) > 0 {
		s.notef(`markdownlint enables these checks by default, but the mdsmith`+
			` analogs are opt-in and use mdsmith's own default settings —`+
			` review and enable each with "<rule>: true": %s`,
			strings.Join(gaps, ", "))
	}

	conv.Notes = s.notes
}

// effectiveOn reports whether an mdsmith rule's markdownlint analogs
// are collectively still on: explicitly enabled, or — for a rule
// covering several MD rules — any analog left at an on-by-default
// state the input did not disable.
func (s *convertState) effectiveOn(name string, rs *ruleState, defaultOn bool) bool {
	if rs.explicitOn {
		return true
	}
	for _, t := range s.idx.ruleMDs[name] {
		if s.disabledMD[t.mdID] {
			continue
		}
		if defaultOn && t.upstreamDefault {
			return true
		}
	}
	return false
}

// otherMDs returns the markdownlint ids mapped to a rule, minus the
// given ones, sorted.
func (s *convertState) otherMDs(name string, minus []string) []string {
	skip := map[string]bool{}
	for _, id := range minus {
		skip[id] = true
	}
	var out []string
	for _, t := range s.idx.ruleMDs[name] {
		if !skip[t.mdID] {
			out = append(out, t.mdID)
		}
	}
	sort.Strings(out)
	return out
}

// optInGaps lists "rule (MDxxx, …)" entries for every mdsmith rule the
// input never mentions that is opt-in in mdsmith while its markdownlint
// analog is on by default.
func (s *convertState) optInGaps() []string {
	var gaps []string
	for _, name := range sortedKeys(s.idx.ruleMDs) {
		if _, seen := s.states[name]; seen {
			continue
		}
		if s.defaults[name].Enabled {
			continue
		}
		var ids []string
		for _, t := range s.idx.ruleMDs[name] {
			if t.upstreamDefault {
				ids = append(ids, t.mdID)
			}
		}
		if len(ids) > 0 {
			sort.Strings(ids)
			gaps = append(gaps, fmt.Sprintf("%s (%s)", name, strings.Join(ids, ", ")))
		}
	}
	return gaps
}

// sortedKeys returns the map's keys in ascending order, for
// deterministic processing and notes.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
