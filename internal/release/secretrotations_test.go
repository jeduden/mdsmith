package release

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsISODate(t *testing.T) {
	cases := map[string]bool{
		"2026-05-13": true,
		"2026-02-31": false, // calendar-invalid; Go's time.Parse rejects this
		"2026-5-13":  false,
		"not-a-date": false,
		"":           false,
		"2026-13-01": false,
		"2026-00-15": false,
		"2026-05-32": false,
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, want, IsISODate(input))
		})
	}
}

func TestUTCToday(t *testing.T) {
	// 19:30 in any timezone should resolve to UTC midnight on the
	// same UTC date.
	now := time.Date(2026, 5, 13, 19, 30, 0, 0, time.UTC)
	got := UTCToday(now)
	assert.Equal(t, 0, got.Hour())
	assert.Equal(t, 0, got.Minute())
	assert.Equal(t, time.UTC, got.Location())
	assert.Equal(t, 13, got.Day())
	assert.Equal(t, time.May, got.Month())
	assert.Equal(t, 2026, got.Year())
}

func TestDaysBetween(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	assert.Equal(t, 0, DaysBetween(day(2026, 1, 1), day(2026, 1, 1)))
	assert.Equal(t, 7, DaysBetween(day(2026, 1, 8), day(2026, 1, 1)))
	assert.Equal(t, -3, DaysBetween(day(2026, 1, 7), day(2026, 1, 10)))
}

func TestComputeDueState(t *testing.T) {
	last := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// dueOn = 2026-01-31 (30 days after lastRotated)
	periodDays := 30

	t.Run("ok well before window", func(t *testing.T) {
		got := ComputeDueState(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), last, periodDays)
		assert.Equal(t, DueOK, got.Status)
	})
	t.Run("due within window", func(t *testing.T) {
		got := ComputeDueState(time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), last, periodDays)
		assert.Equal(t, DueDue, got.Status)
		assert.Equal(t, 15, got.DaysUntilDue)
	})
	t.Run("due today is daysUntilDue=0", func(t *testing.T) {
		got := ComputeDueState(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), last, periodDays)
		assert.Equal(t, DueDue, got.Status)
		assert.Equal(t, 0, got.DaysUntilDue)
	})
	t.Run("overdue with negative daysUntilDue", func(t *testing.T) {
		got := ComputeDueState(time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC), last, periodDays)
		assert.Equal(t, DueOverdue, got.Status)
		assert.Equal(t, -5, got.DaysUntilDue)
	})
}

func TestParseFrontMatter(t *testing.T) {
	t.Run("well-formed", func(t *testing.T) {
		fm, err := ParseFrontMatter("---\ntitle: VSCE_PAT\nperiodDays: 335\n---\n# body\n", "test.md")
		require.NoError(t, err)
		assert.Equal(t, "VSCE_PAT", fm["title"])
		assert.EqualValues(t, 335, fm["periodDays"])
	})
	t.Run("no front matter", func(t *testing.T) {
		_, err := ParseFrontMatter("no front matter here\n", "test.md")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no front matter")
	})
	t.Run("unterminated", func(t *testing.T) {
		_, err := ParseFrontMatter("---\ntitle: X\nbody but no close\n", "test.md")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unterminated front matter")
	})
	t.Run("not a mapping", func(t *testing.T) {
		_, err := ParseFrontMatter("---\n- list\n- of\n- scalars\n---\n", "test.md")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a mapping")
	})
}

func TestRepoURL(t *testing.T) {
	t.Run("uses env vars", func(t *testing.T) {
		got := repoURL(MapEnviron{
			"GITHUB_SERVER_URL": "https://github.example.com",
			"GITHUB_REPOSITORY": "acme/widget",
		})
		assert.Equal(t, "https://github.example.com/acme/widget", got)
	})
	t.Run("strips trailing slashes from server", func(t *testing.T) {
		got := repoURL(MapEnviron{
			"GITHUB_SERVER_URL": "https://github.example.com///",
			"GITHUB_REPOSITORY": "acme/widget",
		})
		assert.Equal(t, "https://github.example.com/acme/widget", got)
	})
	t.Run("falls back when empty", func(t *testing.T) {
		got := repoURL(MapEnviron{})
		assert.Equal(t, "https://github.com/jeduden/mdsmith", got)
	})
}

