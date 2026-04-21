// Package settings provides shared helpers for coercing
// rule-configuration values decoded from YAML/CUE into Go types.
//
// Rule packages receive untyped settings maps (e.g. from
// `.mdsmith.yml` overrides). YAML numbers may decode as int,
// float64, or int64 depending on source; YAML sequences decode as
// []any. These helpers give every rule one coercion implementation
// with consistent behavior and a single set of tests.
package settings

import "math"

// ToInt coerces v to an int when v is int, int64, or float64.
// Float inputs are truncated toward zero. NaN, +/-Inf, and float
// values outside the int range are rejected (ok=false).
func ToInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		if n < math.MinInt || n > math.MaxInt {
			return 0, false
		}
		return int(n), true
	}
	return 0, false
}

// ToFloat coerces v to a float64 when v is float64, int, or int64.
// NaN and +/-Inf are rejected. Other types are rejected.
func ToFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// ToStringSlice coerces v to a []string when v is []string or a
// []any containing only strings. The returned slice is always a
// fresh copy so callers can mutate it without affecting the input.
// Other types (including strings) are rejected.
func ToStringSlice(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		out := make([]string, len(s))
		copy(out, s)
		return out, true
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, str)
		}
		return out, true
	}
	return nil, false
}
