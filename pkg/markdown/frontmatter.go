package markdown

import "bytes"

// StripFrontMatter removes YAML front matter delimited by "---\n"
// from the beginning of source. It returns the front matter block
// (including delimiters) and the remaining content. If no front
// matter is found, prefix is nil and content equals source.
//
// The closing fence is recognised only at the start of a line:
// either as the first bytes after the opening fence (empty FM
// body) or preceded by a newline. A naive bytes.Index search
// would otherwise truncate the front matter early when a YAML
// block-scalar value legitimately contains the literal `---\n`
// sequence (e.g. a `notes: |` value whose body includes an
// em-dash row).
func StripFrontMatter(source []byte) (prefix, content []byte) {
	delim := []byte("---\n")
	if !bytes.HasPrefix(source, delim) {
		return nil, source
	}
	rest := source[len(delim):]
	// Empty front matter: closing fence sits right after the
	// opening fence, no body lines between them.
	if bytes.HasPrefix(rest, delim) {
		end := len(delim) + len(delim)
		return source[:end], source[end:]
	}
	// Non-empty body: the closing fence is preceded by the
	// last body line's "\n". Search for "\n---\n" so a
	// fence-shaped substring inside a block scalar (e.g.
	// "  ---\n") cannot be mistaken for the actual fence.
	needle := []byte("\n---\n")
	idx := bytes.Index(rest, needle)
	if idx < 0 {
		return nil, source
	}
	end := len(delim) + idx + len(needle)
	return source[:end], source[end:]
}

// CountLines returns the number of newline-terminated lines in b.
func CountLines(b []byte) int {
	return bytes.Count(b, []byte("\n"))
}