func TestIssueBody(t *testing.T) {
	entry := RotationEntry{
		Title:       "VSCE_PAT",
		LastRotated: "2026-05-12",
		PeriodDays:  335,
		Provider:    "Azure DevOps",
		IssuerURL:   "https://dev.azure.com",
		UsedBy:      "release.yml",
		Scope:       "Marketplace > Manage",
	}
	env := MapEnviron{}
	t.Run("overdue headline negates daysUntilDue", func(t *testing.T) {
		body := IssueBody(entry, "vsce-pat.md", DueResult{Status: DueOverdue, DaysUntilDue: -7}, env)
		assert.Contains(t, body, "OVERDUE by 7 days")
	})
	t.Run("due today", func(t *testing.T) {
		body := IssueBody(entry, "vsce-pat.md", DueResult{Status: DueDue, DaysUntilDue: 0}, env)
		assert.Contains(t, body, "is due today")
	})
	t.Run("due in N days", func(t *testing.T) {
		body := IssueBody(entry, "vsce-pat.md", DueResult{Status: DueDue, DaysUntilDue: 15}, env)
		assert.Contains(t, body, "is due in 15 days")
	})
	t.Run("renders field table", func(t *testing.T) {
		body := IssueBody(entry, "vsce-pat.md", DueResult{Status: DueDue, DaysUntilDue: 5}, env)
		assert.Contains(t, body, "| Provider | Azure DevOps |")
		assert.Contains(t, body, "| Period (days) | 335 |")
		assert.Contains(t, body, "vsce-pat.md")
	})
}

func TestValidateRotationEntry(t *testing.T) {
	good := map[string]any{
		"title":       "VSCE_PAT",
		"lastRotated": "2026-05-12",
		"periodDays":  335,
		"provider":    "Azure DevOps",
		"issuerUrl":   "https://dev.azure.com",
		"usedBy":      "release.yml",
		"scope":       "Marketplace > Manage",
	}
	t.Run("accepts a complete entry", func(t *testing.T) {
		out, err := ValidateRotationEntry(good, "vsce-pat.md")
		require.NoError(t, err)
		assert.Equal(t, "VSCE_PAT", out.Title)
		assert.Equal(t, 335, out.PeriodDays)
	})
	t.Run("rejects missing required key", func(t *testing.T) {
		fm := copyMap(good)
		delete(fm, "title")
		_, err := ValidateRotationEntry(fm, "x.md")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "x.md")
		assert.Contains(t, err.Error(), "title")
	})
	t.Run("rejects calendar-invalid lastRotated", func(t *testing.T) {
		fm := copyMap(good)
		fm["lastRotated"] = "2026-02-31"
		_, err := ValidateRotationEntry(fm, "x.md")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "lastRotated")
		assert.Contains(t, err.Error(), "ISO-8601")
	})
	t.Run("rejects non-integer periodDays", func(t *testing.T) {
		fm := copyMap(good)
		fm["periodDays"] = "soon"
		_, err := ValidateRotationEntry(fm, "x.md")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "periodDays")
		assert.Contains(t, err.Error(), "integer")
	})
	t.Run("rejects non-positive periodDays", func(t *testing.T) {
		for _, bad := range []int{0, -1} {
			fm := copyMap(good)
			fm["periodDays"] = bad
			_, err := ValidateRotationEntry(fm, "x.md")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "periodDays")
			assert.Contains(t, err.Error(), "positive")
		}
	})
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// --- record-rotation logic ---

func TestSplitFrontMatter(t *testing.T) {
	t.Run("well-formed", func(t *testing.T) {
		text := "---\ntitle: VSCE_PAT\nperiodDays: 335\n---\n# body\n"
		out, err := splitFrontMatter(text, "test.md")
		require.NoError(t, err)
		assert.Equal(t, "---\n", out.Opening)
		assert.Equal(t, "title: VSCE_PAT\nperiodDays: 335", out.YAMLBlock)
		assert.Equal(t, "\n---\n# body\n", out.ClosingPlusBody)
		assert.Equal(t, text, out.Opening+out.YAMLBlock+out.ClosingPlusBody)
	})
	t.Run("no front matter", func(t *testing.T) {
		_, err := splitFrontMatter("no front matter here\n", "test.md")
		require.Error(t, err)
	})
	t.Run("unterminated", func(t *testing.T) {
		_, err := splitFrontMatter("---\ntitle: X\nno close\n", "test.md")
		require.Error(t, err)
	})
}

