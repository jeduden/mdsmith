package listscan

import "testing"

func TestOpeningFenceRel_BacktickInInfoString(t *testing.T) {
	// A backtick fence whose info string contains a backtick is not valid.
	line := []byte("```go`extra")
	if _, ok := openingFenceRel(line, 0, 0); ok {
		t.Error("expected false for backtick fence with backtick in info string")
	}
}

func TestOpeningFenceRel_CleanInfoString(t *testing.T) {
	line := []byte("```go")
	if _, ok := openingFenceRel(line, 0, 0); !ok {
		t.Error("expected true for valid backtick fence")
	}
}

func TestOpeningFenceRel_TildeAllowsBacktickInInfo(t *testing.T) {
	// Tilde fences do not restrict backticks in info strings.
	line := []byte("~~~go`extra")
	if _, ok := openingFenceRel(line, 0, 0); !ok {
		t.Error("expected true for tilde fence with backtick in info string")
	}
}
