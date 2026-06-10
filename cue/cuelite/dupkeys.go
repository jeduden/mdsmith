package cuelite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// scanDuplicateKeys walks a JSON document with the streaming token decoder
// and reports the first object key that appears twice within the same
// object — at any nesting depth, including objects nested in array
// elements. It enforces the strict-JSON no-duplicate contract CompileJSON
// documents, ported from the former CUE-backed implementation (the
// contract is durable across the in-house flip — plan 238).
//
// The scan defers (returns nil, leaving any error to the JSON decoder) on
// the same four inputs the contract documents: a malformed document, a
// second top-level value, invalid UTF-8 input (json.Decoder folds distinct
// invalid-byte keys onto one U+FFFD), and a decoded key containing U+FFFD
// (two lone-surrogate keys decode identically and cannot be told apart).
func scanDuplicateKeys(data []byte) error {
	if !utf8.Valid(data) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var stack []*dupLevel
	for {
		start := dec.InputOffset()
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return nil // malformed: defer to the JSON decoder's own error
		}
		var cur *dupLevel
		if len(stack) > 0 {
			cur = stack[len(stack)-1]
		}
		if cur.keyHasLoneSurrogateEscape(tok, data[start:dec.InputOffset()]) {
			return fmt.Errorf("cuelite: invalid JSON: object key has a lone-surrogate escape")
		}
		if handled, err := cur.recordKey(tok); err != nil {
			return err
		} else if handled {
			continue
		}
		switch tok {
		case json.Delim('{'):
			stack = append(stack, &dupLevel{keys: map[string]struct{}{}})
			continue
		case json.Delim('['):
			stack = append(stack, &dupLevel{})
			continue
		case json.Delim('}'), json.Delim(']'):
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return nil
			}
			parent := stack[len(stack)-1]
			if parent.keys != nil {
				parent.seenKey = false
			}
			continue
		}
		if cur == nil {
			return nil
		}
		if cur.keys != nil {
			cur.seenKey = false
		}
	}
}

// keyHasLoneSurrogateEscape reports whether tok is a KEY at this object level
// whose decoded value holds U+FFFD AND whose raw source bytes carry a
// lone-surrogate escape. A key token decoded to U+FFFD may have come from a
// lone-surrogate escape (`\ud800`) or a literal U+FFFD; they are
// indistinguishable after decode, but the RAW token bytes tell them apart — an
// escape carries a `\u` sequence. CUE rejects the escape ("unmatched surrogate
// pair") and accepts the literal, so this rejects only the escape, restoring
// the pre-flip CompileJSON contract while still accepting a literal U+FFFD key.
// A lone-surrogate VALUE escape is not a key, so it is untouched here and stays
// accepted as a U+FFFD string.
func (l *dupLevel) keyHasLoneSurrogateEscape(tok any, raw []byte) bool {
	if l == nil || l.keys == nil || l.seenKey {
		return false
	}
	s, ok := tok.(string)
	if !ok || !strings.ContainsRune(s, utf8.RuneError) {
		return false
	}
	return rawHasLoneSurrogateEscape(raw)
}

// rawHasLoneSurrogateEscape reports whether the raw JSON token bytes contain a
// lone (unpaired) `\uXXXX` surrogate escape: a high surrogate (D800–DBFF) not
// immediately followed by a low-surrogate escape, or a low surrogate
// (DC00–DFFF) standing alone. This is the residue encoding/json folds to U+FFFD
// and CUE rejects as an "unmatched surrogate pair".
//
// The scan tokenizes JSON string escapes left to right: an ESCAPED backslash
// (`\\`) consumes both bytes, so a following `ud800` is literal text, not a
// `\u` escape (CUE accepts `"\\ud800"`). Only an UNescaped `\` that introduces
// a `\u` is a unicode escape examined for a lone surrogate.
func rawHasLoneSurrogateEscape(raw []byte) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' || i+1 >= len(raw) {
			continue
		}
		// An escaped backslash (or any non-u two-char escape) is consumed whole,
		// so the byte after it cannot start a `\u` escape.
		if raw[i+1] != 'u' {
			i++
			continue
		}
		if i+6 > len(raw) {
			continue
		}
		cu, ok := parseHex4(raw[i+2 : i+6])
		if !ok {
			continue
		}
		if cu >= 0xDC00 && cu <= 0xDFFF {
			// A low surrogate reached here is not the trailing half of a pair (a
			// valid pair skips its low half below), so it is unpaired.
			return true
		}
		if cu >= 0xD800 && cu <= 0xDBFF {
			// A high surrogate must be followed by a `\uDC00–DFFF` low surrogate.
			if i+12 > len(raw) || raw[i+6] != '\\' || raw[i+7] != 'u' {
				return true
			}
			lo, ok := parseHex4(raw[i+8 : i+12])
			if !ok || lo < 0xDC00 || lo > 0xDFFF {
				return true
			}
			// A valid pair: advance past the low half (the six bytes `\uXXXX` at
			// i+6..i+11) so its standalone scan does not misread it as lone.
			i += 11
		}
	}
	return false
}

// parseHex4 parses exactly four hex digits into a code unit, reporting ok=false
// when any byte is not a hex digit.
func parseHex4(b []byte) (uint32, bool) {
	if len(b) != 4 {
		return 0, false
	}
	var v uint32
	for _, c := range b {
		var d uint32
		switch {
		case c >= '0' && c <= '9':
			d = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint32(c-'A') + 10
		default:
			return 0, false
		}
		v = v<<4 | d
	}
	return v, true
}

// dupLevel is one open container in scanDuplicateKeys' stack. keys is
// non-nil for an object level and holds the keys seen so far; nil marks an
// array level. seenKey is true when the next string token at an object
// level is a value (the key was already read).
type dupLevel struct {
	keys    map[string]struct{}
	seenKey bool
}

// recordKey handles tok when it is the key half of a key/value pair at this
// object level, returning handled=true so the caller advances. It reports
// an error on the first duplicate key. tok is not a key (handled=false)
// when the level is nil/array, already past the key, or not a string. A key
// whose decode produced U+FFFD is consumed but skipped for dup tracking.
func (l *dupLevel) recordKey(tok any) (bool, error) {
	if l == nil || l.keys == nil || l.seenKey {
		return false, nil
	}
	s, ok := tok.(string)
	if !ok {
		return false, nil
	}
	l.seenKey = true
	if strings.ContainsRune(s, utf8.RuneError) {
		return true, nil
	}
	if _, dup := l.keys[s]; dup {
		return true, fmt.Errorf("duplicate JSON key %q", s)
	}
	l.keys[s] = struct{}{}
	return true, nil
}