func TestUpdateLastRotated(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bare unquoted",
			in:   "title: VSCE_PAT\nlastRotated: 2026-04-01\nperiodDays: 335",
			want: "title: VSCE_PAT\nlastRotated: \"2026-05-12\"\nperiodDays: 335",
		},
		{
			name: "double-quoted",
			in:   "lastRotated: \"2026-04-01\"\nperiodDays: 335",
			want: "lastRotated: \"2026-05-12\"\nperiodDays: 335",
		},
		{
			name: "single-quoted normalized to double",
			in:   "lastRotated: '2026-04-01'\nperiodDays: 335",
			want: "lastRotated: \"2026-05-12\"\nperiodDays: 335",
		},
		{
			name: "preserves trailing inline comment",
			in:   "lastRotated: 2026-04-01 # rotated after incident\nperiodDays: 335",
			want: "lastRotated: \"2026-05-12\" # rotated after incident\nperiodDays: 335",
		},
		{
			name: "tolerates leading indent",
			in:   "  lastRotated: 2026-04-01\n  periodDays: 335",
			want: "  lastRotated: \"2026-05-12\"\n  periodDays: 335",
		},
		{
			name: "no-op when already requested date",
			in:   "lastRotated: \"2026-05-12\"\nperiodDays: 335",
			want: "lastRotated: \"2026-05-12\"\nperiodDays: 335",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := UpdateLastRotated(tc.in, "2026-05-12", "vsce-pat.md")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	t.Run("missing lastRotated line", func(t *testing.T) {
		_, err := UpdateLastRotated("title: X\nperiodDays: 30", "2026-05-12", "x.md")
		require.Error(t, err)
	})
}

// --- filesystem-backed FindEntry + RecordRotation ---

// fakeRotationsDir creates a temp directory laid out like
// docs/development/secret-rotations/ under a synthetic repo root,
// populated with the given per-secret files. Returns the repo
// root so callers can pass it directly to RecordRotation /
// CheckSecretRotations.
func fakeRotationsDir(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, RotationsDirName)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	return root
}

func TestFindEntry(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"vsce-pat.md": "---\ntitle: VSCE_PAT\nlastRotated: \"2026-05-12\"\nperiodDays: 335\n---\nbody\n",
		"ovsx-pat.md": "---\ntitle: OVSX_PAT\nlastRotated: \"2026-05-12\"\nperiodDays: 335\n---\nbody\n",
	})
	dir := filepath.Join(root, RotationsDirName)
	t.Run("finds known title", func(t *testing.T) {
		res, err := FindEntry(dir, "VSCE_PAT")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "vsce-pat.md"), res.Path)
		assert.Equal(t, []string{"OVSX_PAT", "VSCE_PAT"}, res.Titles)
	})
	t.Run("unknown title surfaces known list", func(t *testing.T) {
		_, err := FindEntry(dir, "MISSING")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown title")
		assert.Contains(t, err.Error(), "OVSX_PAT")
		assert.Contains(t, err.Error(), "VSCE_PAT")
	})
}

func TestFindEntryRejectsMalformed(t *testing.T) {
	t.Run("missing front matter", func(t *testing.T) {
		root := fakeRotationsDir(t, map[string]string{"bad.md": "no front matter here\n"})
		_, err := FindEntry(filepath.Join(root, RotationsDirName), "X")
		require.Error(t, err)
	})
	t.Run("unterminated front matter", func(t *testing.T) {
		root := fakeRotationsDir(t, map[string]string{"bad.md": "---\ntitle: X\nno close\n"})
		_, err := FindEntry(filepath.Join(root, RotationsDirName), "X")
		require.Error(t, err)
	})
	t.Run("duplicate titles fail loudly", func(t *testing.T) {
		root := fakeRotationsDir(t, map[string]string{
			"a.md": "---\ntitle: SAME\nlastRotated: \"2026-05-12\"\nperiodDays: 335\n---\n",
			"b.md": "---\ntitle: SAME\nlastRotated: \"2026-05-12\"\nperiodDays: 335\n---\n",
		})
		_, err := FindEntry(filepath.Join(root, RotationsDirName), "SAME")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate title")
	})
}

