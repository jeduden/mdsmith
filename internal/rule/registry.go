package rule

var registry []Rule

// Register adds a rule to the global registry.
func Register(r Rule) {
	registry = append(registry, r)
}

// All returns a copy of all registered rules.
func All() []Rule {
	result := make([]Rule, len(registry))
	copy(result, registry)
	return result
}

// ByID returns the registered rule with the given ID, or nil.
func ByID(id string) Rule {
	for _, r := range registry {
		if r.ID() == id {
			return r
		}
	}
	return nil
}

// ByName returns the registered rule with the given Name, or nil.
func ByName(name string) Rule {
	for _, r := range registry {
		if r.Name() == name {
			return r
		}
	}
	return nil
}

// Reset clears the registry. Used for testing.
func Reset() {
	registry = nil
}
