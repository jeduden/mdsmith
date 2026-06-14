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
		line, remaining := nextFlatLine(rest)
		rest = remaining

		if len(line) == 0 || line[0] == '#' {
			continue
		}
		key, val, ok := parseFlatLine(line)
		if !ok {
			return nil, false
		}
		if result == nil {
			result = make(map[string]any, 4)
		}
		// Bail on duplicates: yaml.v3 errors; the fast path must not accept them.
		if _, dup := result[key]; dup {
			return nil, false
		}
		result[key] = val
	}

	return result, true
}

// parseFlatLine parses a single non-empty, non-comment YAML front-matter line
// into its key and scalar value. Returns ("", nil, false) when the line
// requires full YAML parsing.
func parseFlatLine(line []byte) (string, any, bool) {
	if bytes.HasPrefix(line, []byte("---")) || bytes.HasPrefix(line, []byte("...")) {
		return "", nil, false
	}
	if line[0] == ' ' || line[0] == '\t' {
		return "", nil, false
	}

	colonPos := findKeyColon(line)
	if colonPos < 0 {
		return "", nil, false
	}

	key := string(line[:colonPos])
	if !isValidFlatKey(key) {
		return "", nil, false
	}

	var rawVal []byte
	if colonPos+1 < len(line) {
		rawVal = bytes.TrimLeft(line[colonPos+1:], " \t")
	}

	// Bail on block scalars, flow collections, anchors, aliases, tags, explicit keys.
	if len(rawVal) > 0 {
		switch rawVal[0] {
		case '|', '>', '[', '{', '&', '*', '?', '!':
			return "", nil, false
		}
	}

	rawVal = stripFlatInlineComment(rawVal)
	rawVal = bytes.TrimRight(rawVal, " \t")

	val, ok := parseFlatScalar(rawVal)
	if !ok {
		return "", nil, false
	}
	return key, val, true
}

