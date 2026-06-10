package markdownlint

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/yamlutil"
)

// commentWidth caps the rendered width of note comment lines in the
// emitted config.
const commentWidth = 72

// EmitConfig renders a Conversion as .mdsmith.yml bytes: a header
// comment naming the source file, the notes as a "Not converted" list,
// then only the converted rule entries — unmentioned rules keep their
// mdsmith defaults when the file is loaded.
func EmitConfig(conv *Conversion, source string) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Converted from %s by mdsmith init --from-markdownlint.\n", source)
	b.WriteString("# Rules not listed here keep their mdsmith defaults.\n")
	if len(conv.Notes) > 0 {
		b.WriteString("#\n# Not converted:\n")
		for _, note := range conv.Notes {
			for _, line := range wrapComment(note, commentWidth) {
				b.WriteString(line)
				b.WriteByte('\n')
			}
		}
	}
	b.WriteByte('\n')

	body := struct {
		FrontMatter bool                      `yaml:"front-matter"`
		Rules       map[string]config.RuleCfg `yaml:"rules"`
	}{FrontMatter: true, Rules: conv.Rules}

	data, err := yamlutil.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshalling converted config: %w", err)
	}
	b.Write(data)
	return []byte(b.String()), nil
}

// wrapComment renders one note as `# - …` comment lines wrapped at
// width, with `#   ` continuation lines. A word longer than the width
// stays on its own overlong line.
func wrapComment(text string, width int) []string {
	const first, cont = "# - ", "#   "
	var lines []string
	line := first
	for _, word := range strings.Fields(text) {
		sep := ""
		if line != first && line != cont {
			sep = " "
		}
		if len(line)+len(sep)+len(word) > width && sep != "" {
			lines = append(lines, line)
			line = cont
			sep = ""
		}
		line += sep + word
	}
	lines = append(lines, line)
	return lines
}
