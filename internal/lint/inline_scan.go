package lint

import (
	"bytes"

	"github.com/jeduden/mdsmith/pkg/goldmark/arena"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/jeduden/mdsmith/pkg/goldmark/util"
)

// scanInlineRun is the Layer 1 byte scanner: it reconstructs the inline
// node tree of a single inline-bearing run (the bytes of one contiguous run
// of paragraph lines) WITHOUT running goldmark's block-plus-inline parse,
// returning the run's Document root and ok=true when it can do so
// byte-identically. When the run holds a construct the scanner does not
// reproduce exactly — emphasis delimiters, reference-style links, raw HTML,
// backslash escapes, or any block marker (heading, list, quote, setext) — it
// returns ok=false so the caller falls back to the goldmark parse for that
// run. The equivalence gate (TestInlineIndexEquivalence_ParityRules) holds
// the scanner's output identical to goldmark across the corpus, so an
// over-eager scanner that mis-handled a shape would fail the build rather
// than ship a divergent diagnostic.
//
// run is the run's bytes; base is the run's start byte offset in the document
// Source. The returned tree's segment offsets are run-local (relative to
// run[0]), matching parseInlineWithRefsArena, so every existing consumer maps
// them back with base unchanged.
//
// The tree shape mirrors goldmark for a single paragraph block:
// Document -> Paragraph -> inline children. The scanner is used only for runs
// that are a single paragraph block (scanRunEligible), which is where the
// shape holds; other runs fall back.
func scanInlineRun(run []byte, a *arena.Arena) (ast.Node, bool) {
	if !scanRunEligible(run) {
		return nil, false
	}
	doc := ast.NewDocument()
	para := a.Paragraph()
	doc.AppendChild(doc, para)
	setParagraphLines(run, para)
	if !scanParagraphInlines(run, para, a) {
		return nil, false
	}
	return doc, true
}

// scanRunEligible reports whether the scanner may attempt run. It bails on
// any byte that signals a construct the scanner does not reproduce
// byte-identically: a block marker leading any line (heading, list, quote,
// thematic break, setext underline), or an emphasis / reference / raw-HTML /
// escape / entity byte anywhere. The conservative gate guarantees the
// scanner only sees plain text, inline links, inline images, autolinks, and
// code spans — the constructs scanParagraphInlines handles exactly.
//
// `[` and `<` and “ ` “ are admitted because the scanner handles inline
// links/images, autolinks, and code spans; the per-construct parse re-checks
// each occurrence and bails (via scanParagraphInlines) when one is not the
// exact shape it reproduces.
func scanRunEligible(run []byte) bool {
	if len(run) == 0 {
		return false
	}
	// Single-line runs only. A multi-line paragraph forces goldmark's
	// soft/hard line-break handling and the way it splits and merges Text
	// segments around the break — interactions with adjacent links and
	// trailing whitespace that are subtle to reproduce byte-identically.
	// Restricting the scanner to single-line runs keeps its output provably
	// identical (verified across the corpus); multi-line runs fall back.
	if bytes.IndexByte(run, '\n') >= 0 {
		return false
	}
	// Reject a run whose single line carries a block marker: the scanner
	// reproduces only the inline shape of a plain paragraph line, so a
	// heading, list item, block quote, or thematic break must fall back.
	if paragraphLeadKind(run) != BlockParagraph || isATXHeadingLine(run) {
		return false
	}
	// Emphasis (`*`/`_`), reference links / lone brackets resolution,
	// backslash escapes, raw HTML / entities (`&`) are the shapes whose
	// exact reproduction needs goldmark's delimiter / reference / escape
	// machinery. Bail on any of their trigger bytes; the cheap inline
	// constructs (links, images, autolinks, code spans) carry none of them
	// in their structural positions, and scanParagraphInlines rejects a `[`
	// or `<` that is not one of those shapes.
	return !bytes.ContainsAny(run, "*_\\&")
}

