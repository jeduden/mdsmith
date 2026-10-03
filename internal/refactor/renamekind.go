package refactor

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jeduden/mdsmith/internal/mdtext"
)

// RenameKind names the symbol a rename targets. Its values are the
// strings the hosts' explicit selector (the CLI's `--as`,
// Session.Rename's `as`) accepts; the empty kind asks Rename to
// auto-detect.
type RenameKind string

// The rename kinds every host dispatches on.
const (
	KindHeading RenameKind = "heading"
	KindLabel   RenameKind = "label"
)

// renameKinds is the one list of explicit kinds every host accepts;
// ParseRenameKind, InvalidRenameKindError, and the hosts' selector
// messages all derive from it.
var renameKinds = []RenameKind{KindHeading, KindLabel}

// kindName holds the nouns a kind's messages use: symbol in the
// auto-detect messages ("a heading and a link-ref label"), missing in
// MissingSymbolError ("no link reference").
type kindName struct{ symbol, missing string }

// kindNouns names every kind in renameKinds; a new kind adds its row
// here and every host message picks it up.
var kindNouns = map[RenameKind]kindName{
	KindHeading: {symbol: "heading", missing: "heading"},
	KindLabel:   {symbol: "link-ref label", missing: "link reference"},
}

// symbolNoun is k's noun in the auto-detect messages; an unlisted kind
// names itself.
func (k RenameKind) symbolNoun() string {
	if n, ok := kindNouns[k]; ok {
		return n.symbol
	}
	return string(k)
}

// missingNoun is k's noun in MissingSymbolError; an unlisted kind
// names itself.
func (k RenameKind) missingNoun() string {
	if n, ok := kindNouns[k]; ok {
		return n.missing
	}
	return string(k)
}

// RenameSymbolList renders every kind's symbol noun joined by conj
// ("and", "or"), each prefixed by "a" when article is set, so a host's
// ambiguity and no-target messages name every kind auto-detect tries:
// RenameSymbolList("and", true) is "a heading and a link-ref label".
func RenameSymbolList(conj string, article bool) string {
	parts := make([]string, len(renameKinds))
	for i, k := range renameKinds {
		parts[i] = k.symbolNoun()
		if article {
			parts[i] = "a " + parts[i]
		}
	}
	return joinList(parts, conj)
}

// RenameKindList renders renameKinds as an English "a or b" list,
// formatting each kind with verb ("%s" bare, "%q" quoted), so a host's
// invalid-selector message names every valid kind without
// hard-coding them.
func RenameKindList(verb string) string {
	parts := make([]string, len(renameKinds))
	for i, k := range renameKinds {
		parts[i] = fmt.Sprintf(verb, string(k))
	}
	return joinOr(parts)
}

// joinOr joins parts as an English disjunction: "a", "a or b",
// "a, b, or c".
func joinOr(parts []string) string { return joinList(parts, "or") }

// joinList joins parts as an English list with conj: "a", "a conj b",
// "a, b, conj c".
func joinList(parts []string, conj string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " " + conj + " " + parts[1]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", " + conj + " " + parts[len(parts)-1]
}

// Sentinel outcomes of Rename that are not engine conflicts. Hosts
// map them to their own message text and exit code or error value.
var (
	// ErrAmbiguousRename: auto-detect found both a heading and a
	// link-ref label named oldName.
	ErrAmbiguousRename = errors.New("name matches both " + RenameSymbolList("and", true))
	// ErrNoRenameTarget: auto-detect found neither.
	ErrNoRenameTarget = errors.New("no " + RenameSymbolList("or", false) + " matches the name")
	// ErrNothingToRename: the symbol exists but renaming it changes
	// no byte. Rename returns it as a NothingToRenameError, which
	// matches this sentinel under errors.Is.
	ErrNothingToRename = errors.New("nothing to rename")
)

// InvalidRenameKindError reports an explicit kind that is neither
// KindHeading nor KindLabel.
type InvalidRenameKindError struct{ Kind string }

