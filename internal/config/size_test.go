package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"2MB", 2 * 1024 * 1024},
		{"2mb", 2 * 1024 * 1024},
		{"2Mb", 2 * 1024 * 1024},
		{"500KB", 500 * 1024},
		{"500kb", 500 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"0", 0},
		{"1024", 1024},
		{"100", 100},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseSize(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSize_Invalid(t *testing.T) {
	tests := []struct {
		input   string
		wantErr string
	}{
		{"", "empty size string"},
		{"abc", "unrecognized unit"},
		{"-1", "negative size"},
		{"-5MB", "negative size"},
		{"2TB", "unrecognized unit"},
		{"2.5MB", "invalid size"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := ParseSize(tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