// nextFlatLine splits the next line from rest, trimming any trailing CR.
// It returns the line (without the newline) and the remaining bytes after it.
func nextFlatLine(rest []byte) (line, remaining []byte) {
	if idx := bytes.IndexByte(rest, '\n'); idx >= 0 {
		line = rest[:idx]
		remaining = rest[idx+1:]
	} else {
		line = rest
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return
}

// findKeyColon returns the index of the first colon in line that is immediately
// followed by a space, tab, or end-of-line — the YAML mapping key separator.
// Returns -1 when no valid separator is found.
func findKeyColon(line []byte) int {
	for i := 0; i < len(line); i++ {
		if line[i] != ':' {
			continue
		}
		if i+1 == len(line) || line[i+1] == ' ' || line[i+1] == '\t' {
			return i
		}
	}
	return -1
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
// value. A comment starts at the first " #" or "\t#" sequence. The leading
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
// end with a double-quote byte). Supported escapes are handled by
// decodeDoubleEscapeByte; any unrecognised backslash sequence causes a bail
// (false). An unescaped closing quote before the final byte also bails.
func parseDoubleQuoted(raw []byte) (string, bool) {
	if len(raw) < 2 || raw[len(raw)-1] != '"' {
		return "", false // unclosed or bare "
	}
	inner := raw[1 : len(raw)-1] // strip outer "..."

	// Fast path: no backslash means no escapes to process.
	if bytes.IndexByte(inner, '\\') < 0 {
		// Verify no unescaped closing quote mid-string.
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
			return "", false // unescaped closing quote before end of inner
		}
		if b != '\\' {
			buf.WriteByte(b)
			continue
		}
		i++
		if i >= len(inner) {
			return "", false // trailing backslash
		}
		decoded, ok := decodeDoubleEscapeByte(inner[i])
		if !ok {
			return "", false // unknown escape sequence → bail
		}
		buf.WriteByte(decoded)
	}
	return buf.String(), true
}

// decodeDoubleEscapeByte maps the byte after a backslash in a double-quoted
// YAML string to its decoded value. Returns (0, false) for escape sequences
// the fast path does not handle — the caller must bail to the full parser.
func decodeDoubleEscapeByte(ch byte) (byte, bool) {
	switch ch {
	case '\\':
		return '\\', true
	case '"':
		return '"', true
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'r':
		return '\r', true
	case 'a':
		return '\a', true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'v':
		return '\v', true
	case '0':
		return 0, true
	case ' ':
		return ' ', true
	}
	return 0, false
}

// parseSingleQuoted decodes a single-quoted YAML scalar. The only escape
// sequence in single-quoted strings is two consecutive single quotes, which
// represent one literal single quote. A lone single quote inside the inner
// content means the real closing quote was earlier and there is trailing
// content; the function bails (false) so the caller routes through the full
// parser.
func parseSingleQuoted(raw []byte) (string, bool) {
	if len(raw) < 2 || raw[len(raw)-1] != '\'' {
		return "", false // unclosed
	}
	inner := raw[1 : len(raw)-1]

	// Fast path: no embedded quote at all.
	if bytes.IndexByte(inner, '\'') < 0 {
		return string(inner), true
	}

	var buf strings.Builder
	buf.Grow(len(inner))
	for i := 0; i < len(inner); i++ {
		if inner[i] != '\'' {
			buf.WriteByte(inner[i])
			continue
		}
		// A single quote must be the first half of a doubled-quote escape pair.
		if i+1 >= len(inner) || inner[i+1] != '\'' {
			return "", false // lone quote → premature close + trailing content
		}
		buf.WriteByte('\'')
		i++ // consume the second quote of the pair
	}
	return buf.String(), true
}

// parsePlainFlatScalar returns the Go value for a plain (unquoted) YAML
// scalar, matching yaml.v3's type inference into any. Returns (nil, false)
// for ambiguous or complex values that yaml.v3 would interpret in ways the
// fast path cannot replicate without the full parser.
func parsePlainFlatScalar(raw []byte) (any, bool) {
	// A plain scalar may not begin with a YAML indicator character.
	// yaml.v3 raises a parse error for these, so the fast path must defer
	// rather than treat the value as a plain string. The first-byte switch
	// in FlatScalarFrontMatter already rejects | > [ { & * ? ! and quotes;
	// here we cover the remainder: ',' and '%' (and '@', '`' for safety),
	// plus '-', '?' and ':' when followed by a space or end of value (which
	// make them block-sequence / mapping indicators rather than scalars).
	switch raw[0] {
	case ',', '%', '@', '`':
		return nil, false
	case '-', '?', ':':
		if len(raw) == 1 || raw[1] == ' ' || raw[1] == '\t' {
			return nil, false
		}
	}

	s := string(raw)

	if v, ok, done := yamlLiteralValue(s); done {
		return v, ok
	}

	return parsePlainNumericOrString(s)
}

// yamlLiteralValue resolves YAML null, boolean, and special float spellings.
// When done is true the caller should return (v, ok) immediately. When done
// is false the value is not a recognised literal and parsing should continue.
func yamlLiteralValue(s string) (v any, ok bool, done bool) {
	switch s {
	case "null", "Null", "NULL", "~":
		return nil, true, true
	case "true":
		return true, true, true
	case "false":
		return false, true, true
	}
	lower := strings.ToLower(s)
	switch lower {
	case "true", "false", "yes", "no", "on", "off":
		return nil, false, true // yaml.v3 bool spelling; bail so type matches
	case ".inf", "-.inf", "+.inf", ".nan":
		return nil, false, true // yaml.v3 float special; bail
	}
	return nil, false, false
}

// parsePlainNumericOrString parses a plain YAML scalar as an integer or
// string. The caller has already eliminated YAML literal spellings (null,
// bool, float specials) and leading YAML indicator bytes.
func parsePlainNumericOrString(s string) (any, bool) {
	// Bail on numeric syntax yaml.v3 handles as non-int (hex, octal, binary,
	// underscore-grouped, float, scientific notation, leading '+').
	if strings.ContainsRune(s, '_') {
		return nil, false
	}
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X' ||
		s[1] == 'o' || s[1] == 'O' || s[1] == 'b' || s[1] == 'B') {
		return nil, false
	}
	if strings.ContainsAny(s, ".eE") {
		return nil, false
	}
	if s[0] == '+' {
		return nil, false
	}
	// Bail on YAML timestamp candidates. yaml.v3 resolves plain scalars that
	// begin with exactly four digits followed by '-' (e.g. "2026-01-01") to a
	// time.Time, not a string. Timestamp forms with a clock component also
	// contain ':' and are caught by the metacharacter check below, but the
	// date-only form has no other indicator, so detect it here.
	if looksLikeTimestamp(s) {
		return nil, false
	}

	if isDecimalIntStr(s) {
		n, err := strconv.Atoi(s)
		if err == nil {
			return n, true
		}
		return nil, false // overflow: yaml.v3 uses int64 or float64
	}

	// Leading-zero non-integer (e.g. "01", "007") is octal in yaml 1.1.
	start := 0
	if s[0] == '-' {
		start = 1
	}
	if len(s)-start > 1 && s[start] == '0' {
		return nil, false
	}

	// Plain string — bail on metacharacters that are unsafe in plain scalars.
	if strings.ContainsAny(s, ":#{}[]!&*@`") {
		return nil, false
	}
	return s, true
}

// looksLikeTimestamp reports whether s could be parsed by yaml.v3 as a YAML
// timestamp. yaml.v3's parseTimestamp first requires exactly four leading
// digits followed by '-'; only then does it attempt the allowed timestamp
// layouts. The fast path uses the same guard and bails conservatively for any
// such candidate, since it cannot reproduce the time.Time result and a false
// return is always safe.
func looksLikeTimestamp(s string) bool {
	if len(s) < 5 {
		return false
	}
	for i := 0; i < 4; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s[4] == '-'
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
