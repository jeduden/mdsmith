//go:build !wasm

package requiredstructure

import "os"

// sameStatFile reports whether docPath and schemaPath resolve to the
// same on-disk file, using os.Stat + os.SameFile (inode identity).
// The bool ok is false when either stat fails, signalling the caller
// to fall back to a path comparison. Built only on native: tinygo's
// wasm runtime omits os.SameFile, and a WASM host has no OS disk. See
// docs/background/concepts/engine-api.md.
func sameStatFile(docPath, schemaPath string) (same, ok bool) {
	docInfo, errDoc := os.Stat(docPath)
	schemaInfo, errSchema := os.Stat(schemaPath)
	if errDoc != nil || errSchema != nil {
		return false, false
	}
	return os.SameFile(docInfo, schemaInfo), true
}
