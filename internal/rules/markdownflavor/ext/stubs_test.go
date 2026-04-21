package ext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExtensionStubsAreSafe exercises the no-op interface methods
// that goldmark requires (Close / CloseBlock / CanInterruptParagraph /
// CanAcceptIndentedLine / Dump). None do meaningful work; the tests
// just ensure they don't panic when invoked.
func TestExtensionStubsAreSafe(t *testing.T) {
	assert.NotPanics(t, func() {
		(&mathBlockParser{}).Close(nil, nil, nil)
		_ = (&mathBlockParser{}).CanInterruptParagraph()
		_ = (&mathBlockParser{}).CanAcceptIndentedLine()
	}, "mathBlockParser stubs must not panic")

	assert.NotPanics(t, func() {
		(&abbreviationBlockParser{}).Close(nil, nil, nil)
		_ = (&abbreviationBlockParser{}).CanInterruptParagraph()
		_ = (&abbreviationBlockParser{}).CanAcceptIndentedLine()
	}, "abbreviationBlockParser stubs must not panic")

	assert.NotPanics(t, func() {
		(&superscriptParser{}).CloseBlock(nil, nil)
		(&subscriptParser{}).CloseBlock(nil, nil)
		(&mathInlineParser{}).CloseBlock(nil, nil)
	}, "inline parser CloseBlock stubs must not panic")
}

// TestDumpDoesNotPanic calls Dump on each custom AST node type with a
// nil source buffer. Dump is a debug helper; we only care that it
// runs without crashing.
func TestDumpDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() { (&SuperscriptNode{}).Dump(nil, 0) })
	assert.NotPanics(t, func() { (&SubscriptNode{}).Dump(nil, 0) })
	assert.NotPanics(t, func() { (&MathBlockNode{}).Dump(nil, 0) })
	assert.NotPanics(t, func() { (&MathInlineNode{}).Dump(nil, 0) })
	assert.NotPanics(t, func() { (&AbbreviationDefinition{}).Dump(nil, 0) })
	assert.NotPanics(t, func() { (&AbbreviationReference{}).Dump(nil, 0) })
}
