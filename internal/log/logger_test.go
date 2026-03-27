package log

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrintf_Enabled(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{Enabled: true, W: &buf}

	l.Printf("config: %s", ".mdsmith.yml")

	want := "config: .mdsmith.yml\n"
	assert.Equal(t, want, buf.String())
}

func TestPrintf_Disabled(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{Enabled: false, W: &buf}

	l.Printf("config: %s", ".mdsmith.yml")

	assert.Equal(t, "", buf.String(), "expected no output")
}

func TestPrintf_MultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{Enabled: true, W: &buf}

	l.Printf("file: %s", "README.md")
	l.Printf("rule: %s %s", "MDS001", "line-length")

	want := "file: README.md\nrule: MDS001 line-length\n"
	assert.Equal(t, want, buf.String())
}