func TestRecordRotation(t *testing.T) {
	t.Run("rewrites the file in place", func(t *testing.T) {
		const before = "---\n" +
			"title: VSCE_PAT\n" +
			"lastRotated: \"2026-04-01\"\n" +
			"periodDays: 335\n" +
			"---\nbody\n"
		root := fakeRotationsDir(t, map[string]string{"vsce-pat.md": before})
		changed, err := RecordRotation(root, "VSCE_PAT", "2026-05-12")
		require.NoError(t, err)
		assert.True(t, changed)
		got, err := os.ReadFile(filepath.Join(root, RotationsDirName, "vsce-pat.md"))
		require.NoError(t, err)
		assert.Contains(t, string(got), `lastRotated: "2026-05-12"`)
	})
	t.Run("no-op when date already matches", func(t *testing.T) {
		const before = "---\n" +
			"title: VSCE_PAT\n" +
			"lastRotated: \"2026-05-12\"\n" +
			"periodDays: 335\n" +
			"---\nbody\n"
		root := fakeRotationsDir(t, map[string]string{"vsce-pat.md": before})
		changed, err := RecordRotation(root, "VSCE_PAT", "2026-05-12")
		require.NoError(t, err)
		assert.False(t, changed)
	})
	t.Run("rejects calendar-invalid date", func(t *testing.T) {
		root := fakeRotationsDir(t, map[string]string{
			"v.md": "---\ntitle: V\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n---\n",
		})
		_, err := RecordRotation(root, "V", "2026-02-31")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ISO-8601")
	})
}

// --- LoadRotations and CheckSecretRotations smoke test ---

func TestLoadRotationsSortsByTitle(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: VSCE_PAT\nlastRotated: \"2026-05-12\"\nperiodDays: 335\n" +
			"provider: Azure\nissuerUrl: https://x\nusedBy: r\nscope: s\n---\n",
		"o.md": "---\ntitle: OVSX_PAT\nlastRotated: \"2026-05-12\"\nperiodDays: 335\n" +
			"provider: OVSX\nissuerUrl: https://x\nusedBy: r\nscope: s\n---\n",
	})
	got, err := LoadRotations(filepath.Join(root, RotationsDirName))
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "OVSX_PAT", got[0].Entry.Title)
	assert.Equal(t, "VSCE_PAT", got[1].Entry.Title)
}

func TestLoadRotationsRejectsBadPeriodDays(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: VSCE_PAT\nlastRotated: \"2026-05-12\"\nperiodDays: 0\n" +
			"provider: Azure\nissuerUrl: https://x\nusedBy: r\nscope: s\n---\n",
	})
	_, err := LoadRotations(filepath.Join(root, RotationsDirName))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "positive")
}

// recordingGH returns a GHRunner that records each invocation
// into the provided slice. `responses` maps the first arg of
// the call (e.g. "issue") to the canned stdout for that call.
// `errs` maps the first two args joined by a space (e.g.
// "issue list", "label create") to an error to return for that
// specific call — used to exercise per-call failure branches.
// `err` is a blanket error that fires on every call.
type recordingGH struct {
	calls     [][]string
	responses map[string][]byte
	errs      map[string]error
	err       error
}

func (r *recordingGH) Run(args []string) ([]byte, error) {
	r.calls = append(r.calls, args)
	if r.err != nil {
		return nil, r.err
	}
	if r.errs != nil && len(args) >= 2 {
		if e, ok := r.errs[args[0]+" "+args[1]]; ok {
			return nil, e
		}
	}
	if r.responses != nil && len(args) > 0 {
		if out, ok := r.responses[args[0]]; ok {
			return out, nil
		}
	}
	return nil, nil
}

// CheckSecretRotations uses an injected GHRunner so tests
// never touch the real `gh` binary or rely on the runner
// filesystem letting a temp shell script execute.
func TestCheckSecretRotationsCallsGhForDue(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: VSCE_PAT\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n" +
			"provider: Azure\nissuerUrl: https://x\nusedBy: r\nscope: s\n---\n",
	})
	gh := &recordingGH{responses: map[string][]byte{"issue": []byte("[]")}}

	// 60 days after lastRotated; periodDays=30, so the entry is
	// overdue.
	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	res, err := CheckSecretRotations(root, CheckRotationsOptions{
		Now: now,
		GH:  gh.Run,
		Env: MapEnviron{},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"VSCE_PAT"}, res.Opened)
	assert.Empty(t, res.Skipped)

	// Confirm the search, label-create, and issue-create all happened.
	var saw struct{ list, label, create bool }
	for _, c := range gh.calls {
		if len(c) < 2 {
			continue
		}
		switch {
		case c[0] == "issue" && c[1] == "list":
			saw.list = true
		case c[0] == "issue" && c[1] == "create":
			saw.create = true
		case c[0] == "label" && c[1] == "create":
			saw.label = true
		}
	}
	assert.True(t, saw.list, "issue list never called: %v", gh.calls)
	assert.True(t, saw.create, "issue create never called: %v", gh.calls)
	assert.True(t, saw.label, "label create never called: %v", gh.calls)
}

