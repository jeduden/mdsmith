//go:build wasm

package fix

import "os"

// chmodFile is a no-op under WASM. tinygo's wasm runtime omits
// os.Chmod, and a WASM host fixes documents in memory through
// fix.Source / fix.SourceWithRules rather than the on-disk
// atomicWriteFile path that calls this, so the file mode is never
// applied there. See docs/background/concepts/engine-api.md.
func chmodFile(_ string, _ os.FileMode) error { return nil }
