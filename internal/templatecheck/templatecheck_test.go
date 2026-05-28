package templatecheck

import (
	"strings"
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelpers_DefensiveBranches exercises the nil-pipe and
// empty-ident guards in the unexported helpers. These branches
// are not reachable through real Hugo AST input (the parser never
// emits a nil pipe or a zero-length Ident slice), but they guard
// callers that pass synthetic nodes — and the project's coverage
// gate requires every branch to be driven.
func TestHelpers_DefensiveBranches(t *testing.T) {
	t.Run("pipeReferencesSummary handles nil", func(t *testing.T) {
		assert.False(t, pipeReferencesSummary(nil))
	})
	t.Run("pipeAssignsSummary handles nil", func(t *testing.T) {
		assert.False(t, pipeAssignsSummary(nil))
	})
	t.Run("pipeOutputsSummaryViaRenderString handles nil", func(t *testing.T) {
		assert.False(t, pipeOutputsSummaryViaRenderString(nil))
	})
	t.Run("pipeOutputsSummaryViaRenderString handles empty Cmds", func(t *testing.T) {
		assert.False(t, pipeOutputsSummaryViaRenderString(&parse.PipeNode{}))
	})
	t.Run("cmdIsRenderString handles empty Args", func(t *testing.T) {
		assert.False(t, cmdIsRenderString(&parse.CommandNode{}))
	})
	t.Run("cmdIsRenderString handles FieldNode with empty Ident", func(t *testing.T) {
		cmd := &parse.CommandNode{Args: []parse.Node{&parse.FieldNode{}}}
		assert.False(t, cmdIsRenderString(cmd))
	})
	t.Run("cmdIsRenderString handles ChainNode with empty Field", func(t *testing.T) {
		cmd := &parse.CommandNode{Args: []parse.Node{&parse.ChainNode{}}}
		assert.False(t, cmdIsRenderString(cmd))
	})
	t.Run("cmdIsRenderString handles VariableNode with empty Ident", func(t *testing.T) {
		cmd := &parse.CommandNode{Args: []parse.Node{&parse.VariableNode{}}}
		assert.False(t, cmdIsRenderString(cmd))
	})
	t.Run("argReferencesSummary returns false for unknown node type", func(t *testing.T) {
		assert.False(t, argReferencesSummary(&parse.StringNode{}))
	})
	t.Run("walker.lineOf clamps positions past end of content", func(t *testing.T) {
		w := &walker{content: "a\nb"}
		// pos == 100 (past content end) gets clamped to len(content)=3,
		// which lands on the second line.
		assert.Equal(t, 2, w.lineOf(parse.Pos(100)))
	})
	t.Run("walk handles nil ListNode through interface", func(t *testing.T) {
		w := &walker{path: "x", content: ""}
		// IfNode without else has ElseList == nil *ListNode; the walk
		// path receives that as a non-nil parse.Node interface wrapping
		// a nil pointer and must not panic.
		var nilList *parse.ListNode
		w.walk(nilList)
		assert.Empty(t, w.violations)
	})
	t.Run("walk handles nil interface", func(t *testing.T) {
		w := &walker{path: "x", content: ""}
		// Literal nil — the parse.Node interface itself is nil. The
		// outer `if n == nil` short-circuits before the type switch.
		w.walk(nil)
		assert.Empty(t, w.violations)
	})
	t.Run("argReferencesSummary ChainNode whose Field carries Params.summary", func(t *testing.T) {
		// `{{ (.X).Params.summary }}` — the trailing field chain
		// itself contains Params.summary; chainIsSummary returns
		// true without needing to recurse into the receiver.
		got, err := Scan("file.html", `{{ (.X).Params.summary }}`)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})
	t.Run("pipeOutputsSummaryViaRenderString summary in later stage Args[1:]", func(t *testing.T) {
		// `{{ .X | someFunc .Params.summary | .RenderString }}` —
		// summary arrives via a positional arg to a non-RenderString
		// middle stage, not as the pipe head. The output-tracking
		// loop's else-branch must set summaryFlowing so the
		// terminating .RenderString stage returns true.
		got, err := Scan("file.html", `{{ .X | someFunc .Params.summary | .RenderString }}`)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

// TestScan_TableDriven enumerates every safe and unsafe shape the
// AST classifier recognises. Each entry is a full template body
// (delimiters included) — the scanner parses it the same way Hugo
// would.
func TestScan_TableDriven(t *testing.T) {
	cases := []struct {
		name      string
		template  string
		wantCount int
	}{
		// Safe: presence predicates.
		{"if presence",
			`{{ if .Params.summary }}<p>{{ .RenderString (dict "display" "inline") .Params.summary }}</p>{{ end }}`, 0},
		{"if not", `{{ if not .Params.summary }}x{{ end }}`, 0},
		{"if and compound", `{{ if and .Params.summary .X }}{{ .RenderString (dict) .Params.summary }}{{ end }}`, 0},
		{"if or compound", `{{ if or .Params.summary .Other }}x{{ end }}`, 0},
		{"else if", `{{ if .X }}{{ else if .Params.summary }}{{ .RenderString (dict) .Params.summary }}{{ end }}`, 0},
		{"if subfield", `{{ if .Params.summary.HTML }}x{{ end }}`, 0},
		{"eq comparison in if predicate", `{{ if eq .Params.summary "default" }}x{{ end }}`, 0},

		// Safe: .RenderString call shapes.
		{"positional with dict", `{{ .RenderString (dict "display" "inline") .Params.summary }}`, 0},
		{"positional bare", `{{ .RenderString .Params.summary }}`, 0},
		{"piped one stage", `{{ .Params.summary | .RenderString }}`, 0},
		{"piped two stages", `{{ .Params.summary | strings.TrimSpace | .RenderString }}`, 0},
		{"subfield positional", `{{ .RenderString (dict) .Params.summary.HTML }}`, 0},
		{"subfield piped", `{{ .Params.summary.HTML | .RenderString }}`, 0},
		{"qualified $.RenderString", `{{ $.RenderString (dict "display" "inline") .Params.summary }}`, 0},
		{"sub-pipeline arg to RenderString", `{{ .RenderString (dict) (printf "wrapper: %s" .Params.summary) }}`, 0},

		// Forbidden: rebinding.
		{"with rebind", `{{ with .Params.summary }}{{ . }}{{ end }}`, 1},
		{"else with rebind", `{{ with .Y }}{{ else with .Params.summary }}{{ . }}{{ end }}`, 1},
		{"range string", `{{ range .Params.summary }}{{ . }}{{ end }}`, 1},

		// Forbidden: variable assignment.
		{"var assign action", `{{ $s := .Params.summary }}`, 1},
		{"var assign in if", `{{ if $s := .Params.summary }}{{ $s }}{{ end }}`, 1},
		{"var assign in with", `{{ with $s := .Params.summary }}{{ $s }}{{ end }}`, 1},
		{"var assign in range", `{{ range $i, $v := .Params.summary }}{{ $v }}{{ end }}`, 1},

		// Forbidden: bare output, no RenderString.
		{"bare action", `{{ .Params.summary }}`, 1},
		{"printf no render", `{{ printf "x is %s" .Params.summary }}`, 1},

		// Forbidden: piped to non-RenderString.
		{"piped to print", `{{ .Params.summary | print "x is" .Page.RenderString }}`, 1},

		// Forbidden: comparison fed to non-render.
		{"co-occurrence in printf without render", `{{ printf "%v %v" (.RenderString "foo") .Params.summary }}`, 1},

		// Case-insensitive: lowercase `.params.summary` is still the same field.
		{"lowercase params", `{{ .params.summary }}`, 1},

		// Comments are stripped by the parser (no ParseComments).
		{"comment mentioning field", `{{/* renders .Params.summary via .RenderString */}}`, 0},

		// String literals are not field references — never flagged.
		{"summary literal inside string", `{{ printf "Warning: .Params.summary missing" .X }}`, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Scan("test.html", tc.template)
			require.NoError(t, err)
			if tc.wantCount != len(got) {
				lines := make([]string, len(got))
				for i, v := range got {
					lines[i] = v.Why
				}
				t.Fatalf("template %q: want %d violations, got %d:\n  %s",
					tc.template, tc.wantCount, len(got), strings.Join(lines, "\n  "))
			}
		})
	}
}

// TestScan_DefineBlock pins that violations inside
// `{{ define "name" }}...{{ end }}` blocks are caught. Hugo
// layouts wrap their content in `{{ define "main" }}` — the
// parser stores the define body in a separate tree in treeSet,
// not in `tree.Root`. A walker that only visits `tree.Root`
// silently passes over the actual layout body.
func TestScan_DefineBlock(t *testing.T) {
	content := `{{ define "main" }}
<p>{{ with .Params.summary }}<span>{{ . }}</span>{{ end }}</p>
{{ end }}`
	got, err := Scan("page.html", content)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Why, "with")
}

// TestScan_QualifiedFieldAccess pins detection of summary
// references that don't start with `.Params`: `$.Params.summary`
// (VariableNode), `.Page.Params.summary` (FieldNode with leading
// qualifier).
func TestScan_QualifiedFieldAccess(t *testing.T) {
	cases := []struct {
		name      string
		template  string
		wantCount int
	}{
		{"dollar-context value reference", `{{ $.Params.summary }}`, 1},
		{"dollar-context piped to RenderString", `{{ $.Params.summary | .RenderString }}`, 0},
		{"Page-qualified bare", `{{ .Page.Params.summary }}`, 1},
		{"Page-qualified through RenderString", `{{ .RenderString (dict) .Page.Params.summary }}`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Scan("file.html", tc.template)
			require.NoError(t, err)
			assert.Len(t, got, tc.wantCount, "violations: %+v", got)
		})
	}
}

// TestScan_ChainReceiver pins detection of summary references
// hidden in the parenthesised receiver of a ChainNode, e.g.
// `{{ (.Params.summary).Foo }}` — the summary lives in the
// PipeNode receiver, not the trailing field chain.
func TestScan_ChainReceiver(t *testing.T) {
	got, err := Scan("file.html", `{{ (.Params.summary).Foo }}`)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

// TestScan_SubPipeVarAssign pins detection of variable assignment
// hidden inside a sub-pipeline argument:
// `{{ .RenderString (dict) ($s := .Params.summary) }}` —
// the outer pipe has no Decl, but the sub-pipe carries `$s := ...`.
func TestScan_SubPipeVarAssign(t *testing.T) {
	got, err := Scan("file.html", `{{ .RenderString (dict) ($s := .Params.summary) }}`)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Why, "variable assignment")
}

// TestScan_TemplateInvocation pins that passing .Params.summary
// into a `{{ template "name" .Params.summary }}` invocation is
// flagged. The sub-template's body sees the value bound to `.`,
// which the scanner cannot follow back to the field.
func TestScan_TemplateInvocation(t *testing.T) {
	cases := []struct {
		name      string
		template  string
		wantCount int
	}{
		{
			"template passes summary as dot",
			`{{ define "foo" }}{{ . }}{{ end }}{{ template "foo" .Params.summary }}`,
			1,
		},
		{
			"template passes unrelated value",
			`{{ define "foo" }}{{ . }}{{ end }}{{ template "foo" .Site.Title }}`,
			0,
		},
		{
			"block with summary as pipe",
			`{{ block "main" .Params.summary }}{{ . }}{{ end }}`,
			1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Scan("file.html", tc.template)
			require.NoError(t, err)
			assert.Len(t, got, tc.wantCount, "violations: %+v", got)
		})
	}
}

