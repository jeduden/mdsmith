package refactor

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
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
func joinOr(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " or " + parts[1]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", or " + parts[len(parts)-1]
}

// Sentinel outcomes of Rename that are not engine conflicts. Hosts
// map them to their own message text and exit code or error value.
var (
	// ErrAmbiguousRename: auto-detect found both a heading and a
	// link-ref label named oldName.
	ErrAmbiguousRename = errors.New("name matches both a heading and a link-ref label")
	// ErrNoRenameTarget: auto-detect found neither.
	ErrNoRenameTarget = errors.New("no heading or link-ref label matches the name")
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
	if e.Kind == KindHeading {
		return fmt.Sprintf("no heading %q", e.Name)
	}
	return fmt.Sprintf("no link reference %q", e.Name)
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

// editsLeaveUnchanged reports whether applying edits to source yields
// source byte for byte. A plan ApplyEdits rejects is not a no-op: the
// host's own apply step reports it.
func editsLeaveUnchanged(source []byte, edits []Edit) bool {
	out, err := ApplyEdits(source, edits)
	return err == nil && bytes.Equal(out, source)
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
