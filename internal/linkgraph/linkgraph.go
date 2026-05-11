// Package linkgraph extracts Markdown links and heading anchors so the
// link-validity rule (MDS027) and the `backlinks` subcommand share one
// implementation of the link walk, anchor slug rules, and target
// parsing.
package linkgraph

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
)

// Target is the parsed shape of a link destination URL.
//
// Raw is the original destination string. Path and Anchor are URL-
// decoded. LocalAnchor is true when the destination was an anchor-only
// reference (e.g. `#section`).
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

	path := u.Path
	if path == "" && u.Opaque != "" {
		path = u.Opaque
	}

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
// CLI callers (like `mdsmith backlinks`) that want file-relative line
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

// CollectAnchors returns the set of heading anchors defined in f, with
// GitHub-compatible disambiguation suffixes (-1, -2, …) when the same
// slug repeats. The set keys are the slugified anchor names; values
// are always true so callers can use map-lookup.
func CollectAnchors(f *lint.File) map[string]bool {
	anchors := make(map[string]bool)
	if f == nil || f.AST == nil {
		return anchors
	}
	seen := make(map[string]int)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		slug := mdtext.Slugify(headingText(h, f.Source))
		if slug == "" {
			return ast.WalkContinue, nil
		}

		count := seen[slug]
		anchor := slug
		if count > 0 {
			anchor = fmt.Sprintf("%s-%d", slug, count)
		}
		seen[slug] = count + 1
		anchors[anchor] = true

		return ast.WalkContinue, nil
	})

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

// headingText concatenates the heading's text-bearing leaves into a
// plain string. Code spans and other inline nodes pass through their
// text children. This mirrors the behavior MDS027 used before the
// link-graph refactor; LSP/index uses a richer ExtractPlainText but
// the two agree on heading content because headings contain only
// text-like children in practice.
func headingText(node ast.Node, source []byte) string {
	var b strings.Builder
	appendHeadingText(&b, node, source)
	return b.String()
}

func appendHeadingText(b *strings.Builder, node ast.Node, source []byte) {
	switch n := node.(type) {
	case *ast.Text:
		b.Write(n.Segment.Value(source))
		return
	case *ast.String:
		b.Write(n.Value)
		return
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		appendHeadingText(b, child, source)
	}
}

// linkText returns the visible link text (everything between `[` and
// `]`). Image alt text and emphasis are flattened to plain text so
// JSON/text output stays readable.
func linkText(link *ast.Link, source []byte) string {
	return mdtext.ExtractPlainText(link, source)
}

// linkPosition returns the 1-based source line and column of a link
// node. The returned line does NOT include f.LineOffset; ExtractLinks
// applies the offset before returning.
func linkPosition(f *lint.File, n ast.Node) (int, int) {
	offset := firstTextOffset(n)
	if offset < 0 {
		return 1, 1
	}
	line := f.LineOfOffset(offset)
	lineStart := 0
	for i := 0; i < offset && i < len(f.Source); i++ {
		if f.Source[i] == '\n' {
			lineStart = i + 1
		}
	}
	return line, offset - lineStart + 1
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
