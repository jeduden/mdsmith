package metrics

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
)

// Row holds computed metric values for a single file.
type Row struct {
	Path    string
	Metrics map[string]Value
}

// Collect computes all selected metrics for each file path.
func Collect(paths []string, defs []Definition) ([]Row, error) {
	rows := make([]Row, 0, len(paths))
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %q: %w", path, err)
		}

		doc := NewDocument(path, source)
		values := make(map[string]Value, len(defs))
		for _, def := range defs {
			v, err := def.Compute(doc)
			if err != nil {
				return nil, fmt.Errorf("computing %q for %q: %w", def.Name, path, err)
			}
			values[def.Name] = v
		}

		rows = append(rows, Row{
			Path:    path,
			Metrics: values,
		})
	}
	return rows, nil
}

// SortRows sorts rows deterministically by a metric and path tiebreaker.
func SortRows(rows []Row, by Definition, order Order) {
	sort.Slice(rows, func(i, j int) bool {
		a := rows[i].Metrics[by.Name]
		b := rows[j].Metrics[by.Name]

		// Available values sort before unavailable values.
		if a.Available != b.Available {
			return a.Available
		}

		if a.Available && b.Available {
			diff := a.Number - b.Number
			if math.Abs(diff) > 1e-9 {
				if order == OrderAsc {
					return diff < 0
				}
				return diff > 0
			}
		}

		// Stable deterministic tie-break.
		return rows[i].Path < rows[j].Path
	})
}

// LimitRows returns at most top rows (if top > 0).
func LimitRows(rows []Row, top int) []Row {
	if top <= 0 || top >= len(rows) {
		return rows
	}
	return rows[:top]
}

// FormatValue renders a metric value for text output.
func FormatValue(def Definition, value Value) string {
	v := JSONValue(def, value)
	if v == nil {
		return "-"
	}

	switch n := v.(type) {
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return fmt.Sprintf("%.*f", def.Precision, n)
	default:
		return "-"
	}
}

// JSONValue converts a metric value into a JSON-safe scalar.
// Unavailable values return nil.
func JSONValue(def Definition, value Value) any {
	if !value.Available {
		return nil
	}

	switch def.Kind {
	case KindInteger:
		return int64(math.Round(value.Number))
	case KindFloat:
		if def.Precision < 0 {
			return value.Number
		}
		scale := math.Pow10(def.Precision)
		return math.Round(value.Number*scale) / scale
	default:
		return value.Number
	}
}
