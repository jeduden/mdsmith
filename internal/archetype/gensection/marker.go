package gensection

import "bytes"

// IsRawStartMarker reports whether line begins a processing-instruction
// start marker for the given directive name (e.g. "<?catalog ...").
// It operates on raw bytes for use before AST parsing (e.g. merge driver).
func IsRawStartMarker(line []byte, name string) bool {
	return bytes.HasPrefix(bytes.TrimSpace(line), []byte("<?"+name))
}

// IsRawEndMarker reports whether line is a processing-instruction
// end marker for the given directive name (e.g. "<?/catalog?>").
// It operates on raw bytes for use before AST parsing (e.g. merge driver).
func IsRawEndMarker(line []byte, name string) bool {
	return bytes.Equal(bytes.TrimSpace(line), []byte("<?/"+name+"?>"))
}
