package cuelite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// bytesReader wraps a byte slice as an io.Reader for json.Decoder, so the
// JSON lifters stream without copying the slice.
func bytesReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}

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
