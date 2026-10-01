package mdfence

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
)

// finalLines are opener lines whose info verdict OpenFinal must share
// with goldmark both as a final line without a newline and with one.
var finalLines = []string{
	"```", "```x", "```\v", "```\f", "``` x", "```xy", "```  ", "``` \r",
	"~~~`", "~~~x", "  ```x", "````x", "```x\r", "```\r",
}

// forkFence parses src and reports whether its first block is a fenced
// code block and whether that block carries a non-empty info string.
func forkFence(src string) (isFence, hasInfo bool) {
	root := goldmark.DefaultParser().Parse(text.NewReader([]byte(src)))
	fcb, ok := root.FirstChild().(*ast.FencedCodeBlock)
	if !ok {
		return false, false
	}
	return true, fcb.Info != nil && fcb.Info.Segment.Len() > 0
}

func TestOpenFinal_MatchesFork(t *testing.T) {
	for _, line := range finalLines {
		for _, final := range []bool{true, false} {
			src := line
			if !final {
				src += "\n"
			}
			wantFence, wantInfo := forkFence(src)
			f, ok := OpenFinal([]byte(line), final)
			assert.Equal(t, wantFence, ok, "fence %q final=%v", line, final)
			assert.Equal(t, wantInfo, f.HasInfo, "info %q final=%v", line, final)
		}
	}
}

func TestOpenFinal(t *testing.T) {
	tests := []struct {
		line    string
		final   bool
		hasInfo bool
	}{
		{"```x", true, false},
		{"```x", false, true},
		{"```\v", true, false},
		{"```\v", false, true},
		{"``` x", true, true},
		{"```xy", true, true},
	}
	for _, tt := range tests {
		f, ok := OpenFinal([]byte(tt.line), tt.final)
		assert.True(t, ok, "%q", tt.line)
		assert.Equal(t, tt.hasInfo, f.HasInfo, "%q final=%v", tt.line, tt.final)
	}
}
