package markdownlint

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/bytelimit"
)

// ConvertFile reads, parses, and converts the markdownlint config at
// path, returning the .mdsmith.yml bytes ready to write plus the
// conversion notes (also embedded in the emitted header comment). The
// read is capped at bytelimit.DefaultMaxInputBytes.
func ConvertFile(path string) (data []byte, notes []string, err error) {
	raw, err := bytelimit.ReadFileLimited(path, bytelimit.DefaultMaxInputBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	parsed, err := Parse(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	conv, err := Convert(parsed)
	if err != nil {
		return nil, nil, err
	}
	data, err = EmitConfig(conv, path)
	return data, conv.Notes, err
}
