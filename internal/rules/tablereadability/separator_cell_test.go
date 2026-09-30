package tablereadability

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsSeparatorCell_MatchesRegexp pins the hand-rolled scanner to the
// regexp it replaces (^:?-+:?$) on every shape a delimiter cell can take.
func TestIsSeparatorCell_MatchesRegexp(t *testing.T) {
	ref := regexp.MustCompile(`^:?-+:?$`)
	for _, c := range []string{
		"", ":", "::", "-", "--", ":-", "-:", ":-:", ":--:", "---", ":---:",
		"-:-", "- -", " -", "- ", ":-:-", "a", "-a", "::-", "-::", ":-x", "–",
	} {
		assert.Equal(t, ref.MatchString(c), isSeparatorCell([]byte(c)), "bytes %q", c)
		assert.Equal(t, ref.MatchString(c), isSeparatorCell(c), "string %q", c)
	}
}
