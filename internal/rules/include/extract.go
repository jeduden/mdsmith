package include

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/lint"
)

// ExtractProjector returns the `mdsmith extract` JSON projection of
// targetFile, given the already-loaded file bytes. The host
// application (cmd/mdsmith, the LSP server, or release tooling)
// wires the real implementation, which loads .mdsmith.yml,
// resolves the kind, composes the schema, parses the target, and
// runs internal/extract.Extract on it. Tests provide a hermetic
// stub.
//
// readFS is the filesystem the include rule already read targetFile
// from; the projector can reuse it to load any sibling files the
// schema points at. host is the file the directive lives in; the
// projector uses host.StripFrontMatter / host.MaxInputBytes so the
// target parses with the same coordinate system the rest of the
// lint run uses.
//
// The function lives behind a package variable instead of a direct
// dependency on internal/config + internal/rules/requiredstructure
// so include/ stays in its layer of the dependency graph (cmd →
// engine → rules → lint / schema / extract). A rule package may
// not import a sibling rule package, and internal/config sits
// above rules; importing either back would form a compile cycle
// or break the rule-boundaries integration test.
type ExtractProjector func(
	host *lint.File, readFS fs.FS, targetFile string, data []byte,
) (any, error)

// projectExtract is the package-level projector the include rule
// consults at runtime. It is set by the host (see cmd/mdsmith) via
// SetExtractProjector. Nil means no projector is installed: an
// `extract:` directive then surfaces a clear diagnostic instead of
// silently returning nothing.
//
// The mutex guards concurrent SetExtractProjector calls (LSP config
// reloads, parallel tests via t.Cleanup) against in-flight reads
// from Rule.Check goroutines the engine fans out across files.
var (
	projectExtractMu sync.RWMutex
	projectExtract   ExtractProjector
)

// SetExtractProjector installs the package-level projector used by
// `<?include?>` directives carrying `extract:`. Calling with nil
// clears the projector (used by tests).
func SetExtractProjector(fn ExtractProjector) {
	projectExtractMu.Lock()
	defer projectExtractMu.Unlock()
	projectExtract = fn
}

// getExtractProjector returns the currently-installed projector
// under the read lock so the engine's parallel Check goroutines do
// not race with concurrent SetExtractProjector calls.
func getExtractProjector() ExtractProjector {
	projectExtractMu.RLock()
	defer projectExtractMu.RUnlock()
	return projectExtract
}

// extractContentKeys lists the well-known projection keys that mark
// an object as a "leaf" content carrier. When a path lookup ends at
// an object holding exactly one of these keys (and no siblings),
// walkExtractPath returns the inner value so callers do not have to
// know which projection rule produced the wrapper. The set mirrors
// the default key names contentBaseKey returns in internal/extract.
var extractContentKeys = []string{"text", "code", "inline", "items", "rows"}

// walkExtractPath walks a dotted path through the extract JSON tree
// rooted at data. Empty path segments and missing keys are errors;
// the final value is returned as-is unless it is a single-content-key
// wrapper object, in which case the inner content is returned.
//
// Examples (with data = {"tagline": {"text": "Hello"}}):
//
//	walkExtractPath(data, "tagline.text") -> "Hello"
//	walkExtractPath(data, "tagline")      -> "Hello" (single content key)
//
// Multi-key objects with no single content key are ambiguous and
// surface as an error so callers can require a more specific path.
func walkExtractPath(data map[string]any, dotted string) (any, error) {
	dotted = strings.TrimSpace(dotted)
	if dotted == "" {
		return nil, fmt.Errorf("path is empty")
	}
	parts := strings.Split(dotted, ".")
	var cur any = data
	for i, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("path has empty segment at position %d", i+1)
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"cannot descend into %q: parent is %T, not an object",
				p, cur)
		}
		next, found := obj[p]
		if !found {
			return nil, fmt.Errorf("missing key %q in extract projection", p)
		}
		cur = next
	}
	// If the final value is an object, try the single-content-key
	// fallback so callers do not have to spell out ".text" when the
	// only thing the section carries is its paragraph content.
	if obj, ok := cur.(map[string]any); ok {
		inner, err := pickContentKey(obj)
		if err != nil {
			return nil, err
		}
		return inner, nil
	}
	return cur, nil
}