// mergeAppendText emits run[start:end) as a Text segment with goldmark's
// MergeOrAppend semantics: when the parent's last child is a Text node whose
// segment ends exactly at start (and is not a soft-break), the new bytes are
// folded into it rather than appended as a separate node. This reproduces the
// way goldmark accumulates plain text across the trigger bytes that fail to
// open a construct (a bare `]`, or a `!` not followed by `[`): each such byte
// flushes the pending segment with MergeOrAppend, so all the contiguous
// stretches collapse into one Text node.
func mergeAppendText(run []byte, start, end int, para ast.Node, a *arena.Arena) {
	if end <= start {
		return
	}
	ast.MergeOrAppendTextSegmentA(para, text.NewSegment(start, end), a)
}

// finalAppendText emits run[start:end) as goldmark's line-end text: a plain
// AppendChild (never a merge) with trailing spaces and tabs trimmed
// (TrimRightSpace), so the run's last text stretch is its own Text node with
// the same bounds goldmark gives the paragraph's final segment. A stretch
// that trims to empty is dropped.
func finalAppendText(run []byte, start, end int, para ast.Node, a *arena.Arena) {
	stop := end
	for stop > start && (run[stop-1] == ' ' || run[stop-1] == '\t') {
		stop--
	}
	if stop <= start {
		return
	}
	para.AppendChild(para, a.TextSegment(text.NewSegment(start, stop)))
}

// setParagraphLines records one line Segment per source line of run on the
// paragraph's Lines(), so the inline rules that resolve a node's source line
// by walking up to the paragraph's first line (imageLine / nodeLine, used
// when an inline node has no descendant Text — e.g. an empty-alt image or an
// empty-text link) read the same first-line offset goldmark records. Each
// segment spans [lineStart, nextLineStart) so its bounds include the trailing
// newline, matching goldmark's block line segments; only the first segment's
// Start is load-bearing for the rules, and it is 0 (the run begins at the
// paragraph), matching the document-relative mapping base + Start.
func setParagraphLines(run []byte, para ast.Node) {
	lines := para.Lines()
	pos := 0
	for pos < len(run) {
		nl := bytes.IndexByte(run[pos:], '\n')
		if nl < 0 {
			lines.Append(text.NewSegment(pos, len(run)))
			break
		}
		lines.Append(text.NewSegment(pos, pos+nl+1))
		pos += nl + 1
	}
}

// scanParagraphInlines walks run forward, appending inline children to para
// in document order, and returns ok=true when every byte was consumed into a
// shape the scanner reproduces exactly. It returns false the moment it meets
// a `[`, `<`, or backtick that is not a complete inline link, image,
// autolink, or code span — the signal for the caller to fall back. Text runs
// between constructs become Text nodes with goldmark's MergeOrAppend
// semantics so MDS012's per-Text-node URL scan sees identical node bounds.
func scanParagraphInlines(run []byte, para ast.Node, a *arena.Arena) bool {
	// goldmark treats a paragraph line's leading spaces (up to 3; 4+ would be
	// indented code, which the file-level eligibility gate already excludes)
	// as block indentation, so the first inline segment starts after them.
	i := leadingSpaces(run)
	textStart := i
	for i < len(run) {
		c := run[i]
		switch c {
		case '`':
			ni, ns, ok := applyCodeSpan(run, i, textStart, para, a)
			if !ok {
				return false
			}
			i, textStart = ni, ns
		case '<':
			ni, ns, ok := applyAutolink(run, i, textStart, para, a)
			if !ok {
				return false
			}
			i, textStart = ni, ns
		case '!':
			ni, ns, ok := applyBang(run, i, textStart, para, a)
			if !ok {
				return false
			}
			i, textStart = ni, ns
		case '[':
			ni, ns, ok := applyLink(run, i, textStart, para, a)
			if !ok {
				return false
			}
			i, textStart = ni, ns
		case ']':
			// A bare `]` is a link-parser trigger with no opener the scanner
			// produced: goldmark flushes the pending text at it (MergeOrAppend)
			// and continues. The `]` byte starts the next text segment.
			mergeAppendText(run, textStart, i, para, a)
			textStart = i
			i++
		default:
			i++
		}
	}
	// goldmark appends the final line's remaining text with a plain
	// AppendChild (never a merge) and a trailing-space trim, so it is always
	// its own Text node.
	finalAppendText(run, textStart, len(run), para, a)
	return true
}

