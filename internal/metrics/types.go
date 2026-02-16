package metrics

import (
	"fmt"
	"strings"
)

// Scope defines the entity level a metric applies to.
type Scope string

const (
	// ScopeFile indicates a file-level metric.
	ScopeFile Scope = "file"
)

// ParseScope parses a user-provided scope value.
func ParseScope(raw string) (Scope, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(ScopeFile):
		return ScopeFile, nil
	default:
		return "", fmt.Errorf("unknown scope %q (supported: file)", raw)
	}
}

// Order defines metric sort order.
type Order string

const (
	// OrderAsc sorts from smallest to largest.
	OrderAsc Order = "asc"
	// OrderDesc sorts from largest to smallest.
	OrderDesc Order = "desc"
)

// ParseOrder parses a user-provided sort order.
func ParseOrder(raw string) (Order, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(OrderDesc):
		return OrderDesc, nil
	case string(OrderAsc):
		return OrderAsc, nil
	default:
		return "", fmt.Errorf("unknown order %q (supported: asc, desc)", raw)
	}
}

// ValueKind describes how to render a numeric metric value.
type ValueKind string

const (
	// KindInteger renders values as rounded integers.
	KindInteger ValueKind = "integer"
	// KindFloat renders values with fixed decimal precision.
	KindFloat ValueKind = "float"
)

// Value is a computed numeric metric value.
type Value struct {
	Number    float64
	Available bool
}

// AvailableValue constructs an available metric value.
func AvailableValue(n float64) Value {
	return Value{
		Number:    n,
		Available: true,
	}
}

// UnavailableValue constructs an unavailable metric value.
func UnavailableValue() Value {
	return Value{}
}

// Definition describes a metric and how to compute it.
type Definition struct {
	ID           string
	Name         string
	Description  string
	Scope        Scope
	Kind         ValueKind
	Precision    int
	Default      bool
	DefaultOrder Order
	Compute      func(doc *Document) (Value, error)
}
