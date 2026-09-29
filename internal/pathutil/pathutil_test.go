package pathutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAbsOrDriveOrUNC(t *testing.T) {
	cases := []struct {
		name string
		p    string
		want bool
	}{
		{"empty", "", false},
		{"relative", "docs/api.md", false},
		{"relative backslash", `docs\api.md`, false},
		{"single char", "a", false},
		{"digit before colon", "1:x", false},
		{"posix absolute", "/etc/passwd", true},
		{"root only", "/", true},
		{"forward-slash UNC", "//server/share", true},
		// Backslashes count as separators on every host, so the raw
		// Windows forms are caught without the caller normalizing first.
		{"backslash UNC", `\\server\share`, true},
		{"backslash root-relative", `\notes.md`, true},
		{"drive forward slash", "C:/Windows", true},
		{"drive backslash", `C:\Windows`, true},
		{"lowercase drive", "c:/x", true},
		{"bare drive", "C:", true},
		// Drive-relative (`C:x` resolves against drive C's cwd): not
		// workspace-relative either, so it is rejected on purpose.
		{"drive relative", "a:b.md", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsAbsOrDriveOrUNC(tc.p))
		})
	}
}
