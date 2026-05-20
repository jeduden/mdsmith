package tableformat

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- Category coverage ---

func TestCategory_TableFormat(t *testing.T) {
	r := &Rule{Pad: 1}
	assert.Equal(t, "table", r.Category())
}

// --- Fix: tables == 0 branch ---

func TestFix_NoTables_ReturnsUnchangedBytes(t *testing.T) {
	// When there are no tables, Fix returns bytes equal to the source.
	src := "# Just a heading\n\nSome text, no tables here.\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	result := r.Fix(f)
	assert.Equal(t, src, string(result))
}

// --- Fix: already-formatted branch ---

func TestFix_AlreadyFormattedTable_ReturnsUnchangedBytes(t *testing.T) {
	// Table is already properly formatted — Fix returns bytes equal to the source.
	src := "| a   | b      |\n| --- | ------ |\n| foo | barbaz |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	result := r.Fix(f)
	assert.Equal(t, src, string(result))
}
