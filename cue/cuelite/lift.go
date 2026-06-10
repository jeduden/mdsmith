package cuelite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"
)

// liftJSON parses a strict-JSON document into a concrete engine value. It
// is the in-house replacement for the CUE JSON lift CompileJSON used to
// delegate to: it enforces the same no-duplicate-key contract (a duplicate
// object key at any depth is rejected, naming the key — see CompileJSON),
// rejects any document that is not strict JSON (an unquoted key, a CUE
// expression), and builds an OPEN struct carrying every key concretely.
// Lifted data is open on purpose: closedness is a SCHEMA property, so a
// `close({a:int})` schema must reject an extra data key while a plain
// `{a:int}` schema accepts it. Were the data closed, unifying it with an open
// schema would import the data's closedness and change that outcome.
//
// json.Decoder with UseNumber preserves a number's exact text, so an
// integer lifts to kInt and a non-integral or out-of-int64 number to
// kFloat — matching the int64/float64 model. A lone-surrogate escape
// ("\ud800") decodes to U+FFFD under encoding/json and is accepted as a
// concrete string, the in-house engine's own stable behavior (the CUE lift
// rejected it as an invalid Unicode value; the differential harness's
// oracle is updated in lockstep).
func liftJSON(data []byte) (*engineValue, error) {
	// Strict JSON is UTF-8. encoding/json silently replaces an invalid byte
	// with U+FFFD, which would accept a document CUE's lift rejects; reject it
	// here so the data arm classifies malformed bytes as a compile failure.
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("cuelite: invalid JSON: not valid UTF-8")
	}
	if err := scanDuplicateKeys(data); err != nil {
		return nil, err
	}
	var raw any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("cuelite: invalid JSON: %w", err)
	}
	// Reject any content after the first top-level value, so `{"x":1} {...}`
	// or a trailing `}` is not silently accepted (matching strict JSON). At the
	// top level dec.More() does not flag trailing tokens, so probe for one more
	// token: anything but EOF is trailing data.
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("cuelite: invalid JSON: trailing data after top-level value")
	}
	return liftAny(raw)
}

// liftAny converts a decoded JSON value (from encoding/json with
// UseNumber) into a concrete engine value. Objects become OPEN structs
// (closedness is a schema property, not a data property — see liftJSON),
// arrays become closed lists, and scalars become concrete leaves.
func liftAny(x any) (*engineValue, error) {
	switch t := x.(type) {
	case nil:
		return &engineValue{kind: kNull}, nil
	case bool:
		return &engineValue{kind: kBool, b: t}, nil
	case string:
		return &engineValue{kind: kString, str: t}, nil
	case json.Number:
		return liftNumber(t)
	case map[string]any:
		return liftMap(t)
	case []any:
		return liftSlice(t)
	default:
		return nil, fmt.Errorf("cuelite: unsupported JSON value %T", x)
	}
}

// liftNumber converts a json.Number to an int64 leaf when it parses as an
// integer, else a float64 leaf, matching the int64/float64 value model.
func liftNumber(n json.Number) (*engineValue, error) {
	if i, err := n.Int64(); err == nil {
		return &engineValue{kind: kInt, i: i}, nil
	}
	// strconv.ParseFloat returns the ±Inf value together with an ErrRange for
	// an out-of-range literal (1e999); json.Number.Float64 forwards both. CUE
	// accepts such a literal as a concrete number, so the in-house lifter
	// keeps the ±Inf float rather than rejecting the document — the engine's
	// own stable behavior, with the differential oracle aligned.
	f, err := strconv.ParseFloat(n.String(), 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return nil, fmt.Errorf("cuelite: number %s: %w", n.String(), err)
	}
	return &engineValue{kind: kFloat, f: f}, nil
}

// liftMap converts a decoded JSON object to a closed struct, preserving no
// particular order (Go maps are unordered; field order does not affect
// unification or the leaf set). Each value is lifted recursively. A
// lone-surrogate-ESCAPE key is rejected before this point by scanDuplicateKeys
// (see CompileJSON), so every key here is a faithful round-trip.
func liftMap(m map[string]any) (*engineValue, error) {
	out := &engineValue{kind: kStruct}
	for k, v := range m {
		ev, err := liftAny(v)
		if err != nil {
			return nil, err
		}
		out.fields = append(out.fields, field{name: k, val: ev})
	}
	return out, nil
}

// liftSlice converts a decoded JSON array to a closed list (fixed length,
// no open tail), with each element lifted recursively.
func liftSlice(s []any) (*engineValue, error) {
	out := &engineValue{kind: kList}
	for _, el := range s {
		ev, err := liftAny(el)
		if err != nil {
			return nil, err
		}
		out.prefix = append(out.prefix, ev)
	}
	return out, nil
}

// liftMapValue converts a map[string]any (the document's parsed front
// matter) directly into a concrete engine value, with no JSON marshal/
// parse round-trip — the hot path plan 218 mandates. It accepts the value
// shapes a YAML/JSON front-matter decoder produces: map[string]any,
// []any, string, bool, the integer and float numeric kinds, json.Number,
// and nil. An unrecognized concrete type (a time.Time, say) is an error
// so a silent mis-validation cannot slip through.
func liftMapValue(x any) (*engineValue, error) {
	switch t := x.(type) {
	case nil:
		return &engineValue{kind: kNull}, nil
	case bool:
		return &engineValue{kind: kBool, b: t}, nil
	case string:
		return &engineValue{kind: kString, str: t}, nil
	case int:
		return &engineValue{kind: kInt, i: int64(t)}, nil
	case int64:
		return &engineValue{kind: kInt, i: t}, nil
	case float64:
		return &engineValue{kind: kFloat, f: t}, nil
	case float32:
		return &engineValue{kind: kFloat, f: float64(t)}, nil
	case json.Number:
		return liftNumber(t)
	case map[string]any:
		out := &engineValue{kind: kStruct}
		for k, v := range t {
			ev, err := liftMapValue(v)
			if err != nil {
				return nil, err
			}
			out.fields = append(out.fields, field{name: k, val: ev})
		}
		return out, nil
	case []any:
		out := &engineValue{kind: kList}
		for _, el := range t {
			ev, err := liftMapValue(el)
			if err != nil {
				return nil, err
			}
			out.prefix = append(out.prefix, ev)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("cuelite: unsupported front-matter value %T", x)
	}
}
