//go:build !wasm

package recipesafety

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
)

// TestMDS040RegisteredOnNative asserts the recipe-safety rule
// self-registers on native builds. Its registration lives in a
// //go:build !wasm file (register.go); under WASM the rule is omitted
// because it presumes a real shell. This test, also tagged !wasm,
// pins the native contract.
func TestMDS040RegisteredOnNative(t *testing.T) {
	found := false
	for _, r := range rule.All() {
		if r.ID() == "MDS040" {
			found = true
			break
		}
	}
	assert.True(t, found, "MDS040 must be registered on native builds")
}
