package yamlutil

import (
	"bytes"
	"strconv"
	"strings"
)

// FlatScalarFrontMatter attempts to decode a YAML front-matter body (the
// bytes between the two --- delimiters, without the delimiters themselves)
// into a map[string]any without invoking the yaml.v3 parser. It returns
// (result, true) when every line is an unambiguous top-level key: scalar
// pair; (nil, false) when any line requires full YAML parsing. Callers that
// receive false must fall back to [UnmarshalSafe].
//
// Security contract: inputs with anchors (&) or alias indicators (*) as
// value-first bytes cause an immediate false return so the caller routes
// them through [UnmarshalSafe], which rejects them via [RejectYAMLAliases].
// The fast path never expands aliases.
//
// The returned map is nil when the body is empty (matching yaml.v3 behaviour
// for an empty document).
func FlatScalarFrontMatter(body []byte) (map[string]any, bool) {
	if len(body) == 0 {
		return nil, true
	}

	var result map[string]any
	rest := body

	for len(rest) > 0 {
		var line []byte
		if idx := bytes.IndexByte(rest, '\n'); idx >= 0 {
			line = rest[:idx]
			rest = rest[idx+1:]
		} else {
			line = rest
			rest = nil
		}
		// Trim Windows-style CR.
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}

		if len(line) == 0 {
			continue // blank line
		}
		if line[0] == '#' {
			continue // comment line
		}
		// Multi-doc markers.
		if bytes.HasPrefix(line, []byte("---")) || bytes.HasPrefix(line, []byte("...")) {
			return nil, false
		}
		// Leading whitespace = nested mapping or sequence continuation.
		if line[0] == ' ' || line[0] == '\t' {
			return nil, false
		}

		// Locate the key separator: first ':' followed by ' ', '\t', or EOL.
		colonPos := -1
		for i := 0; i < len(line); i++ {
			if line[i] != ':' {
				continue
			}
			if i+1 == len(line) || line[i+1] == ' ' || line[i+1] == '\t' {
				colonPos = i
				break
			}
		}
		if colonPos < 0 {
			return nil, false // no valid key separator
		}

		key := string(line[:colonPos])
		if !isValidFlatKey(key) {
			return nil, false
		}

		// Raw value: everything after the ': ' (or ':' at EOL).
		var rawVal []byte
		if colonPos+1 < len(line) {
			rawVal = bytes.TrimLeft(line[colonPos+1:], " \t")
		}

		// Bail on block scalars, flow collections, anchors, aliases, tags,
		// and explicit keys before even attempting value parsing.
		if len(rawVal) > 0 {
			switch rawVal[0] {
			case '|', '>', '[', '{', '&', '*', '?', '!':
				return nil, false
			}
		}

		// Strip trailing inline comment (" # ..." or "\t# ...").
		rawVal = stripFlatInlineComment(rawVal)
		rawVal = bytes.TrimRight(rawVal, " \t")

		val, ok := parseFlatScalar(rawVal)
		if !ok {
			return nil, false
		}

		if result == nil {
			result = make(map[string]any, 4)
		}
		result[key] = val
	}

	return result, true
}

// isValidFlatKey reports whether key is a safe top-level YAML mapping key
// for the fast path: non-empty, starts with an ASCII letter or digit, and
// contains only ASCII letters, digits, hyphens, or underscores. Quoted or
// complex keys are not accepted; callers bail on those.
func isValidFlatKey(key string) bool {
	if len(key) == 0 {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_':
			if i == 0 {
				return false // key must start with letter or digit
			}
		default:
			return false
		}
	}
	return true
}

// stripFlatInlineComment removes a YAML inline comment from a plain scalar
// value. A comment starts at the first ' #' or '\t#' sequence. The leading
// space/tab before '#' is consumed along with the comment.
func stripFlatInlineComment(val []byte) []byte {
	for i := 1; i < len(val); i++ {
		if val[i] == '#' && (val[i-1] == ' ' || val[i-1] == '\t') {
			return val[:i-1]
		}
	}
	return val
}

// parseFlatScalar converts the raw bytes of a YAML scalar value into the
// appropriate Go type, matching the output of yaml.v3 decoding into any.
// Returns (nil, false) to signal "bail out and use UnmarshalSafe".
func parseFlatScalar(raw []byte) (any, bool) {
	if len(raw) == 0 {
		return nil, true // empty value is YAML null
	}

	switch raw[0] {
	case '"':
		s, ok := parseDoubleQuoted(raw)
		if !ok {
			return nil, false
		}
		return s, true
	case '\'':
		s, ok := parseSingleQuoted(raw)
		if !ok {
			return nil, false
		}
		return s, true
	}

	return parsePlainFlatScalar(raw)
}