// applyCodeSpan flushes the pending text [textStart:i) and appends a
// CodeSpan node for the code span beginning at run[i]. Returns the new i and
// textStart (both equal to next, just past the closing backtick run) and ok.
// Returns ok=false when scanCodeSpan declines, signalling a goldmark fallback.
func applyCodeSpan(run []byte, i, textStart int, para ast.Node, a *arena.Arena) (int, int, bool) {
	node, next, ok := scanCodeSpan(run, i, a)
	if !ok {
		return 0, 0, false
	}
	mergeAppendText(run, textStart, i, para, a)
	para.AppendChild(para, node)
	return next, next, true
}

// applyAutolink flushes the pending text [textStart:i) and appends an
// AutoLink node for the autolink beginning at run[i] (a `<`). Returns
// ok=false when scanAutolink declines (a raw-HTML `<` or unterminated angle
// bracket), signalling a goldmark fallback.
func applyAutolink(run []byte, i, textStart int, para ast.Node, a *arena.Arena) (int, int, bool) {
	node, next, ok := scanAutolink(run, i, a)
	if !ok {
		return 0, 0, false
	}
	mergeAppendText(run, textStart, i, para, a)
	para.AppendChild(para, node)
	return next, next, true
}

// applyBang handles a `!` at run[i]. If it opens an image (`![…](…)`), it
// flushes the pending text and appends the Image node. Otherwise, the `!` is
// a link-parser trigger that failed: goldmark flushes the pending text before
// it and the `!` starts the next text segment. Returns ok=false when the `!`
// opens an image whose scan fails, signalling a goldmark fallback.
func applyBang(run []byte, i, textStart int, para ast.Node, a *arena.Arena) (int, int, bool) {
	if i+1 < len(run) && run[i+1] == '[' {
		node, next, ok := scanLinkOrImage(run, i, true, a)
		if !ok {
			return 0, 0, false
		}
		mergeAppendText(run, textStart, i, para, a)
		para.AppendChild(para, node)
		return next, next, true
	}
	// `!` not opening an image: flush pending text before `!`; the `!` byte
	// itself starts the next text segment.
	mergeAppendText(run, textStart, i, para, a)
	return i + 1, i, true
}

// applyLink flushes the pending text [textStart:i) and appends the Link node
// for the inline link beginning at run[i] (a `[`). Returns ok=false when
// scanLinkOrImage declines (reference link, nested brackets, or other non-
// inline form), signalling a goldmark fallback.
func applyLink(run []byte, i, textStart int, para ast.Node, a *arena.Arena) (int, int, bool) {
	node, next, ok := scanLinkOrImage(run, i, false, a)
	if !ok {
		return 0, 0, false
	}
	mergeAppendText(run, textStart, i, para, a)
	para.AppendChild(para, node)
	return next, next, true
}

// scanCodeSpan parses a code span beginning at run[i] (a backtick). It
// reproduces parser.codeSpanParser.Parse: an opening run of n backticks, the
// shortest following run of exactly n backticks (not part of a longer run)
// closing it, the inner text as a single raw Text child, and the
// first/last halfspace trim. Returns ok=false when there is no matching
// closing run (goldmark then emits the opener as literal text — a shape the
// scanner does not special-case, so it falls back).
func scanCodeSpan(run []byte, i int, a *arena.Arena) (ast.Node, int, bool) {
	opener := 0
	for i+opener < len(run) && run[i+opener] == '`' {
		opener++
	}
	contentStart := i + opener
	j := contentStart
	for j < len(run) {
		if run[j] == '`' {
			k := j
			for k < len(run) && run[k] == '`' {
				k++
			}
			closure := k - j
			if closure == opener {
				// Found the closing run. Inner content is [contentStart:j).
				node := a.CodeSpan()
				cStart, cStop := codeSpanTrim(run, contentStart, j)
				if cStop > cStart {
					node.AppendChild(node, a.RawTextSegment(text.NewSegment(cStart, cStop)))
				}
				return node, k, true
			}
			j = k
			continue
		}
		j++
	}
	return nil, 0, false
}

