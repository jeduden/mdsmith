package query

import "testing"

var matchTests = []struct {
	name    string
	expr    string
	fm      map[string]any
	want    bool
	wantErr bool
}{
	{
		name: "matching field",
		expr: `status: "✅"`,
		fm:   map[string]any{"status": "✅", "id": 42},
		want: true,
	},
	{
		name: "non-matching field",
		expr: `status: "✅"`,
		fm:   map[string]any{"status": "🔲", "id": 42},
		want: false,
	},
	{
		name: "missing field",
		expr: `status: "✅"`,
		fm:   map[string]any{"id": 42},
		want: false,
	},
	{
		name: "nil front matter",
		expr: `status: "✅"`,
		fm:   nil,
		want: false,
	},
	{
		name: "schema-string proto value",
		expr: `status: "✅"`,
		fm:   map[string]any{"status": `"🔲" | "🔳" | "✅"`},
		want: false,
	},
	{
		name: "compound expression matches",
		expr: `status: "✅", id: >50`,
		fm:   map[string]any{"status": "✅", "id": 60},
		want: true,
	},
	{
		name: "compound expression partial fail",
		expr: `status: "✅", id: >50`,
		fm:   map[string]any{"status": "✅", "id": 30},
		want: false,
	},
	{
		name:    "invalid CUE expression",
		expr:    `status: [[[`,
		fm:      map[string]any{"status": "✅"},
		wantErr: true,
	},
}

func TestMatch(t *testing.T) {
	for _, tt := range matchTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Match(tt.expr, tt.fm)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompile_Valid(t *testing.T) {
	m, err := Compile(`status: "✅"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := m.Match(map[string]any{"status": "✅"})
	if !got {
		t.Fatal("expected match")
	}
}

func TestCompile_Invalid(t *testing.T) {
	_, err := Compile(`status: [[[`)
	if err == nil {
		t.Fatal("expected error for invalid CUE expression")
	}
}
