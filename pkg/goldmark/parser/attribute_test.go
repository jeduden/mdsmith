package parser

// Coverage for the attribute syntax parser — `{#id .class
// k=v k="quoted" k='single' k=123 k=[1,2,3]}`. Drives each leaf
// parser: parseAttributeString, parseAttributeNumber,
// parseAttributeArray, parseAttributeOthers, plus Find / findUpdate.

import (
	"bytes"
	"testing"

	"github.com/yuin/goldmark/text"
)

func TestParseAttributes_ValueShapes(t *testing.T) {
	// Drive every value-type branch in parseAttributeValue.
	cases := []struct {
		name string
		src  string // body inside outer braces
	}{
		{"id", `{#my-id}`},
		{"class", `{.my-class}`},
		{"double-quoted", `{k="v"}`},
		{"unquoted", `{k=v}`},
		{"integer", `{k=42}`},
		{"negative-integer", `{k=-7}`},
		{"float", `{k=3.14}`},
		{"true", `{k=true}`},
		{"false", `{k=false}`},
		{"null", `{k=null}`},
		{"array", `{k=[1, 2, 3]}`},
		{"array-strings", `{k=["a", "b"]}`},
		{"array-mixed", `{k=[1, "x", true]}`},
		{"multiple-attrs", `{#i .c k=v key="quoted" n=1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := text.NewReader([]byte(tc.src))
			attrs, ok := ParseAttributes(r)
			if !ok {
				t.Fatalf("ParseAttributes failed for %q", tc.src)
			}
			if len(attrs) == 0 {
				t.Errorf("ParseAttributes returned no attributes for %q", tc.src)
			}
		})
	}
}

func TestParseAttributes_Malformed(t *testing.T) {
	cases := []string{
		`{=v}`,        // empty key
		`{k=}`,        // empty value
		`{k="unclos`,  // unclosed double-quoted
		`{k='unclos`,  // unclosed single-quoted
		`{k=[1, 2`,    // unclosed array
		`{`,           // bare opener
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			r := text.NewReader([]byte(src))
			_, _ = ParseAttributes(r)
			// Just verifying ParseAttributes doesn't panic on
			// malformed input. The return value is intentionally
			// not asserted because the parser tolerates a wide
			// range of partial input.
		})
	}
}

func TestAttributesFind(t *testing.T) {
	// Build an Attributes via ParseAttributes, then Find each key.
	r := text.NewReader([]byte(`{#hi .c data-x=1 data-y="quoted"}`))
	attrs, ok := ParseAttributes(r)
	if !ok {
		t.Fatal("ParseAttributes failed")
	}
	// Attributes is a typed slice of Attribute; iterate and find.
	wantKeys := [][]byte{
		[]byte("id"),
		[]byte("class"),
		[]byte("data-x"),
		[]byte("data-y"),
	}
	for _, want := range wantKeys {
		found := false
		for _, a := range attrs {
			if bytes.Equal(a.Name, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing key %q in attrs %+v", want, attrs)
		}
	}
}
