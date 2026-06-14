package yamlutil

import "testing"

// TestIsDecimalIntStrEdgeCases covers defensive branches that are unreachable
// via the public FlatScalarFrontMatter API (parsePlainFlatScalar guards
// against empty strings and bare "-" before calling isDecimalIntStr).
func TestIsDecimalIntStrEdgeCases(t *testing.T) {
	if isDecimalIntStr("") {
		t.Error("empty string should not be a decimal int")
	}
	if isDecimalIntStr("-") {
		t.Error("bare minus should not be a decimal int")
	}
}
