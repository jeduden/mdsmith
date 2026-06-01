package noreferencestyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// byteIdentityCase pins the EXACT diagnostic output MDS043 produced
// on the regex+second-parse implementation of
// collectReferenceDefinitions, so plan 188's conversion to
// f.LinkReferences() + a byte scanner cannot change any byte of any
// diagnostic. Each want row is the full (Line, Column, Message)
// triple; Severity is always Warning and is asserted separately so a
// row stays readable.
//
// The corpus deliberately exercises only the definition-LOCATING
// path (the part being converted): unused single/multi definitions,
// a duplicate-label pair (goldmark keeps one ref but both source
// lines are flagged unused — load-bearing behaviour the byte scanner
// must reproduce), a 3-space-indented definition (column math), a
// titled definition, a definition inside a fenced code block (must
// be filtered), a destination-less `[x]:` line (goldmark never
// registers it, so neither path may flag it), and a definition
// silenced because a reference-style link is present. Footnotes and
// link rewriting are covered by rule_test.go and are untouched by
// the conversion.
type byteIdentityWant struct {
	line int
	col  int
	msg  string
}

type byteIdentityCase struct {
	name string
	src  string
	want []byteIdentityWant
}

// byteIdentityCases is the pinned corpus. Split across two helpers
// so neither exceeds the funlen limit and the test body stays a thin
// assertion loop.
func byteIdentityCases() []byteIdentityCase {
	return append(byteIdentityCasesA(), byteIdentityCasesB()...)
}

func byteIdentityCasesA() []byteIdentityCase {
	return []byteIdentityCase{
		{
			name: "unused single definition",
			src:  "Plain text.\n\n[unused]: https://example.com\n",
			want: []byteIdentityWant{
				{3, 1, "unused reference definition: [unused]"},
			},
		},
		{
			name: "unused multiple definitions in document order",
			src:  "Plain prose.\n\n[a]: https://x.example\n[b]: https://y.example\n",
			want: []byteIdentityWant{
				{3, 1, "unused reference definition: [a]"},
				{4, 1, "unused reference definition: [b]"},
			},
		},
		{
			name: "duplicate label flags both source lines",
			src:  "Plain prose.\n\n[a]: https://x.example\n[a]: https://y.example\n",
			want: []byteIdentityWant{
				{3, 1, "unused reference definition: [a]"},
				{4, 1, "unused reference definition: [a]"},
			},
		},
	}
}

func byteIdentityCasesB() []byteIdentityCase {
	return []byteIdentityCase{
		{
			name: "three-space-indented definition keeps column on the bracket",
			src:  "Plain prose.\n\n   [ref]: https://example.com\n",
			want: []byteIdentityWant{
				{3, 4, "unused reference definition: [ref]"},
			},
		},
		{
			name: "definition with title",
			src:  "Plain prose.\n\n[ref]: https://example.com \"Title\"\n",
			want: []byteIdentityWant{
				{3, 1, "unused reference definition: [ref]"},
			},
		},
		{
			name: "definition followed by trailing text",
			src:  "abc def ghi.\n\n[unused]: https://example.com\nmore text after.\n",
			want: []byteIdentityWant{
				{3, 1, "unused reference definition: [unused]"},
			},
		},
		{
			name: "definition inside fenced code block is not flagged",
			src:  "Plain prose.\n\n```text\n[ref]: https://example.com\n```\n",
			want: nil,
		},
		{
			name: "destination-less bracket line is never a definition",
			src:  "Plain text.\n\n[broken]:\n",
			want: nil,
		},
		{
			name: "definition silenced when reference-style link present",
			src:  "[a][used] and stuff.\n\n[used]: https://x\n[unused]: https://y\n",
			want: []byteIdentityWant{
				// Only the reference-style link fires; the unused-def
				// pass stays quiet because hasRefLinks is true.
				{1, 1, msgRefLink},
			},
		},
	}
}

func TestMDS043_DefinitionDiagnostics_ByteIdentity(t *testing.T) {
	for _, tc := range byteIdentityCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f, err := lint.NewFile("t.md", []byte(tc.src))
			require.NoError(t, err)
			diags := (&Rule{}).Check(f)
			require.Len(t, diags, len(tc.want),
				"diagnostic count must match the pinned baseline")
			for i, w := range tc.want {
				assert.Equal(t, w.line, diags[i].Line, "diag %d line", i)
				assert.Equal(t, w.col, diags[i].Column, "diag %d column", i)
				assert.Equal(t, w.msg, diags[i].Message, "diag %d message", i)
				assert.Equal(t, "MDS043", diags[i].RuleID, "diag %d rule id", i)
				assert.Equal(t, lint.Warning, diags[i].Severity, "diag %d severity", i)
			}
		})
	}
}
