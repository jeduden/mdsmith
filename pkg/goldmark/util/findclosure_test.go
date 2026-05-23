package util_test

import (
	"testing"

	"github.com/yuin/goldmark/util"
)

func TestFindClosure(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		opener       byte
		closure      byte
		codeSpan     bool
		allowNesting bool
		want         int
	}{
		{"basic-close", "abc)", '(', ')', false, false, 3},
		{"nested-allowed", "a(b)c)", '(', ')', false, true, 5},
		{"nested-disallowed", "a(b)c)", '(', ')', false, false, -1},
		{"no-close", "abc", '(', ')', false, false, -1},
		{"escape-skip", `a\)b)`, '(', ')', false, false, 4},
		{"code-span-skip", "a `)` b)", '(', ')', true, false, 7},
		{"code-span-multi-backtick", "a `` ` ) `` b)", '(', ')', true, false, 13},
		{"empty", "", '(', ')', false, false, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := util.FindClosure([]byte(c.in), c.opener, c.closure, c.codeSpan, c.allowNesting)
			if got != c.want {
				t.Errorf("FindClosure(%q, %q, %q, codeSpan=%v, nest=%v) = %d, want %d",
					c.in, c.opener, c.closure, c.codeSpan, c.allowNesting, got, c.want)
			}
		})
	}
}
