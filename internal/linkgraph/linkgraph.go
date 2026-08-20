// Package linkgraph extracts Markdown links and heading anchors so the
// link-validity rule (MDS027) and the `backlinks` subcommand share one
// implementation of the link walk, anchor slug rules, and target
// parsing.
package linkgraph

import (
	"bytes"
	"net/url"
	"strings"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
)

// Target is the parsed shape of a link destination URL.
//
// Raw is the original destination string as it appeared in the source.
// Path and Anchor are the decoded path and fragment components — both
// are populated from url.URL, which percent-decodes them on parse.
// LocalAnchor is true when the destination was an anchor-only
// reference (e.g. `#section`).
//
// Anchor matching against CollectAnchors output must still go through
// NormalizeAnchor: that runs Slugify (and a defensive PathUnescape) to
// produce the same form CollectAnchors stores.
type Target struct {
	Raw         string
	Path        string
	Anchor      string
	LocalAnchor bool
}

// isExternalDestination reports whether dest certainly has a URL
// scheme or is protocol-relative — the two shapes ParseTarget rejects
// via u.Scheme/u.Host. dest must already be space-trimmed.
//
// It mirrors net/url's getScheme: a scheme is an ASCII letter followed
// by letters, digits, '+', '-' or '.', terminated by ':', and only
// before the first '/', '?' or '#'. Answering on the raw bytes lets
// the common external link skip both the string copy and the url.URL
// the full parse allocates.
//
// Conservative by construction: it returns true only for destinations
// net/url would also call external, so a false answer simply falls
// through to the full parse.
func isExternalDestination(dest []byte) bool {
	if len(dest) >= 2 && dest[0] == '/' && dest[1] == '/' {
		return true
	}
	for i := 0; i < len(dest); i++ {
		c := dest[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			// Still inside a candidate scheme.
		case c >= '0' && c <= '9', c == '+', c == '-', c == '.':
			// Legal in a scheme, but not as its first character.
			if i == 0 {
				return false
			}
		case c == ':':
			// A scheme needs at least one leading letter.
			return i > 0
		default:
			// '/', '?', '#' or anything else ends the scheme search.
			return false
		}
	}
	return false
}

// needsURLParse reports whether dest contains anything that makes
// net/url do more than split on '#': a percent-escape to decode, a
// query string to strip, a control character it rejects outright, or
// a colon — which, once isExternalDestination has ruled out a real
// scheme, means net/url rejects the destination ("first path segment
// in URL cannot contain colon") rather than accepting it as a path.
// Without those, url.URL's Path and Fragment are plain substrings of
// the destination.
func needsURLParse(dest []byte) bool {
	for _, c := range dest {
		if c == '%' || c == '?' || c == ':' || c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}

// ParseTargetBytes parses a Markdown link destination held as bytes.
// It is the form the AST walks use, and returns exactly what
// ParseTarget returns for the same bytes — ParseTarget stays the
// reference implementation, and the equivalence is pinned by an
// exhaustive differential test.
//
// Two shapes avoid work the full parse would do. An external
// destination is rejected straight from the bytes, with no string
// copy at all. An ordinary local destination — no percent-escape, no
// query, no control character — is split on '#' into substrings of
// the one string this makes, instead of allocating a url.URL to
// recover the same two fields.
//
// Applies "Stay in []byte" and "the cheapest call is the one you
// never make" from docs/development/high-performance-go.md.
func ParseTargetBytes(dest []byte) (Target, bool) {
	trimmed := bytes.TrimSpace(dest)
	if len(trimmed) == 0 || isExternalDestination(trimmed) {
		return Target{}, false
	}
	if needsURLParse(trimmed) {
		// Pass the trimmed form: ParseTarget trims again, and Raw is
		// the trimmed value either way, so copying the surrounding
		// whitespace would be pure waste.
		return ParseTarget(string(trimmed))
	}

	raw := string(trimmed)
	path, anchor := raw, ""
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		path, anchor = raw[:i], raw[i+1:]
	}

	if path == "" {
		if anchor == "" {
			return Target{}, false
		}
		return Target{Raw: raw, Anchor: anchor, LocalAnchor: true}, true
	}
	return Target{Raw: raw, Path: path, Anchor: anchor}, true
}