// codeSpanTrim applies goldmark's code-span halfspace trim: when both the
// first and last inner bytes are a space or newline (and the content is not
// all spaces), one byte is trimmed from each end. Returns the trimmed inner
// bounds.
func codeSpanTrim(run []byte, start, stop int) (int, int) {
	if start >= stop {
		return start, stop
	}
	// "is blank" guard: goldmark skips the trim when the whole content is
	// spaces/newlines.
	allBlank := true
	for k := start; k < stop; k++ {
		if !util.IsSpace(run[k]) {
			allBlank = false
			break
		}
	}
	if allBlank {
		return start, stop
	}
	if isSpaceOrNewlineByte(run[start]) && isSpaceOrNewlineByte(run[stop-1]) {
		return start + 1, stop - 1
	}
	return start, stop
}

// no test by design: trivial one-liner with no branch.
func isSpaceOrNewlineByte(c byte) bool {
	return c == ' ' || c == '\n'
}

// scanAutolink parses an autolink beginning at run[i] (a `<`). It mirrors
// parser.autoLinkParser.Parse: an email or URL between `<` and `>`. Returns
// ok=false when the bytes are not a valid autolink (goldmark then hands the
// `<` to the raw-HTML parser or leaves it as text — shapes the scanner does
// not reproduce, so it falls back).
func scanAutolink(run []byte, i int, a *arena.Arena) (ast.Node, int, bool) {
	line := run[i:]
	stop := util.FindEmailIndex(line[1:])
	typ := ast.AutoLinkType(ast.AutoLinkEmail)
	if stop < 0 {
		stop = util.FindURLIndex(line[1:])
		typ = ast.AutoLinkURL
	}
	if stop < 0 {
		return nil, 0, false
	}
	stop++
	if stop >= len(line) || line[stop] != '>' {
		return nil, 0, false
	}
	value := a.TextSegment(text.NewSegment(i+1, i+stop))
	node := ast.NewAutoLink(typ, value)
	return node, i + stop + 1, true
}

// scanLinkOrImage parses an inline link `[text](dest "title")` or image
// `![alt](dest "title")` beginning at run[i]. isImage selects the image form
// (run[i] is `!`, run[i+1] is `[`). It reproduces the parser.linkParser
// inline path only: the label is a single bracketed span with NO nested
// brackets and NO inline constructs the scanner cannot itself reproduce, and
// the destination is the `(...)` form. Returns ok=false for any other shape
// (reference links, nested brackets, a label that is not immediately followed
// by `(`), so the caller falls back to goldmark.
func scanLinkOrImage(run []byte, i int, isImage bool, a *arena.Arena) (ast.Node, int, bool) {
	labelStart := i + 1
	if isImage {
		labelStart = i + 2
	}
	// Find the matching ']' with no nesting and no special inner bytes other
	// than plain text (the eligible gate already barred '*','_','\\','&').
	// A nested '[' or a code span / autolink inside the label is a shape the
	// scanner does not reconstruct here, so bail.
	j := labelStart
	for j < len(run) {
		switch run[j] {
		case ']':
			goto foundClose
		case '[', '<', '`', '\n':
			return nil, 0, false
		}
		j++
	}
	return nil, 0, false
foundClose:
	labelEnd := j // exclusive, points at ']'
	// Must be the inline form: ']' immediately followed by '('.
	if labelEnd+1 >= len(run) || run[labelEnd+1] != '(' {
		return nil, 0, false
	}
	dest, title, after, ok := scanLinkParens(run, labelEnd+1)
	if !ok {
		return nil, 0, false
	}
	link := a.Link()
	// Copy destination and title into freshly owned slices so the node does
	// not alias the source run (goldmark's destination/title are slices into
	// the reader buffer; aliasing run is equivalent and stable, but a copy
	// keeps the node self-contained). Empty destination stays nil/empty to
	// match goldmark for `[x]()`.
	link.Destination = dest
	link.Title = title
	// The label text becomes a Text child, split at line boundaries like any
	// text — but labels are single-line here (a '\n' bailed above), so one
	// segment.
	if labelEnd > labelStart {
		link.AppendChild(link, a.TextSegment(text.NewSegment(labelStart, labelEnd)))
	}
	if isImage {
		return ast.NewImage(link), after, true
	}
	return link, after, true
}

