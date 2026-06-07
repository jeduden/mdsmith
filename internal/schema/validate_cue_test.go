//go:build !wasm

package schema

import (
	"testing"

	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/token"
	"github.com/stretchr/testify/assert"
)

// TestSchemaDiagFromCUEError_EmptyPathNoActual exercises the
// !hasActual branch inside the "extra field" arm of
// schemaDiagFromCUEError. An errors.Error whose Path() returns nil
// has no corresponding front-matter value (lookupFM returns false),
// so the diagnostic fills the Actual slot with the "<extra field>"
// sentinel rather than the document value.
func TestSchemaDiagFromCUEError_EmptyPathNoActual(t *testing.T) {
	sch := &Schema{Frontmatter: map[string]string{"id": "int"}}
	docFM := map[string]any{"id": 42}
	// errors.Newf with NoPos produces an Error whose Path() is nil,
	// so lookupFM returns (nil, false) → !hasActual is true.
	ce := errors.Newf(token.NoPos, "synthetic root-level CUE error")
	d := schemaDiagFromCUEError(sch, docFM, ce)
	assert.Equal(t, "<extra field>", d.Actual)
	assert.Equal(t, "not declared in schema", d.Expected)
}
