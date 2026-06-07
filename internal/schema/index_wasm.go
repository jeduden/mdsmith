//go:build wasm

package schema

import "github.com/jeduden/mdsmith/internal/lint"

// This file holds the WASM stubs for the schema-index side-output
// read and write paths. The `<?index?>` feature writes and reads a
// JSON index file next to the source document on the OS filesystem,
// which a GOOS=js GOARCH=wasm host (running against an in-memory
// MemWorkspace, with no OS disk) cannot do. The tinygo runtime also
// omits os.Chmod and friends. So both entry points are no-ops under
// WASM. See docs/background/concepts/engine-api.md.

// WriteIndex is a no-op under WASM: there is no OS filesystem to
// write the index side-output to. MDS020's Fix swallows this return
// value, so a schema that declares an `index:` simply produces no
// sidecar file in a WASM host.
func WriteIndex(_ *lint.File, _ *Schema) error { return nil }

// ValidateIndex returns no diagnostics under WASM: with no OS disk to
// read the index side-output from, the staleness check is skipped.
// MDS020's other checks (heading structure, filename, content) still
// run.
func ValidateIndex(_ *lint.File, _ *Schema, _ MakeDiag) []lint.Diagnostic { return nil }
