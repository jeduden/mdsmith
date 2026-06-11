package syntax

import (
	"fmt"
	"strconv"
)

// number.go validates and decodes the CUE number-literal grammar (plan 240).
// The scanner captures a number's raw bytes greedily; validateNumber then
// applies CUE's rules the greedy scan does not: a leading-zero decimal integer
// is illegal (CUE: "illegal integer number"; `010` is NOT octal), and every
// `_` digit separator must be immediately followed by a digit (CUE: "illegal
// '_' in number"; a trailing, doubled, or non-digit-adjacent `_` is rejected).
// A leading zero is legal in a FLOAT (`01.5`, `0e5`) and in a base-prefixed
// integer (`0x10`). The accepted set was probed against cuelang.org/go v0.16.1.

// validateNumber checks a scanned number literal against CUE's leading-zero and
// underscore rules. raw is the literal's source text; isFloat reports whether
// the scanner saw a fraction or exponent (so the leading-zero rule applies to
// integers only). It returns an error naming the violation, or nil when the
// literal is well-formed.
func validateNumber(raw string, isFloat bool) error {
	if err := validateUnderscores(raw); err != nil {
		return err
	}
	if !isFloat {
		return validateIntLeadingZero(raw)
	}
	return nil
}

// validateUnderscores enforces that every `_` in the literal is immediately
// followed by a digit (decimal, or hex within a base-prefixed literal). A `_`
// that is trailing, doubled, or adjacent to a `.`/`e`/prefix boundary is
// illegal, matching CUE's "illegal '_' in number".
func validateUnderscores(raw string) error {
	hex := isBasePrefixed(raw)
	for i := 0; i < len(raw); i++ {
		if raw[i] != '_' {
			continue
		}
		if i+1 >= len(raw) {
			return fmt.Errorf("illegal '_' in number %q", raw)
		}
		next := raw[i+1]
		ok := isDigitByte(next) || (hex && isHexDigit(next))
		if !ok {
			return fmt.Errorf("illegal '_' in number %q", raw)
		}
	}
	return nil
}

// validateIntLeadingZero rejects a decimal integer with a redundant leading
// zero (`010`, `00`), which CUE reports as "illegal integer number". A lone
// `0`, a base-prefixed integer (`0x…`), and a leading-zero float are exempt;
// the caller passes only non-float, and base-prefixed literals are skipped
// here.
func validateIntLeadingZero(raw string) error {
	if isBasePrefixed(raw) {
		return nil
	}
	if len(raw) >= 2 && raw[0] == '0' {
		return fmt.Errorf("illegal integer number %q", raw)
	}
	return nil
}

// isBasePrefixed reports whether the literal carries a 0x/0o/0b base prefix
// (any letter case the scanner admitted), so the leading-zero and hex-digit
// underscore rules treat it as a based integer rather than a decimal.
func isBasePrefixed(raw string) bool {
	if len(raw) < 2 || raw[0] != '0' {
		return false
	}
	switch raw[1] {
	case 'x', 'X', 'o', 'O', 'b', 'B':
		return true
	}
	return false
}

// ParseIntLiteral decodes an integer literal's source text into an int64,
// stripping `_` separators and reading an unprefixed literal as base 10 (NOT
// base 0, which would mis-read a leading-zero literal as octal). A 0x/0o/0b
// prefix is honored. It is the in-house replacement for the strconv.ParseInt
// base-0 call compileBasicLit used, with base 10 for the decimal case so the
// scanner's leading-zero rejection is the only octal-shaped path.
func ParseIntLiteral(raw string) (int64, error) {
	clean := stripNumUnderscores(raw)
	if isBasePrefixed(clean) {
		return strconv.ParseInt(clean, 0, 64)
	}
	return strconv.ParseInt(clean, 10, 64)
}

// ParseFloatLiteral decodes a float literal's source text into a float64,
// stripping `_` separators. It is the in-house replacement for the
// strconv.ParseFloat call compileBasicLit used.
func ParseFloatLiteral(raw string) (float64, error) {
	return strconv.ParseFloat(stripNumUnderscores(raw), 64)
}

// stripNumUnderscores removes the `_` digit-group separators so strconv can
// parse the literal. It mirrors compile.go's stripUnderscores, kept in the
// syntax package so the literal decoders live beside the scanner that produced
// them.
func stripNumUnderscores(s string) string {
	if !hasUnderscore(s) {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '_' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

// hasUnderscore reports whether s contains a `_`, the cheap guard
// stripNumUnderscores uses to avoid allocating when there is nothing to strip.
func hasUnderscore(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '_' {
			return true
		}
	}
	return false
}