func TestAsStringTypes(t *testing.T) {
	_, err := asString(42)
	assert.Error(t, err)
}

func TestAsIntTypes(t *testing.T) {
	cases := []struct {
		name    string
		input   any
		want    int
		wantErr bool
	}{
		{"int", int(7), 7, false},
		{"int64", int64(7), 7, false},
		{"float64 whole", float64(7), 7, false},
		{"float64 fractional", float64(7.5), 0, true},
		{"string numeric", "7", 7, false},
		{"string non-numeric", "soon", 0, true},
		{"unsupported type", []int{1}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := asInt(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestOSEnvironGetenv(t *testing.T) {
	t.Setenv("MDSMITH_RELEASE_TEST_VAR", "value-from-env")
	assert.Equal(t, "value-from-env", osEnviron{}.Getenv("MDSMITH_RELEASE_TEST_VAR"))
}

func TestIssueBodyDefaultsEnvWhenNil(t *testing.T) {
	body := IssueBody(RotationEntry{Title: "X"}, "x.md",
		DueResult{Status: DueDue, DaysUntilDue: 5}, nil)
	assert.Contains(t, body, "is due in 5 days")
}

// TestCheckSecretRotationsSkipsExistingIssue arranges the fake
// GHRunner to return one matching issue from `issue list`,
// exercising the "skipped" branch of the main loop.
func TestCheckSecretRotationsSkipsExistingIssue(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: VSCE_PAT\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n" +
			"provider: Azure\nissuerUrl: https://x\nusedBy: r\nscope: s\n---\n",
	})
	gh := &recordingGH{responses: map[string][]byte{
		"issue": []byte(`[{"number":99,"title":"Rotate VSCE_PAT (lastRotated 2026-04-01)"}]`),
	}}

	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	res, err := CheckSecretRotations(root, CheckRotationsOptions{
		Now: now, GH: gh.Run, Env: MapEnviron{},
	})
	require.NoError(t, err)
	assert.Empty(t, res.Opened)
	assert.Equal(t, []string{"VSCE_PAT"}, res.Skipped)
}

// TestCheckSecretRotationsPassesAssignee verifies that when
// REMINDER_ASSIGNEE is set, the createIssue call gets an
// --assignee flag.
func TestCheckSecretRotationsPassesAssignee(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: VSCE_PAT\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n" +
			"provider: Azure\nissuerUrl: https://x\nusedBy: r\nscope: s\n---\n",
	})
	gh := &recordingGH{responses: map[string][]byte{"issue": []byte("[]")}}

	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	_, err := CheckSecretRotations(root, CheckRotationsOptions{
		Now: now, GH: gh.Run,
		Env: MapEnviron{"REMINDER_ASSIGNEE": "octocat"},
	})
	require.NoError(t, err)
	// Find the `issue create` call and confirm the --assignee
	// flag is passed.
	var foundAssignee bool
	for _, c := range gh.calls {
		if len(c) >= 2 && c[0] == "issue" && c[1] == "create" {
			for i, a := range c {
				if a == "--assignee" && i+1 < len(c) && c[i+1] == "octocat" {
					foundAssignee = true
				}
			}
		}
	}
	assert.True(t, foundAssignee, "issue create never carried --assignee octocat: %v", gh.calls)
}

// TestCheckSecretRotationsSurfacesGhFailure covers the
// existingOpenIssue error branch when `gh issue list` returns
// an error, plus the propagation through CheckSecretRotations.
func TestCheckSecretRotationsSurfacesGhFailure(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: VSCE_PAT\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n" +
			"provider: Azure\nissuerUrl: https://x\nusedBy: r\nscope: s\n---\n",
	})
	gh := &recordingGH{err: errors.New("simulated gh failure")}

	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	_, err := CheckSecretRotations(root, CheckRotationsOptions{
		Now: now, GH: gh.Run, Env: MapEnviron{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gh issue list")
}

// TestCheckSecretRotationsRejectsMissingRotationsDir covers the
// "no per-secret files found" branch.
func TestCheckSecretRotationsRejectsMissingRotationsDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, RotationsDirName), 0o755))
	_, err := CheckSecretRotations(root, CheckRotationsOptions{
		Now: time.Now(), GH: (&recordingGH{}).Run, Env: MapEnviron{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no per-secret files")
}

// TestRecordRotationPropagatesSplitFrontMatterError drives the
// splitFrontMatter failure branch in RecordRotation. FindEntry has
// to parse valid front matter first, so the fault is staged on the
// second read: FindEntry sees the real, well-formed file, then the
// re-read inside RecordRotation returns a body whose front matter
// is unterminated, which splitFrontMatter rejects.
func TestRecordRotationPropagatesSplitFrontMatterError(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: V\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n---\n",
	})
	var n int
	withFakeOSFuncs(t, fakeOSFuncs{
		readFile: func(p string) ([]byte, error) {
			n++
			if n == 1 {
				return os.ReadFile(p) // FindEntry reads the real file
			}
			// RecordRotation's own read: opener present but never
			// closed, so splitFrontMatter reports unterminated.
			return []byte("---\ntitle: V\nlastRotated: \"2026-04-01\"\n"), nil
		},
	})
	_, err := RecordRotation(root, "V", "2026-05-12")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unterminated front matter")
}