// pickContentKey returns obj's content payload when obj is a single
// well-known content wrapper ({"text": ...} or {"code": ...} etc.),
// or surfaces an "ambiguous object" error otherwise. The error
// message lists the keys present so the user can pick one.
func pickContentKey(obj map[string]any) (any, error) {
	if len(obj) == 1 {
		for k, v := range obj {
			if isContentKey(k) {
				return v, nil
			}
			return nil, fmt.Errorf(
				"ambiguous object target: single key %q is not a "+
					"recognised content key; "+
					"append %q to the extract path", k, "."+k)
		}
	}
	// Multiple keys: see if exactly one is a content key.
	var hit string
	hits := 0
	for k := range obj {
		if isContentKey(k) {
			hits++
			hit = k
		}
	}
	if hits == 1 {
		return obj[hit], nil
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return nil, fmt.Errorf(
		"ambiguous object target with keys %v; "+
			"pick a leaf with a more specific extract path", keys)
}

// isContentKey reports whether key is one of the well-known content
// projection names from internal/extract's contentBaseKey.
func isContentKey(key string) bool {
	for _, k := range extractContentKeys {
		if k == key {
			return true
		}
	}
	return false
}

// projectExtractValue runs the installed projector on the target
// file, walks the dotted path through the resulting tree, and
// returns the stringified leaf ready for splicing into the include
// body. The caller passes the already-loaded target bytes (data)
// so the read is not duplicated. host is the file the directive
// lives in; resolvedFile is the path of the file the directive
// references, relative to the project root.
//
// A nil projector — or one that fails — surfaces a clear diagnostic
// so the caller can fix the host configuration or the directive.
func projectExtractValue(
	host *lint.File,
	readFS fs.FS, resolvedFile string,
	data []byte, dottedPath string,
) (string, error) {
	fn := getExtractProjector()
	if fn == nil {
		return "", fmt.Errorf(
			"extract: no extract projector is installed; " +
				"the include rule needs a host-installed projector " +
				"to project a typed value from the target file")
	}

	tree, err := fn(host, readFS, resolvedFile, data)
	if err != nil {
		return "", fmt.Errorf("extract: %w", err)
	}

	rootObj, ok := tree.(map[string]any)
	if !ok {
		return "", fmt.Errorf(
			"extract: projection of %q produced %T at root, "+
				"expected an object", resolvedFile, tree)
	}

	leaf, err := walkExtractPath(rootObj, dottedPath)
	if err != nil {
		return "", err
	}
	out, err := formatExtractValue(leaf)
	if err != nil {
		return "", fmt.Errorf("extract %q at %q: %w",
			resolvedFile, dottedPath, err)
	}
	return out, nil
}

// formatExtractValue renders an extract projection leaf as the text
// the include block body should contain. Strings, numbers, and bools
// stringify; list leaves (`items`) render as Markdown bullet lists,
// one entry per line, with list items themselves stringifying via the
// same rules. Map and table leaves (which would otherwise render as
// `map[k:v]` Go-syntax garbage via fmt.Sprint) are refused with an
// error so the caller can pick a more specific extract path.
func formatExtractValue(v any) (string, error) {
	switch val := v.(type) {
	case nil:
		return "", nil
	case string:
		return val, nil
	case bool, int, int64, float64:
		return fmt.Sprint(val), nil
	case []any:
		var b strings.Builder
		for i, item := range val {
			// List items themselves must be scalars; a nested []any
			// would render as `- - inner` (a bullet whose body
			// starts with `- ` rather than a Markdown nested list).
			// Maps would render as `- map[k:v]` Go-syntax garbage.
			// Refuse both shapes and ask the user to drill in.
			switch item.(type) {
			case []any:
				return "", fmt.Errorf(
					"list item %d is a nested list; only scalar "+
						"list items can be spliced into the include body",
					i)
			case map[string]any:
				return "", fmt.Errorf(
					"list item %d is an object; only scalar "+
						"list items can be spliced into the include body",
					i)
			}
			inner, err := formatExtractValue(item)
			if err != nil {
				return "", fmt.Errorf("list item %d: %w", i, err)
			}
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString("- ")
			b.WriteString(inner)
		}
		return b.String(), nil
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "", fmt.Errorf(
			"leaf is an object with keys %v; pick a leaf with a "+
				"more specific extract path", keys)
	default:
		return "", fmt.Errorf(
			"leaf has unsupported type %T; only strings, numbers, "+
				"bools, and lists can be spliced into the include body", val)
	}
}
