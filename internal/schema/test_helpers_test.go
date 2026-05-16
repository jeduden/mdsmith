package schema

// Test helpers for constructing Scope and Matcher values without
// going through the parser. Used by unit tests that exercise the
// validator's branches directly.

// literalScope returns a required-once scope whose Matcher.Regex
// is the input text verbatim. Test inputs use plain alphanumeric
// heading text where regex metacharacters and the source string
// coincide; tests that need a real regex should construct the
// `Scope{Matcher: &Matcher{...}}` literal directly so the intent
// is obvious at the call site.
func literalScope(text string) Scope {
	return Scope{
		Heading: text,
		Matcher: &Matcher{Regex: text},
	}
}

// optionalScope is the literal scope counterpart with
// `repeat: { min: 0, max: 1 }` — an optional once-or-not match.
func optionalScope(text string) Scope {
	return Scope{
		Heading: text,
		Matcher: &Matcher{
			Regex:  text,
			Repeat: Repeat{Set: true, Min: 0, Max: 1},
		},
	}
}

// slotScope returns the canonical wildcard-slot scope —
// `regex: '.+', repeat: { min: 0 }`.
func slotScope() Scope {
	return Scope{
		Matcher: &Matcher{
			Regex:  ".+",
			Repeat: Repeat{Set: true, Min: 0, Max: 0},
		},
	}
}

// preambleScope returns a preamble entry.
func preambleScope() Scope {
	return Scope{Preamble: true}
}

// nested wraps inner scopes under a literal parent. Used by tests
// that build small nested trees without going through the parser.
func nested(parent string, children ...Scope) Scope {
	sc := literalScope(parent)
	sc.Sections = children
	return sc
}