func (e InvalidRenameKindError) Error() string {
	return fmt.Sprintf("rename kind must be %s, got %q", RenameKindList("%q"), e.Kind)
}

// NothingToRenameError reports that renaming the Kind symbol Name
// leaves the file byte-identical: a heading renamed to its own text,
// or a label renamed to the spelling every occurrence already has.
// errors.Is matches it against ErrNothingToRename.
type NothingToRenameError struct {
	Kind RenameKind
	Name string
}

func (e NothingToRenameError) Error() string {
	return fmt.Sprintf("nothing to rename for %s %q", e.Kind, e.Name)
}

// Is makes errors.Is(err, ErrNothingToRename) hold.
func (e NothingToRenameError) Is(target error) bool {
	return target == ErrNothingToRename
}

// MissingSymbolError reports that an explicitly requested heading or
// link-ref label named Name does not exist in the file.
type MissingSymbolError struct {
	Kind RenameKind
	Name string
}

func (e MissingSymbolError) Error() string {
	return fmt.Sprintf("no %s %q", e.Kind.missingNoun(), e.Name)
}

// ParseRenameKind validates a host's explicit kind selector. The
// empty string is valid and means auto-detect.
func ParseRenameKind(s string) (RenameKind, error) {
	k := RenameKind(s)
	if k == "" || slices.Contains(renameKinds, k) {
		return k, nil
	}
	return "", InvalidRenameKindError{Kind: s}
}

// Rename renames the heading or link-ref label oldName in the file
// fileKey (whose bytes are source) to newName and returns the plan
// that rewrites every dependent reference across ws. kind selects the
// symbol, or "" to auto-detect it from source. It is the one rename
// dispatch the CLI and pkg/mdsmith share; besides the engine's own
// errors (HeadingCollisionError, InvalidLabelRuneError, …) it returns
// ErrAmbiguousRename, ErrNoRenameTarget, MissingSymbolError,
// NothingToRenameError (matching ErrNothingToRename), or
// InvalidRenameKindError.
//
// fileKey must be the file's workspace-relative path: a heading rename
// uses it both as the key the file's own edits group under and as the
// path ws looks up incoming anchor edges for. A host whose edit keys
// differ from workspace paths (an LSP document URI) calls Heading
// directly, which takes the two separately.
func Rename(ws Workspace, fileKey string, source []byte, kind RenameKind, oldName, newName string) (Plan, error) {
	line := 0
	switch kind {
	case "":
		k, l, err := detectRenameKind(source, oldName)
		if err != nil {
			return Plan{}, err
		}
		kind, line = k, l
	case KindHeading:
		l, ok := findHeadingLine(source, oldName)
		if !ok {
			return Plan{}, MissingSymbolError{Kind: KindHeading, Name: oldName}
		}
		line = l
	case KindLabel:
		// Checked before LinkRef validates newName, as the heading
		// branch does, so a missing label reports as missing rather
		// than as a collision with (or a bad spelling of) newName.
		if !hasLinkRef(source, oldName) {
			return Plan{}, MissingSymbolError{Kind: KindLabel, Name: oldName}
		}
	default:
		return Plan{}, InvalidRenameKindError{Kind: string(kind)}
	}
	if kind == KindHeading {
		return renameHeadingAt(ws, fileKey, source, line, oldName, newName)
	}
	return renameLabel(fileKey, source, oldName, newName)
}

// renameHeadingAt runs the heading rename for the heading on the
// 1-based source line, turning a no-op plan into a
// NothingToRenameError: Heading's empty plan for a same-text rename,
// and a plan whose only edits rewrite fileKey to the bytes it already
// has (`# *Setup*` renamed Setup → *Setup*). Unchanged heading bytes
// shift no slug, so such a plan touches no other file.
func renameHeadingAt(ws Workspace, fileKey string, source []byte, line int, oldName, newName string) (Plan, error) {
	p, err := Heading(ws, fileKey, fileKey, source, line, oldName, newName)
	if err != nil {
		return Plan{}, err
	}
	if len(p.Edits) == 0 || onlyUnchangedSelf(p, fileKey, source) {
		return Plan{}, NothingToRenameError{Kind: KindHeading, Name: oldName}
	}
	return p, nil
}

