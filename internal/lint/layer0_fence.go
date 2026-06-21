package lint

import "bytes"

// fenceInfo describes an opening fenced-code fence line.
type fenceInfo struct {
	char   byte
	indent int
	length int
	// hasInfo records whether the opening fence carries a non-empty info
	// string after the fence run. goldmark exposes no source position for
	// an info-less, content-less fence, so the projection emits no lines
	// for it — hasInfo drives that quirk.
	hasInfo bool
}

// openingFence parses line as a fenced-code opening fence, returning its
// data and ok=true when it qualifies: indent < 4, a run of >= 3 identical
// fence characters, and (for backtick fences) no backtick in the info
// string. Mirrors fencedCodeBlockParser.Open.
func openingFence(line []byte) (fenceInfo, bool) {
	indent := leadingSpaces(line)
	if indent >= 4 {
		return fenceInfo{}, false
	}
	if indent >= len(line) {
		return fenceInfo{}, false
	}
	ch := line[indent]
	if ch != '`' && ch != '~' {
		return fenceInfo{}, false
	}
	j := indent
	for j < len(line) && line[j] == ch {
		j++
	}
	length := j - indent
	if length < 3 {
		return fenceInfo{}, false
	}
	rest := line[j:]
	if ch == '`' && bytes.IndexByte(rest, '`') >= 0 {
		return fenceInfo{}, false
	}
	return fenceInfo{
		char:    ch,
		indent:  indent,
		length:  length,
		hasInfo: len(bytes.TrimSpace(rest)) > 0,
	}, true
}

// closingFence reports whether line closes a fence opened with fi: indent
// < 4, a run of >= fi.length identical fence characters, and only
// whitespace after the run. Mirrors fencedCodeBlockParser.Continue.
func closingFence(line []byte, fi fenceInfo) bool {
	indent := leadingSpaces(line)
	if indent >= 4 {
		return false
	}
	j := indent
	for j < len(line) && line[j] == fi.char {
		j++
	}
	if j-indent < fi.length {
		return false
	}
	return isBlankLine(line[j:])
}

// advanceFenceState advances the open-fence tracking for a block quote's
// stripped body line, using the fence-open result the caller already
// computed (opensFence / fi) so openingFence is not re-run. When no fence
// is open, a fence opener starts one; when a fence is open, a matching
// closing fence ends it. Used so the quote scan knows a fenced code block
// is still open and therefore cannot be lazily continued by a non-marker
// line.
func advanceFenceState(open *fenceInfo, line []byte, fi fenceInfo, opensFence bool) *fenceInfo {
	if open == nil {
		if opensFence {
			f := fi
			return &f
		}
		return nil
	}
	if closingFence(line, *open) {
		return nil
	}
	return open
}

// tryFence recognises a fenced code block at the cursor. It marks every
// line from the opening fence through the closing fence (or end of
// document for an unclosed fence) as code, records the span, and advances
// the cursor past it. Returns false when the cursor line is not a fence.
func (s *scanner) tryFence() bool {
	fi, ok := openingFence(s.lines[s.i])
	if !ok {
		return false
	}
	openLine := s.i // 0-based opening fence index
	// Scan content lines until a closing fence or EOF. The closing fence
	// is never a content line (goldmark closes before appending it).
	lastContent := 0 // 1-based; 0 means "no content lines"
	closed := false
	s.i++
	for s.i < len(s.lines) {
		if s.trailingEmptyLine(s.i) {
			break
		}
		if closingFence(s.lines[s.i], fi) {
			closed = true
			break
		}
		lastContent = s.i + 1
		s.i++
	}
	// goldmark exposes no source position for an info-less, content-less
	// fence, so addFencedCodeBlockLines emits nothing for it. Mirror that:
	// skip marking entirely when the fence has neither info nor content.
	if fi.hasInfo || lastContent > 0 {
		s.markCode(openLine)
		for ln := openLine + 2; ln <= lastContent; ln++ {
			s.markCode(ln - 1)
		}
		// Mirror addFencedCodeBlockLines: the closing fence is the line
		// after the last content line (or after the opening fence when
		// there were no content lines). For a closed fence that is the
		// matched line; for an unclosed fence it is a phantom line, marked
		// only when within bounds.
		closeLine := lastContent + 1
		if lastContent == 0 {
			closeLine = openLine + 2 // 0-based open +1 to 1-based, +1 next
		}
		if closeLine <= len(s.lines) {
			s.markCode(closeLine - 1)
		}
	}
	if closed {
		s.i++ // advance past the matched closing fence line
	}
	s.addSpan(BlockFencedCode, openLine, s.i-1, 0)
	// Record fence closure for MDS031: closed is local to this scan, so
	// stamp it on the span tryFence just appended.
	s.l0.BlockSpans[len(s.l0.BlockSpans)-1].Closed = closed
	s.prevNonBlankParagraph = false
	return true
}
