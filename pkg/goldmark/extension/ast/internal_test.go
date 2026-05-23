package ast

// Internal unit tests for unreachable-via-public-API branches.

import (
	"testing"
)

func TestAlignment_String_DefaultArm(t *testing.T) {
	// Alignment.String's default arm fires when the value is
	// outside the defined constants.  Not reachable via parser
	// (the parser only emits AlignLeft/Right/Center/None) but
	// can be driven directly with a synthetic value.
	cases := []struct {
		a    Alignment
		want string
	}{
		{AlignLeft, "left"},
		{AlignRight, "right"},
		{AlignCenter, "center"},
		{AlignNone, "none"},
		{Alignment(99), ""}, // default arm
	}
	for _, c := range cases {
		if got := c.a.String(); got != c.want {
			t.Errorf("Alignment(%d).String() = %q, want %q", c.a, got, c.want)
		}
	}
}