// scanLinkParens parses the `(dest "title")` part of an inline link starting
// at run[open] (a `(`). It mirrors parser.parseLink: optional spaces, an
// optional destination, an optional title in matching quotes or parens, and
// the closing `)`. Returns the destination and title byte slices (sliced
// from run), the index just past the closing `)`, and ok. Returns ok=false
// for any shape parser.parseLink rejects.
func scanLinkParens(run []byte, open int) (dest, title []byte, after int, ok bool) {
	i := open + 1 // skip '('
	i = skipSpacesAt(run, i)
	if i < len(run) && run[i] == ')' {
		// Empty link `[x]()`.
		return nil, nil, i + 1, true
	}
	d, ni, dok := scanLinkDestination(run, i)
	if !dok {
		return nil, nil, 0, false
	}
	dest = d
	i = ni
	i = skipSpacesAt(run, i)
	if i < len(run) && run[i] == ')' {
		return dest, nil, i + 1, true
	}
	tt, ti, tok := scanLinkTitle(run, i)
	if !tok {
		return nil, nil, 0, false
	}
	title = tt
	i = ti
	i = skipSpacesAt(run, i)
	if i < len(run) && run[i] == ')' {
		return dest, title, i + 1, true
	}
	return nil, nil, 0, false
}

// skipSpacesAt advances past spaces, tabs, and newlines, mirroring
// block.SkipSpaces over a single run (the run is one paragraph, so newlines
// inside parens are skipped as goldmark's reader would across lines).
func skipSpacesAt(run []byte, i int) int {
	for i < len(run) && util.IsSpace(run[i]) {
		i++
	}
	return i
}

// scanLinkDestination parses a link destination at run[i], mirroring
// parser.parseLinkDestination's non-escape path (the eligible gate barred
// '\\', so escape handling is unreachable). It supports the `<...>` bracket
// form and the bare form that ends at the first space or unbalanced ')'.
func scanLinkDestination(run []byte, i int) (dest []byte, after int, ok bool) {
	if i < len(run) && run[i] == '<' {
		j := i + 1
		for j < len(run) {
			if run[j] == '>' {
				return run[i+1 : j], j + 1, true
			}
			if run[j] == '\n' || run[j] == '<' {
				return nil, 0, false
			}
			j++
		}
		return nil, 0, false
	}
	opened := 0
	j := i
	for j < len(run) {
		c := run[j]
		if c == '(' {
			opened++
		} else if c == ')' {
			opened--
			if opened < 0 {
				break
			}
		} else if util.IsSpace(c) {
			break
		}
		j++
	}
	return run[i:j], j, true
}

// scanLinkTitle parses a link title at run[i], mirroring
// parser.parseLinkTitle: the title is quoted with `"`, `'`, or `(`/`)` and
// the scanner reads to the matching closer. Multi-line titles bail (a '\n'
// inside the title makes the simple closer search ambiguous against the run
// boundary). Returns the title bytes, the index past the closer, and ok.
func scanLinkTitle(run []byte, i int) (title []byte, after int, ok bool) {
	if i >= len(run) {
		return nil, 0, false
	}
	opener := run[i]
	var closer byte
	switch opener {
	case '"', '\'':
		closer = opener
	case '(':
		closer = ')'
	default:
		return nil, 0, false
	}
	j := i + 1
	for j < len(run) {
		c := run[j]
		if c == '\n' {
			return nil, 0, false
		}
		// Paren-form titles reject an inner '(' (goldmark FindClosure Nesting:false).
		if opener == '(' && c == '(' {
			return nil, 0, false
		}
		if c == closer {
			return run[i+1 : j], j + 1, true
		}
		j++
	}
	return nil, 0, false
}