// onlyUnchangedSelf reports whether p edits fileKey alone and those
// edits leave source byte-identical. A plan that edits some other file
// is a real rename even when fileKey's own bytes do not change.
func onlyUnchangedSelf(p Plan, fileKey string, source []byte) bool {
	own, ok := p.Edits[fileKey]
	return ok && len(p.Edits) == 1 && editsLeaveUnchanged(source, own)
}

// renameLabel runs the link-ref rename for a label Rename has already
// found defined in source, turning a plan whose edits leave source
// byte-identical into a NothingToRenameError, so a same-name label
// rename reports like a same-name heading rename.
func renameLabel(fileKey string, source []byte, oldName, newName string) (Plan, error) {
	p, err := LinkRef(fileKey, source, oldName, newName)
	if err != nil {
		return Plan{}, err
	}
	if editsLeaveUnchanged(source, p.Edits[fileKey]) {
		return Plan{}, NothingToRenameError{Kind: KindLabel, Name: oldName}
	}
	return p, nil
}

// editsLeaveUnchanged reports whether every edit replaces its range of
// source with the bytes already there, so the plan rewrites nothing.
// It compares each edit in place rather than splicing a copy of the
// file. A plan ApplyEdits would reject (an edit off the file, across
// lines, outside its row, or overlapping another) is not a no-op: the
// host's own apply step reports it.
func editsLeaveUnchanged(source []byte, edits []Edit) bool {
	es := slices.Clone(edits)
	slices.SortStableFunc(es, func(a, b Edit) int {
		if c := cmp.Compare(a.Range.Start.Line, b.Range.Start.Line); c != 0 {
			return c
		}
		return cmp.Compare(a.Range.Start.Character, b.Range.Start.Character)
	})
	line, lineStart := 0, 0
	for i, e := range es {
		if e.Range.Start.Line < 0 || e.Range.End.Line != e.Range.Start.Line {
			return false
		}
		if i > 0 && es[i-1].Range.Start.Line == e.Range.Start.Line &&
			e.Range.Start.Character < es[i-1].Range.End.Character {
			return false
		}
		for line < e.Range.Start.Line {
			nl := bytes.IndexByte(source[lineStart:], '\n')
			if nl < 0 {
				return false
			}
			lineStart += nl + 1
			line++
		}
		if !editKeepsRow(rowAt(source, lineStart), e) {
			return false
		}
	}
	return true
}

// rowAt returns the line of source starting at byte start, without its
// `\n` or trailing `\r`.
func rowAt(source []byte, start int) []byte {
	row := source[start:]
	if nl := bytes.IndexByte(row, '\n'); nl >= 0 {
		row = row[:nl]
	}
	return bytes.TrimSuffix(row, []byte{'\r'})
}

// editKeepsRow reports whether e addresses a valid range of row and
// its NewText equals the bytes in that range.
func editKeepsRow(row []byte, e Edit) bool {
	if checkEditRange(e, mdtext.UTF16FromByteOffset(row, len(row)), 0) != nil {
		return false
	}
	cur := utf16Cursor{row: row}
	start, end, err := cur.editBytes(e, 0)
	return err == nil && string(row[start:end]) == e.NewText
}

// detectRenameKind decides whether oldName names a heading or a
// link-ref label in source. For a heading it also returns the 1-based
// line findHeadingLine found, so Rename does not search for the
// heading a second time. Both matching is ErrAmbiguousRename; neither
// is ErrNoRenameTarget.
func detectRenameKind(source []byte, oldName string) (RenameKind, int, error) {
	line, isHeading := findHeadingLine(source, oldName)
	isLabel := hasLinkRef(source, oldName)
	switch {
	case isHeading && isLabel:
		return "", 0, ErrAmbiguousRename
	case isHeading:
		return KindHeading, line, nil
	case isLabel:
		return KindLabel, 0, nil
	}
	return "", 0, ErrNoRenameTarget
}