// ParseTarget parses a Markdown link destination into a Target.
// Returns ok=false when the destination is empty, has a scheme or
// host (treated as external), or has neither a path nor a fragment.
func ParseTarget(dest string) (Target, bool) {
	dest = strings.TrimSpace(dest)
	if dest == "" || strings.HasPrefix(dest, "//") {
		return Target{}, false
	}

	u, err := url.Parse(dest)
	if err != nil {
		return Target{}, false
	}
	if u.Scheme != "" || u.Host != "" {
		return Target{}, false
	}

	// u.Opaque is non-empty only on URLs with a scheme; the scheme
	// check above already short-circuits that case, so we can read
	// the path component directly.
	path := u.Path

	if path == "" && u.Fragment != "" {
		return Target{
			Raw:         dest,
			Anchor:      u.Fragment,
			LocalAnchor: true,
		}, true
	}

	if path == "" {
		return Target{}, false
	}

	return Target{
		Raw:    dest,
		Path:   path,
		Anchor: u.Fragment,
	}, true
}

// Link is one parsed Markdown link occurrence in a source file.
//
// Reference-style links (`[text][label]`) are intentionally omitted
// from ExtractLinks results because their destinations resolve through
// the link-reference map rather than a URL; the link-graph builder
// only sees direct destinations.
//
// Line is body-relative — counted from the start of the parsed body,
// not the original file. Lint rules return body-relative diagnostics
// because the engine applies f.LineOffset for front-matter adjustment.
// CLI callers (like `mdsmith list backlinks`) that want file-relative line
// numbers must add f.LineOffset themselves.
// node is the AST node the link was extracted from, kept so Text can
// flatten the visible label on demand. It is nil on a Link built
// outside the walk (a zero value or a test literal), which Text
// reports as an empty label.
type Link struct {
	Line   int
	Column int
	Target Target
	node   ast.Node
}

// Text returns the visible link text (everything between `[` and `]`),
// with image alt text and emphasis flattened to plain text so
// JSON/text output stays readable.
//
// The label is built here rather than during the walk because no rule
// on the `check` path reads it — only `mdsmith list backlinks` does —
// and materialising it per link cost a bytes.Buffer plus a string copy
// for every link in every file. See
// docs/development/high-performance-go.md#skip-work-you-dont-need.
//
// source must be the Source of the File the Link was extracted from;
// the node's segments index into it.
func (l Link) Text(source []byte) string {
	if l.node == nil {
		return ""
	}
	return mdtext.ExtractPlainText(l.node, source)
}

// Links returns every regular Markdown link in document order, memoized
// on the per-Check File. Two calls on the same File return the same
// backing slice; callers must treat it as read-only. The result is
// byte-identical to ExtractLinks. nil is returned for a nil or AST-less
// File, matching ExtractLinks.
//
// Memoized via File.MemoFile (the *File-passing variant of Memo):
// buildLinks is a package-level function, not a closure, so the call
// adds no per-Memo-call heap allocation beyond the cold-path memoEntry.
func Links(f *lint.File) []Link {
	if f == nil {
		return nil
	}
	links, _ := f.MemoFile("linkgraph.links", buildLinks).([]Link)
	return links
}

// buildLinks is the MemoFile-style builder for the Links memo. Defined at
// package scope so the value passed to MemoFile is a plain function
// pointer (no closure capturing f), avoiding the per-call closure allocation.
func buildLinks(f *lint.File) any {
	return ExtractLinks(f)
}

// Images returns every Markdown image in document order, memoized on the
// per-Check File. Two calls on the same File return the same backing slice;
// callers must treat it as read-only. The result is byte-identical to
// ExtractImages. nil is returned for a nil or AST-less File.
//
// Memoized via File.MemoFile: buildImages is a package-level function so
// the call adds no per-Memo-call heap allocation beyond the cold-path
// memoEntry.
func Images(f *lint.File) []Link {
	if f == nil {
		return nil
	}
	links, _ := f.MemoFile("linkgraph.images", buildImages).([]Link)
	return links
}

// buildImages is the MemoFile-style builder for the Images memo.
func buildImages(f *lint.File) any {
	return ExtractImages(f)
}