// parseDoubleQuoted decodes a double-quoted YAML scalar (raw must start and
// end with '"'). Handles the common escape sequences \\, \", \n, \t, \r,
// \a, \b, \f, \v, \0, \/. Any other backslash escape causes a bail (false).
// A closing '"' that is not at the final byte also bails (extra content).
func parseDoubleQuoted(raw []byte) (string, bool) {
	if len(raw) < 2 || raw[len(raw)-1] != '"' {
		return "", false // unclosed or bare "
	}
	inner := raw[1 : len(raw)-1] // strip outer "…"

	// Fast path: no backslash means no escapes to process.
	if bytes.IndexByte(inner, '\\') < 0 {
		// Also verify no unescaped closing quote mid-string.
		if bytes.IndexByte(inner, '"') >= 0 {
			return "", false // mid-string " — extra content after first close
		}
		return string(inner), true
	}

	var buf strings.Builder
	buf.Grow(len(inner))
	for i := 0; i < len(inner); i++ {
		b := inner[i]
		if b == '"' {
			// Unescaped closing quote before end of inner.
			return "", false
		}
		if b != '\\' {
			buf.WriteByte(b)
			continue
		}
		i++
		if i >= len(inner) {
			return "", false // trailing backslash
		}
		switch inner[i] {
		case '\\':
			buf.WriteByte('\\')
		case '"':
			buf.WriteByte('"')
		case 'n':
			buf.WriteByte('\n')
		case 't':
			buf.WriteByte('\t')
		case 'r':
			buf.WriteByte('\r')
		case 'a':
			buf.WriteByte('\a')
		case 'b':
			buf.WriteByte('\b')
		case 'f':
			buf.WriteByte('\f')
		case 'v':
			buf.WriteByte('\v')
		case '0':
			buf.WriteByte(0)
		case ' ':
			buf.WriteByte(' ')
		default:
			return "", false // unknown escape sequence → bail
		}
	}
	return buf.String(), true
}

// parseSingleQuoted decodes a single-quoted YAML scalar (raw must start and
// end with '\''). The only escape sequence in single-quoted YAML strings is
// '' (two single quotes) representing a literal single quote.
func parseSingleQuoted(raw []byte) (string, bool) {
	if len(raw) < 2 || raw[len(raw)-1] != '\'' {
		return "", false // unclosed
	}
	inner := raw[1 : len(raw)-1]

	// Check for unclosed-quote problem: the last character of inner should
	// not be a lone ' (which would mean the real close is further back).
	// With '' being the only escape, the last ' in inner followed by the
	// outer closing ' could be mis-parsed. The YAML rule: an odd number of
	// trailing single-quotes in inner means the string was not correctly
	// delimited, but since we already stripped the outer '...' pair, just
	// handle '' substitution.
	if !bytes.Contains(inner, []byte("''")) {
		return string(inner), true
	}
	return string(bytes.ReplaceAll(inner, []byte("''"), []byte("'"))), true
}

// parsePlainFlatScalar returns the Go value for a plain (unquoted) YAML
// scalar, matching yaml.v3's type inference into any. Returns (nil, false)
// for ambiguous or complex values that yaml.v3 would interpret in ways the
// fast path cannot replicate without the full parser.
func parsePlainFlatScalar(raw []byte) (any, bool) {
	s := string(raw)

	// Null spellings.
	switch s {
	case "null", "Null", "NULL", "~":
		return nil, true
	}

	// Lowercase-only boolean — exact match so yaml 1.1 alternate spellings
	// (True, TRUE, yes, no, on, off) fall through to the bail below.
	switch s {
	case "true":
		return true, true
	case "false":
		return false, true
	}

	// Bail on other yaml.v3/1.1 bool spellings to avoid returning the wrong
	// type (yaml.v3 returns bool, the fast path would return string).
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off":
		return nil, false
	}

	// Bail on YAML 1.1 float-special literals.
	switch strings.ToLower(s) {
	case ".inf", "-.inf", "+.inf", ".nan":
		return nil, false
	}

	// Bail on hex / octal / binary prefixes and underscored numerics.
	if strings.ContainsRune(s, '_') {
		return nil, false
	}
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X' ||
		s[1] == 'o' || s[1] == 'O' || s[1] == 'b' || s[1] == 'B') {
		return nil, false
	}

	// Bail on float-looking values (contain '.' or 'e'/'E' suggestive of
	// scientific notation). A leading '+' also defers to yaml.v3.
	if strings.ContainsAny(s, ".eE") {
		return nil, false
	}
	if len(s) > 0 && s[0] == '+' {
		return nil, false
	}

	// Decimal integer: optional '-', then digits only, no leading zeros.
	if isDecimalIntStr(s) {
		n, err := strconv.Atoi(s)
		if err == nil {
			return n, true
		}
		// Overflow: yaml.v3 would use int64 or float64; bail.
		return nil, false
	}

	// Leading-zero non-integer (e.g. "01", "007") is octal in yaml 1.1.
	start := 0
	if len(s) > 0 && s[0] == '-' {
		start = 1
	}
	if len(s)-start > 1 && s[start] == '0' {
		return nil, false
	}

	// Bail on YAML metacharacters that are unsafe in plain scalars.
	if strings.ContainsAny(s, ":#{}[]!&*@`") {
		return nil, false
	}

	// Plain string.
	return s, true
}

// isDecimalIntStr reports whether s is a valid decimal integer literal:
// an optional leading minus followed by one or more digits with no leading
// zero (unless the value is exactly "0" or "-0").
func isDecimalIntStr(s string) bool {
	if len(s) == 0 {
		return false
	}
	start := 0
	if s[0] == '-' {
		start = 1
	}
	if start >= len(s) {
		return false // bare "-"
	}
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	// Disallow leading zeros in multi-digit values (yaml 1.1 octal).
	if len(s)-start > 1 && s[start] == '0' {
		return false
	}
	return true
}
