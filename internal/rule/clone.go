package rule

import "reflect"

// CloneRule creates a deep copy of a rule. If the rule implements
// Configurable, the clone is produced by creating a new zero-value
// instance and applying the original's DefaultSettings. Otherwise
// it falls back to a reflect-based shallow copy of the struct.
func CloneRule(r Rule) Rule {
	if c, ok := r.(Configurable); ok {
		// Create a new zero-value instance of the same concrete type.
		rv := reflect.ValueOf(r)
		if rv.Kind() == reflect.Ptr {
			newPtr := reflect.New(rv.Elem().Type())
			clone := newPtr.Interface().(Rule)
			if cc, ok := clone.(Configurable); ok {
				_ = cc.ApplySettings(c.DefaultSettings())
			}
			return clone
		}
	}

	// Fallback: reflect-based copy for non-Configurable rules.
	rv := reflect.ValueOf(r)
	if rv.Kind() == reflect.Ptr {
		newPtr := reflect.New(rv.Elem().Type())
		newPtr.Elem().Set(rv.Elem())
		return newPtr.Interface().(Rule)
	}

	// Value type — already a copy.
	return r
}

// CloneInstance returns an independent copy of r that preserves its
// identity and current state. Unlike CloneRule — which, for a
// Configurable rule, builds a zero value and applies DefaultSettings —
// CloneInstance is a faithful shallow copy of the same rule: every
// field, including a rule's struct-stored name/ID and any
// already-applied config, carries over, and the result is a distinct
// pointer. It exists so each Run worker can hold its own rule set:
// the per-file effective-config lookup is keyed by Name(), so a clone
// that zeroed Name() would silently skip the rule.
//
// An embedded sync.Mutex (or similar) is copied while unlocked —
// clones are taken from pristine, idle rule instances before any
// Check runs — so the copy is a valid, independent lock. The shallow
// copy shares slice/map backing with the source; that is safe because
// the engine clones again per file via ConfigureRule before applying
// per-file settings, and rules do not mutate their own config during
// Check.
func CloneInstance(r Rule) Rule {
	rv := reflect.ValueOf(r)
	if rv.Kind() != reflect.Ptr {
		// Value-type rule: the interface already holds a copy.
		return r
	}
	newPtr := reflect.New(rv.Elem().Type())
	newPtr.Elem().Set(rv.Elem())
	return newPtr.Interface().(Rule)
}