// TestCheckSecretRotationsPropagatesLoadError drives the
// LoadRotations failure branch in CheckSecretRotations (distinct
// from the empty-but-present "no per-secret files" branch): a
// rotations directory that does not exist makes the underlying
// ReadDir fail, and the error must propagate.
func TestCheckSecretRotationsPropagatesLoadError(t *testing.T) {
	// t.TempDir() has no docs/development/secret-rotations subtree,
	// so LoadRotations' ReadDir fails before any classification.
	root := t.TempDir()
	_, err := CheckSecretRotations(root, CheckRotationsOptions{
		Now: time.Now(), GH: (&recordingGH{}).Run, Env: MapEnviron{},
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "no per-secret files",
		"a missing dir must surface the ReadDir error, not the empty-dir message")
}

// TestRecordRotationUnknownTitle covers the FindEntry error
// propagation back through RecordRotation.
func TestRecordRotationUnknownTitle(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: VSCE_PAT\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n---\n",
	})
	_, err := RecordRotation(root, "MISSING", "2026-05-12")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown title")
}

// TestRecordRotationRejectsMissingLastRotatedLine covers the
// UpdateLastRotated failure path through RecordRotation when
// the file lacks a `lastRotated:` line.
func TestRecordRotationRejectsMissingLastRotatedLine(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: X\nperiodDays: 30\n---\nno lastRotated key\n",
	})
	_, err := RecordRotation(root, "X", "2026-05-12")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not locate `lastRotated:`")
}

// TestLoadRotationsHandlesMissingDir covers the os.ReadDir
// error branch when the directory does not exist.
func TestLoadRotationsHandlesMissingDir(t *testing.T) {
	_, err := LoadRotations(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

func TestFindEntryMissingTitleKey(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\nperiodDays: 30\n---\n",
	})
	_, err := FindEntry(filepath.Join(root, RotationsDirName), "X")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`title` is missing")
}

func TestFindEntryNonStringTitle(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: 42\nperiodDays: 30\n---\n",
	})
	_, err := FindEntry(filepath.Join(root, RotationsDirName), "42")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`title` is not a string")
}

func TestLoadRotationsPropagatesFrontMatterError(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"bad.md": "no front matter at all\n",
	})
	_, err := LoadRotations(filepath.Join(root, RotationsDirName))
	require.Error(t, err)
}

func TestLoadRotationsPropagatesEntryValidationError(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		// Required keys all present but periodDays is zero,
		// which validateRotationEntry rejects.
		"bad.md": "---\ntitle: X\nlastRotated: \"2026-05-12\"\nperiodDays: 0\n" +
			"provider: P\nissuerUrl: u\nusedBy: r\nscope: s\n---\n",
	})
	_, err := LoadRotations(filepath.Join(root, RotationsDirName))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "positive")
}