// ExtractLinks walks f.AST and returns every regular Markdown link in
// document order. Lines are body-relative (post front-matter strip);
// see the Link doc for why.
func ExtractLinks(f *lint.File) []Link {
	if f == nil || f.AST == nil {
		return nil
	}
	// Deliberately not pre-sized. A byte-needle count of "](" is the
	// only cheap estimate available and it is systematically too high:
	// it also matches images, reference links and code samples, while
	// this walk keeps only local, non-reference links. That turns a
	// link-free file from zero allocations into one, and leaves a
	// document of external links holding a large empty array for the
	// life of the linkgraph.links memo. The memoized collectors in
	// astutil have tight bounds and are pre-sized; this one does not.
	var out []Link
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		l, ok := n.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}
		// Reference-style links carry l.Reference; the link-graph
		// builder skips them so callers see one shape per link.
		if l.Reference != nil {
			return ast.WalkContinue, nil
		}
		target, ok := ParseTargetBytes(l.Destination)
		if !ok {
			return ast.WalkContinue, nil
		}
		line, col := linkPosition(f, l)
		out = append(out, Link{
			Line:   line,
			Column: col,
			Target: target,
			node:   l,
		})
		return ast.WalkContinue, nil
	})
	return out
}

// ExtractImages walks f.AST and returns every Markdown image in
// document order. Both inline (Reference == nil) and reference-style
// (Reference != nil) images are included when their destination can
// be parsed as a local target. Lines are body-relative — same
// convention as Link.
func ExtractImages(f *lint.File) []Link {
	if f == nil || f.AST == nil {
		return nil
	}
	var out []Link
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		img, ok := n.(*ast.Image)
		if !ok {
			return ast.WalkContinue, nil
		}
		target, ok := ParseTargetBytes(img.Destination)
		if !ok {
			return ast.WalkContinue, nil
		}
		line, col := linkPosition(f, img)
		out = append(out, Link{
			Line:   line,
			Column: col,
			Target: target,
			node:   img,
		})
		return ast.WalkContinue, nil
	})
	return out
}

// CollectAnchors returns the set of heading anchors defined in f, with
// GitHub-compatible disambiguation suffixes (-1, -2, …) when slugs
// would otherwise collide. Uniqueness is enforced against the running
// set of produced anchors so a sequence like "Intro" / "Intro" /
// "Intro-1" yields three distinct keys (`intro`, `intro-1`,
// `intro-1-1`) rather than two distinct ones with a collision.
// The set keys are the slugified anchor names; values are struct{}
// so callers must use the comma-ok idiom: _, ok := anchors[key].
func CollectAnchors(f *lint.File) map[string]struct{} {
	anchors := make(map[string]struct{})
	if f == nil || f.AST == nil {
		return anchors
	}
	for _, item := range mdtext.CollectTOCItems(f.AST, f.Source) {
		anchors[item.Anchor] = struct{}{}
	}
	return anchors
}

// NormalizeAnchor URL-decodes raw and slugifies it so the result can
// be compared against CollectAnchors output.
func NormalizeAnchor(raw string) string {
	if decoded, err := url.PathUnescape(raw); err == nil {
		raw = decoded
	}
	return mdtext.Slugify(raw)
}

// linkText returns the visible link text (everything between `[` and
// `]`). Image alt text and emphasis are flattened to plain text so
// JSON/text output stays readable.
func linkText(link *ast.Link, source []byte) string {
	return mdtext.ExtractPlainText(link, source)
}

// linkPosition returns the 1-based source line and column of a link
// node, in body-relative coordinates (no f.LineOffset applied — see
// the Link doc for why).
func linkPosition(f *lint.File, n ast.Node) (int, int) {
	offset := firstTextOffset(n)
	if offset < 0 {
		return 1, 1
	}
	// f.ColumnOfOffset binary-searches the cached newline index, so
	// it's O(log lines) per call instead of the O(column) backward scan
	// a hand-rolled version would do — meaningful for `mdsmith list
	// backlinks` which can call this many times per file.
	return f.LineOfOffset(offset), f.ColumnOfOffset(offset)
}

func firstTextOffset(n ast.Node) int {
	offset := -1
	_ = ast.Walk(n, func(cur ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		text, ok := cur.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		if offset == -1 || text.Segment.Start < offset {
			offset = text.Segment.Start
		}
		return ast.WalkContinue, nil
	})
	return offset
}
