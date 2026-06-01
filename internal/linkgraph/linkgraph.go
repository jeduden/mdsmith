// Package linkgraph extracts Markdown links and heading anchors so the
// link-validity rule (MDS027) and the `backlinks` subcommand share one
// implementation of the link walk, anchor slug rules, and target
// parsing.
package linkgraph

import (
	"net/url"
	"strings"

	"github.com/yuin/goldmark/ast"

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
type Link struct {
	Line   int
	Column int
	Text   string
	Target Target
}

// ExtractLinks walks f.AST and returns every regular Markdown link in
// document order. Lines are body-relative (post front-matter strip);
// see the Link doc for why.
func ExtractLinks(f *lint.File) []Link {
	if f == nil || f.AST == nil {
		return nil
	}
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
		target, ok := ParseTarget(string(l.Destination))
		if !ok {
			return ast.WalkContinue, nil
		}
		line, col := linkPosition(f, l)
		out = append(out, Link{
			Line:   line,
			Column: col,
			Text:   linkText(l, f.Source),
			Target: target,
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
		target, ok := ParseTarget(string(img.Destination))
		if !ok {
			return ast.WalkContinue, nil
		}
		line, col := linkPosition(f, img)
		out = append(out, Link{
			Line:   line,
			Column: col,
			Text:   mdtext.ExtractPlainText(img, f.Source),
			Target: target,
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
	// f.ColumnOfOffset scans backward from offset to the previous
	// newline, so it's O(column) per call instead of the O(offset)
	// forward scan a hand-rolled version would do — meaningful for
	// `mdsmith list backlinks` which can call this many times per file.
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