// TestCheckSecretRotationsSurfacesLabelCreateFailure exercises
// the ensureLabel error branch: `issue list` succeeds, but
// `label create` fails — CheckSecretRotations must propagate.
func TestCheckSecretRotationsSurfacesLabelCreateFailure(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: VSCE_PAT\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n" +
			"provider: Azure\nissuerUrl: https://x\nusedBy: r\nscope: s\n---\n",
	})
	gh := &recordingGH{
		responses: map[string][]byte{"issue": []byte("[]")},
		errs:      map[string]error{"label create": errors.New("label boom")},
	}
	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	_, err := CheckSecretRotations(root, CheckRotationsOptions{
		Now: now, GH: gh.Run, Env: MapEnviron{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gh label create")
}

// TestCheckSecretRotationsSurfacesIssueCreateFailure exercises
// the createIssue error branch.
func TestCheckSecretRotationsSurfacesIssueCreateFailure(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: VSCE_PAT\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n" +
			"provider: Azure\nissuerUrl: https://x\nusedBy: r\nscope: s\n---\n",
	})
	gh := &recordingGH{
		responses: map[string][]byte{"issue": []byte("[]")},
		errs:      map[string]error{"issue create": errors.New("create boom")},
	}
	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	_, err := CheckSecretRotations(root, CheckRotationsOptions{
		Now: now, GH: gh.Run, Env: MapEnviron{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gh issue create")
}

// TestExistingOpenIssueRejectsBadJSON covers the JSON parse
// error branch in existingOpenIssue when `gh issue list`
// returns malformed output.
func TestExistingOpenIssueRejectsBadJSON(t *testing.T) {
	gh := &recordingGH{responses: map[string][]byte{"issue": []byte("not json")}}
	_, err := existingOpenIssue(gh.Run, "Rotate VSCE_PAT (lastRotated 2026-04-01)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing JSON")
}

func TestCheckSecretRotationsDefaultsGHAndEnv(t *testing.T) {
	// All entries are "ok" so the loop never calls GH; this lets
	// us hit the GH=nil → ExecGH default and Env=nil → osEnviron
	// default branches without shelling out to a real `gh`.
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: V\nlastRotated: \"2026-05-12\"\nperiodDays: 4000\n" +
			"provider: P\nissuerUrl: u\nusedBy: r\nscope: s\n---\n",
	})
	res, err := CheckSecretRotations(root, CheckRotationsOptions{
		Now: time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	assert.Empty(t, res.Opened)
	assert.Empty(t, res.Skipped)
}

// TestParseFrontMatterRejectsInvalidYAML exercises the
// yaml.Unmarshal error branch — the front matter fences are
// present but the body is not valid YAML.
func TestParseFrontMatterRejectsInvalidYAML(t *testing.T) {
	_, err := ParseFrontMatter("---\n: : :\n---\n", "x.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid YAML")
}

// TestValidateRotationEntryRejectsNonStringLastRotated covers
// the asString error branch on lastRotated when YAML decodes
// the value as a non-string (e.g. an integer).
func TestValidateRotationEntryRejectsNonStringLastRotated(t *testing.T) {
	fm := map[string]any{
		"title":       "X",
		"lastRotated": 12345,
		"periodDays":  30,
		"provider":    "P",
		"issuerUrl":   "u",
		"usedBy":      "r",
		"scope":       "s",
	}
	_, err := ValidateRotationEntry(fm, "x.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lastRotated")
	assert.Contains(t, err.Error(), "not a string")
}

// TestRecordRotationPropagatesWriteError covers the
// os.WriteFile failure path of RecordRotation by making the
// per-secret directory read-only after FindEntry succeeds.
func TestRecordRotationPropagatesWriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; chmod 0o555 is bypassed")
	}
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: V\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n---\n",
	})
	dir := filepath.Join(root, RotationsDirName)
	// Make the file unwritable. Restore at the end so t.TempDir
	// cleanup can delete it.
	target := filepath.Join(dir, "v.md")
	require.NoError(t, os.Chmod(target, 0o444))
	t.Cleanup(func() { _ = os.Chmod(target, 0o644) })

	_, err := RecordRotation(root, "V", "2026-05-12")
	require.Error(t, err)
}

// withFakeOSFuncs lets a test swap the fs* function variables for
// a single test and restore them automatically. Each non-nil
// function in the fakes overrides; nil falls through to the
// real os.* call so a test that only wants to fail one of the
// three operations doesn't have to re-implement the others.
type fakeOSFuncs struct {
	readDir   func(string) ([]os.DirEntry, error)
	readFile  func(string) ([]byte, error)
	writeFile func(string, []byte, os.FileMode) error
}

func withFakeOSFuncs(t *testing.T, ff fakeOSFuncs) {
	t.Helper()
	prevRD, prevRF, prevWF := fsReadDir, fsReadFile, fsWriteFile
	if ff.readDir != nil {
		fsReadDir = ff.readDir
	}
	if ff.readFile != nil {
		fsReadFile = ff.readFile
	}
	if ff.writeFile != nil {
		fsWriteFile = ff.writeFile
	}
	t.Cleanup(func() {
		fsReadDir, fsReadFile, fsWriteFile = prevRD, prevRF, prevWF
	})
}

