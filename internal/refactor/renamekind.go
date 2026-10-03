package refactor

import (
	"errors"
	"fmt"
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

// Sentinel outcomes of Rename that are not engine conflicts. Hosts
// map them to their own message text and exit code or error value.
var (
	// ErrAmbiguousRename: auto-detect found both a heading and a
	// link-ref label named oldName.
	ErrAmbiguousRename = errors.New("name matches both a heading and a link-ref label")
	// ErrNoRenameTarget: auto-detect found neither.
	ErrNoRenameTarget = errors.New("no heading or link-ref label matches the name")
	// ErrNothingToRename: the heading exists but the new name equals
	// the old one, so the plan has no edits.
	ErrNothingToRename = errors.New("nothing to rename")
)

// InvalidRenameKindError reports an explicit kind that is neither
// KindHeading nor KindLabel.
type InvalidRenameKindError struct{ Kind string }

func (e InvalidRenameKindError) Error() string {
	return fmt.Sprintf("rename kind must be %q or %q, got %q", KindHeading, KindLabel, e.Kind)
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
	switch k := RenameKind(s); k {
	case "", KindHeading, KindLabel:
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
// ErrNothingToRename, or InvalidRenameKindError.
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
		l, ok := FindHeadingLine(source, oldName)
		if !ok {
			return Plan{}, MissingSymbolError{Kind: KindHeading, Name: oldName}
		}
		line = l
	case KindLabel:
	default:
		return Plan{}, InvalidRenameKindError{Kind: string(kind)}
	}
	if kind == KindHeading {
		return renameHeadingAt(ws, fileKey, source, line, oldName, newName)
	}
	return renameLabel(fileKey, source, oldName, newName)
}

func renameHeadingAt(ws Workspace, fileKey string, source []byte, line int, oldName, newName string) (Plan, error) {
	p, err := Heading(ws, fileKey, fileKey, source, line, oldName, newName)
	if err != nil {
		return Plan{}, err
	}
	if len(p.Edits) == 0 {
		return Plan{}, ErrNothingToRename
	}
	return p, nil
}

func renameLabel(fileKey string, source []byte, oldName, newName string) (Plan, error) {
	p, err := LinkRef(fileKey, source, oldName, newName)
	if err != nil {
		return Plan{}, err
	}
	if len(p.Edits[fileKey]) == 0 {
		return Plan{}, MissingSymbolError{Kind: KindLabel, Name: oldName}
	}
	return p, nil
}

// detectRenameKind decides whether oldName names a heading or a
// link-ref label in source. For a heading it also returns the 1-based
// line FindHeadingLine found, so Rename does not parse the file again.
// Both matching is ErrAmbiguousRename; neither is ErrNoRenameTarget.
func detectRenameKind(source []byte, oldName string) (RenameKind, int, error) {
	line, isHeading := FindHeadingLine(source, oldName)
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