// TestScan_MultiLineWith pins multi-line action detection. The
// parser handles newlines inside actions natively; no special
// tokenizer support is needed.
func TestScan_MultiLineWith(t *testing.T) {
	content := "<p>\n{{ with\n  .Params.summary }}\n  <span>{{ . }}</span>\n{{ end }}\n</p>"
	got, err := Scan("file.html", content)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Why, "with")
	// text/template sets the node's Pos to the start of the pipe,
	// so the reported line points at `.Params.summary` on line 3,
	// not the `{{ with` opener on line 2 — more diagnostic because
	// it identifies the offending operand.
	assert.Equal(t, 3, got[0].Line)
}

// TestScan_CRLFAction pins that CRLF line endings inside a
// multi-line action body don't misclassify a presence predicate.
// The text/template lexer accepts any whitespace including \r,
// so the AST is the same whether the file uses LF or CRLF.
func TestScan_CRLFAction(t *testing.T) {
	content := "{{ if\r\n.Params.summary }}{{ .RenderString (dict) .Params.summary }}{{ end }}"
	got, err := Scan("file.html", content)
	require.NoError(t, err)
	assert.Empty(t, got, "CRLF inside a multi-line `if` predicate must not be flagged")
}

// TestScan_UnterminatedActionErrors pins that an unterminated
// `{{` returns a parse error (not silent acceptance). Hugo's own
// build would also fail on such input; Scan surfaces the error
// explicitly rather than swallowing actions.
func TestScan_UnterminatedActionErrors(t *testing.T) {
	_, err := Scan("file.html", `<p>{{ unterminated`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}
