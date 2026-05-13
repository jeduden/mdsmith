package release

import (
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

// CheckSecretRotations needs a fake `gh` binary so the test
// doesn't shell out to the real one. The fake records its
// invocations into a temp file so the test can assert on them.
func TestCheckSecretRotationsCallsGhForDue(t *testing.T) {
	root := fakeRotationsDir(t, map[string]string{
		"v.md": "---\ntitle: VSCE_PAT\nlastRotated: \"2026-04-01\"\nperiodDays: 30\n" +
			"provider: Azure\nissuerUrl: https://x\nusedBy: r\nscope: s\n---\n",
	})

	// Build a tiny fake `gh` that always returns empty JSON for
	// `issue list` (so existingOpenIssue returns nil), exits 0
	// otherwise, and appends every invocation to LOG_FILE.
	logFile := filepath.Join(t.TempDir(), "gh.log")
	fakeGh := filepath.Join(t.TempDir(), "fake-gh.sh")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> \"$LOG_FILE\"\n" +
		"if [ \"$1\" = \"issue\" ] && [ \"$2\" = \"list\" ]; then\n" +
		"  echo '[]'\n" +
		"fi\n" +
		"exit 0\n"
	require.NoError(t, os.WriteFile(fakeGh, []byte(script), 0o755))
	t.Setenv("LOG_FILE", logFile)

	// 60 days after lastRotated; periodDays=30, so the entry is
	// overdue.
	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	res, err := CheckSecretRotations(root, CheckRotationsOptions{
		Now:       now,
		GHCommand: fakeGh,
		Env:       MapEnviron{},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"VSCE_PAT"}, res.Opened)
	assert.Empty(t, res.Skipped)

	log, err := os.ReadFile(logFile)
	require.NoError(t, err)
	// Confirm both the search and the create happened.
	assert.Contains(t, string(log), "issue list")
	assert.Contains(t, string(log), "issue create")
	assert.Contains(t, string(log), "label create")
}
