package metrics

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/bytelimit"
)

// Row holds computed metric values for a single file.
type Row struct {
	Path    string
	Metrics map[string]Value
}

// Collect computes all selected metrics for each file path.
// maxBytes limits the file size that will be read; zero or negative means unlimited.
func Collect(paths []string, defs []Definition, maxBytes int64) ([]Row, error) {
	rows := make([]Row, 0, len(paths))
	for _, path := range paths {
		source, err := bytelimit.ReadFileLimited(path, maxBytes)
		if err != nil {
			return nil, fmt.Errorf("reading %q: %w", path, err)
		}

		doc := NewDocument(path, gensection.AuthoredSource(source))
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
	// Look the metric up once per row, then sort concrete values with
	// slices.SortFunc: no reflect.Swapper, no map lookup per comparison.
	type keyed struct {
		row Row
		val Value
	}
	items := make([]keyed, len(rows))
	for i := range rows {
		items[i] = keyed{row: rows[i], val: rows[i].Metrics[by.Name]}
	}
	slices.SortFunc(items, func(x, y keyed) int {
		a, b := x.val, y.val

		// Available values sort before unavailable values.
		if a.Available != b.Available {
			if a.Available {
				return -1
			}
			return 1
		}

		if a.Available && b.Available {
			diff := a.Number - b.Number
			if math.Abs(diff) > 1e-9 {
				if (order == OrderAsc) == (diff < 0) {
					return -1
				}
				return 1
			}
		}

		// Stable deterministic tie-break.
		return strings.Compare(x.row.Path, y.row.Path)
	})
	for i := range items {
		rows[i] = items[i].row
	}
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