func TestLoadRotationsPropagatesReadFileError(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: V\nlastRotated: \"2026-05-12\"\nperiodDays: 30\n" +
			"provider: P\nissuerUrl: u\nusedBy: r\nscope: s\n---\n",
	})
	withFakeOSFuncs(t, fakeOSFuncs{
		readFile: func(string) ([]byte, error) { return nil, errors.New("read boom") },
	})
	_, err := LoadRotations(filepath.Join(root, RotationsDirName))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read boom")
}

func TestFindEntryPropagatesReadFileError(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: V\nlastRotated: \"2026-05-12\"\nperiodDays: 30\n---\n",
	})
	withFakeOSFuncs(t, fakeOSFuncs{
		readFile: func(string) ([]byte, error) { return nil, errors.New("read boom") },
	})
	_, err := FindEntry(filepath.Join(root, RotationsDirName), "V")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read boom")
}

func TestRecordRotationPropagatesReadFileError(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: V\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n" +
			"provider: P\nissuerUrl: u\nusedBy: r\nscope: s\n---\n",
	})
	// FindEntry has to succeed first (real read), so swap only
	// fsReadFile to fail after FindEntry has called it. Simplest:
	// fail every call; FindEntry's read also errors and the test
	// still hits a return path — but that's the FindEntry branch
	// rather than RecordRotation's. Use a counter: succeed for
	// the first N reads (FindEntry's), then fail.
	var n int
	withFakeOSFuncs(t, fakeOSFuncs{
		readFile: func(p string) ([]byte, error) {
			n++
			if n == 1 {
				return os.ReadFile(p) // let FindEntry through
			}
			return nil, errors.New("read boom")
		},
	})
	_, err := RecordRotation(root, "V", "2026-05-12")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read boom")
}

func TestRecordRotationPropagatesWriteFileError(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: V\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n" +
			"provider: P\nissuerUrl: u\nusedBy: r\nscope: s\n---\n",
	})
	withFakeOSFuncs(t, fakeOSFuncs{
		writeFile: func(string, []byte, os.FileMode) error {
			return errors.New("write boom")
		},
	})
	_, err := RecordRotation(root, "V", "2026-05-12")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write boom")
}

func TestValidateRotationEntryRejectsQuoteInTitle(t *testing.T) {
	fm := map[string]any{
		"title":       `bad"title`,
		"lastRotated": "2026-05-12",
		"periodDays":  30,
		"provider":    "P",
		"issuerUrl":   "u",
		"usedBy":      "r",
		"scope":       "s",
	}
	_, err := ValidateRotationEntry(fm, "x.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "double-quote")
}

// TestLoadRotationsSkipsNonMarkdown covers the continue branch
// of LoadRotations when the directory holds a non-.md file
// alongside the valid per-secret files.
func TestLoadRotationsSkipsNonMarkdown(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: V\nlastRotated: \"2026-05-12\"\nperiodDays: 30\n" +
			"provider: P\nissuerUrl: u\nusedBy: r\nscope: s\n---\n",
		"README.txt": "not a per-secret file\n",
	})
	got, err := LoadRotations(filepath.Join(root, RotationsDirName))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "V", got[0].Entry.Title)
}

// TestFindEntrySkipsNonMarkdown covers the continue branch of
// FindEntry when the directory holds a non-.md file alongside
// the valid per-secret files.
func TestFindEntrySkipsNonMarkdown(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md":       "---\ntitle: V\nlastRotated: \"2026-05-12\"\nperiodDays: 30\n---\n",
		"README.txt": "not a per-secret file\n",
	})
	res, err := FindEntry(filepath.Join(root, RotationsDirName), "V")
	require.NoError(t, err)
	assert.Contains(t, res.Path, "v.md")
}

// TestFindEntryPropagatesReadDirError mirrors the LoadRotations
// missing-dir test for FindEntry.
func TestFindEntryPropagatesReadDirError(t *testing.T) {
	_, err := FindEntry(filepath.Join(t.TempDir(), "missing"), "V")
	require.Error(t, err)
}

func TestValidateRotationEntryRejectsNonStringTitle(t *testing.T) {
	fm := map[string]any{
		"title":       12345,
		"lastRotated": "2026-05-12",
		"periodDays":  30,
		"provider":    "P",
		"issuerUrl":   "u",
		"usedBy":      "r",
		"scope":       "s",
	}
	_, err := ValidateRotationEntry(fm, "x.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title")
	assert.Contains(t, err.Error(), "not a string")
}
