package pack

import (
	"fmt"
	"path/filepath"

	"github.com/jeduden/mdsmith/internal/convention"
	"github.com/jeduden/mdsmith/internal/wordlist"
)

func init() {
	register(Pack{
		Name:    "wordlists",
		Summary: "Curated no-llm-tells word-lists (ai-speak, ai-openers)",
		files:   wordlistFiles,
	})
}

// wordlistFiles renders one `.mdsmith/wordlists/<name>.yaml` per built-in
// no-llm-tells list. Each body carries a header comment recording its
// origin and the exact `lists:` reference that wires it into the matching
// rule, then the curated entries. The files are the user's to edit;
// nothing reads them until a rule names them.
func wordlistFiles() []File {
	lists := convention.NoLLMTellsWordlists()
	out := make([]File, 0, len(lists))
	for _, wl := range lists {
		header := fmt.Sprintf(
			"# .mdsmith/wordlists/%[1]s.yaml\n"+
				"#\n"+
				"# Scaffolded by `mdsmith init --add wordlists` from the built-in\n"+
				"# no-llm-tells vocabulary. This file is yours: add or remove\n"+
				"# entries freely. Reference it from a rule's lists: key, e.g.\n"+
				"#\n"+
				"#   rules:\n"+
				"#     %[2]s:\n"+
				"#       lists: [%[1]s]\n",
			wl.Name, wl.Rule)
		// RenderFile only errors on empty entries; the curated lists are
		// non-empty (guaranteed by the drift test), so discard the error.
		data, _ := wordlist.RenderFile(header, wl.Entries)
		out = append(out, File{
			Path: filepath.Join(".mdsmith", "wordlists", wl.Name+".yaml"),
			Data: data,
		})
	}
	return out
}
