package lint

import "github.com/jeduden/mdsmith/internal/mdfence"

// advanceFenceState advances the open-fence tracking for a block quote's
// stripped body line, using the fence-open result the caller already
// computed (opensFence / fi) so mdfence.Open is not re-run. When no fence
// is open, a fence opener starts one; when a fence is open, a matching
// closing fence ends it. Used so the quote scan knows a fenced code block
// is still open and therefore cannot be lazily continued by a non-marker
// line. The state is a Fence value whose zero (Char == 0) means no fence
// is open: tracking it by pointer would move every body line's Open
// result to the heap.
func advanceFenceState(open mdfence.Fence, line []byte, fi mdfence.Fence, opensFence bool) mdfence.Fence {
	if open.Char == 0 {
		if opensFence {
			return fi
		}
		return mdfence.Fence{}
	}
	if mdfence.Close(line, open) {
		return mdfence.Fence{}
	}
	return open
}

// tryFence recognises a fenced code block at the cursor. It marks every
// line from the opening fence through the closing fence (or end of
// document for an unclosed fence) as code, records the span, and advances
// the cursor past it. Returns false when the cursor line is not a fence.
func (s *scanner) tryFence() bool {
	fi, ok := mdfence.OpenFinal(s.lines[s.i], s.i+1 == s.final)
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
		if mdfence.Close(s.lines[s.i], fi) {
			closed = true
			break
		}
		lastContent = s.i + 1
		s.i++
	}
	// goldmark exposes no source position for an info-less, content-less
	// fence, so addFencedCodeBlockLines emits nothing for it. Mirror that:
	// skip marking entirely when the fence has neither info nor content.
	if fi.HasInfo || lastContent > 0 {
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
