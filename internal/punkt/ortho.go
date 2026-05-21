package punkt

// Orthographic context flags. Mirrors upstream
// (sentences/ortho.go); kept identical so any user who reads
// Storage.OrthoContext entries — the trained data — sees the same
// integer encoding.
const (
	orthoBegUc = 1 << 1
	orthoMidUc = 1 << 2
	orthoUnkUc = 1 << 3
	orthoBegLc = 1 << 4
	orthoMidLc = 1 << 5
	orthoUnkLc = 1 << 6
	orthoUc    = orthoBegUc + orthoMidUc + orthoUnkUc
	orthoLc    = orthoBegLc + orthoMidLc + orthoUnkLc
)

// OrthoContext determines the orthographic-evidence heuristic
// (section 4.1.1 of the Punkt paper). The reformulation keeps the
// same trio of returns as upstream: 1 (sentence starter), 0 (not a
// sentence starter), -1 (unknown).
//
// The struct embeds storage so heuristic lookups are direct field
// accesses (Storage.OrthoContext, SentStarters, etc.) instead of
// going through the embedded interface mash upstream uses. The
// caller passes a typeBuf that heuristic uses to compute
// TypeNoSentPeriod without allocating; reset to length 0 on entry.
type OrthoContext struct {
	Storage *Storage
}

// heuristic decides whether token starts a sentence.
//
//	 1 — token is capitalized AND its trained orthotype has the
//	     lowercase bit set AND the middle-uppercase bit is clear.
//	 0 — token is lowercase AND (orthotype has any uppercase bit OR
//	     never appears sentence-initial with lowercase).
//	-1 — unknown.
//
// typeBuf is a reusable byte slice used to compute TypeNoSentPeriod
// without allocating. Pass `state.typeBuf` (or any slice with cap
// large enough for the type result); the slice header on return
// is irrelevant — heuristic does not retain typeBuf.
//
// Upstream also loops over Punctuation() returning 0 for a bare
// punctuation token. We inline the same check: the punctuation set
// is the ASCII subset of upstream's `;:,.!?；：，。！？` (CJK
// dropped per doc.go).
func (o *OrthoContext) heuristic(token *Token, typeBuf []byte) int {
	if token == nil {
		return 0
	}
	if len(token.Tok) == 1 {
		switch token.Tok[0] {
		case ';', ':', ',', '.', '!', '?':
			return 0
		}
	}

	typeBuf = typeNoSentPeriod(token, typeBuf)
	// typeNoSentPeriod returns a []byte; the OrthoContext lookup is
	// keyed by string. The string conversion of a []byte triggers a
	// copy and allocation in general — but Go's compiler elides the
	// copy for `m[string(b)]` lookups specifically. See
	// https://github.com/golang/go/issues/3512 and the compiler's
	// `string(b)` map-key special case.
	orthoCtx := o.Storage.OrthoContext[string(typeBuf)]

	if firstUpper(token.Tok) && (orthoCtx&orthoLc > 0 && orthoCtx&orthoMidUc == 0) {
		return 1
	}
	if firstLower(token.Tok) && (orthoCtx&orthoUc > 0 || orthoCtx&orthoBegLc == 0) {
		return 0
	}
	return -1
}
