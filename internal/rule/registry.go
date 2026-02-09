package rule

var registry []Rule

func Register(r Rule) {
	registry = append(registry, r)
}

func All() []Rule {
	result := make([]Rule, len(registry))
	copy(result, registry)
	return result
}

func ByID(id string) Rule {
	for _, r := range registry {
		if r.ID() == id {
			return r
		}
	}
	return nil
}

// Reset clears the registry. Used for testing.
func Reset() {
	registry = nil
}
