package metrics

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

var registry = []Definition{
	{
		ID:           "MET001",
		Name:         "bytes",
		Description:  "File size measured in bytes.",
		Scope:        ScopeFile,
		Kind:         KindInteger,
		Precision:    0,
		Default:      true,
		DefaultOrder: OrderDesc,
		Compute: func(doc *Document) (Value, error) {
			return AvailableValue(float64(doc.ByteCount())), nil
		},
	},
	{
		ID:           "MET002",
		Name:         "lines",
		Description:  "Total non-virtual line count.",
		Scope:        ScopeFile,
		Kind:         KindInteger,
		Precision:    0,
		Default:      true,
		DefaultOrder: OrderDesc,
		Compute: func(doc *Document) (Value, error) {
			return AvailableValue(float64(doc.LineCount())), nil
		},
	},
	{
		ID:           "MET003",
		Name:         "words",
		Description:  "Word count from extracted plain text.",
		Scope:        ScopeFile,
		Kind:         KindInteger,
		Precision:    0,
		Default:      true,
		DefaultOrder: OrderDesc,
		Compute: func(doc *Document) (Value, error) {
			words, err := doc.WordCount()
			if err != nil {
				return UnavailableValue(), err
			}
			return AvailableValue(float64(words)), nil
		},
	},
	{
		ID:           "MET004",
		Name:         "headings",
		Description:  "Heading count (#, ##, etc.).",
		Scope:        ScopeFile,
		Kind:         KindInteger,
		Precision:    0,
		Default:      true,
		DefaultOrder: OrderDesc,
		Compute: func(doc *Document) (Value, error) {
			headings, err := doc.HeadingCount()
			if err != nil {
				return UnavailableValue(), err
			}
			return AvailableValue(float64(headings)), nil
		},
	},
	{
		ID:           "MET005",
		Name:         "token-estimate",
		Description:  "Estimated token count using 0.75 tokens per word.",
		Scope:        ScopeFile,
		Kind:         KindInteger,
		Precision:    0,
		Default:      true,
		DefaultOrder: OrderDesc,
		Compute: func(doc *Document) (Value, error) {
			words, err := doc.WordCount()
			if err != nil {
				return UnavailableValue(), err
			}
			estimate := math.Round(float64(words) * 0.75)
			return AvailableValue(estimate), nil
		},
	},
	{
		ID:           "MET006",
		Name:         "conciseness",
		Description:  "Heuristic conciseness score (0-100, lower is less concise).",
		Scope:        ScopeFile,
		Kind:         KindFloat,
		Precision:    1,
		Default:      true,
		DefaultOrder: OrderAsc,
		Compute: func(doc *Document) (Value, error) {
			text, err := doc.PlainText()
			if err != nil {
				return UnavailableValue(), err
			}
			return AvailableValue(concisenessScore(text)), nil
		},
	},
}

// All returns all metrics sorted by ID.
func All() []Definition {
	defs := append([]Definition(nil), registry...)
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].ID < defs[j].ID
	})
	return defs
}

// ForScope returns all metrics for a scope, sorted by ID.
func ForScope(scope Scope) []Definition {
	all := All()
	defs := make([]Definition, 0, len(all))
	for _, def := range all {
		if def.Scope == scope {
			defs = append(defs, def)
		}
	}
	return defs
}

// Defaults returns default-selected metrics for a scope.
func Defaults(scope Scope) []Definition {
	defs := ForScope(scope)
	out := make([]Definition, 0, len(defs))
	for _, def := range defs {
		if def.Default {
			out = append(out, def)
		}
	}
	return out
}

// Lookup searches by metric ID (case-insensitive) or by name.
func Lookup(query string) (Definition, bool) {
	for _, def := range All() {
		if matches(def, query) {
			return def, true
		}
	}
	return Definition{}, false
}

// LookupScope searches by metric ID (case-insensitive) or name within scope.
func LookupScope(scope Scope, query string) (Definition, bool) {
	for _, def := range ForScope(scope) {
		if matches(def, query) {
			return def, true
		}
	}
	return Definition{}, false
}

// Resolve resolves user-selected metric names/IDs for a scope.
// Empty names returns default metrics.
func Resolve(scope Scope, names []string) ([]Definition, error) {
	if len(names) == 0 {
		return Defaults(scope), nil
	}

	seen := make(map[string]struct{}, len(names))
	defs := make([]Definition, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}

		def, ok := LookupScope(scope, name)
		if !ok {
			return nil, unknownMetricErr(scope, name)
		}

		if _, exists := seen[def.ID]; exists {
			continue
		}
		seen[def.ID] = struct{}{}
		defs = append(defs, def)
	}

	if len(defs) == 0 {
		return nil, fmt.Errorf("no metrics selected")
	}
	return defs, nil
}

// SplitList parses comma-separated metric names.
func SplitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func matches(def Definition, query string) bool {
	q := strings.TrimSpace(query)
	if q == "" {
		return false
	}
	return strings.EqualFold(def.ID, q) || def.Name == strings.ToLower(q)
}

func unknownMetricErr(scope Scope, name string) error {
	return fmt.Errorf(
		"unknown metric %q (available: %s)",
		name,
		strings.Join(availableNames(scope), ", "),
	)
}

func availableNames(scope Scope) []string {
	defs := ForScope(scope)
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	sort.Strings(names)
	return names
}
