package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBatchGCTarget(t *testing.T) {
	// GOGC unset: apply the batch target so a short-lived check/fix run
	// stops collecting on every heap doubling.
	assert.Equal(t, batchGCPercent, batchGCTarget(""))

	// GOGC pinned by the user: leave the runtime default untouched.
	assert.Equal(t, -1, batchGCTarget("50"))
	assert.Equal(t, -1, batchGCTarget("off"))
	assert.Equal(t, -1, batchGCTarget("100"))
}
