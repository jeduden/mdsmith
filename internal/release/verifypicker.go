package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

// pickerRow is one install-row parsed from the rendered homepage.
type pickerRow struct {
	cmdDefault  string
	cmdWindows  string
	hasNoscript bool
	noscriptCmd string
}

// VerifyInstallPicker probes the rendered homepage HTML produced by
// `hugo --minify ...` and asserts that the install picker
// (website/layouts/partials/install-picker.html) faithfully rendered
// channels. The partial emits one `install-row` per channel, in
// channels.yaml order, each carrying a `data-cmd-default` attribute
// equal to the channel `command`. For the channels that declare a
// Windows-specific `command-windows`, the row also carries a
// `data-cmd-windows` attribute (the filter JS swaps to it under the
// Windows chip) plus a <noscript> fallback whose `.cmd` span repeats
// the same command as text when JavaScript is off.
//
// None of this is visible to the channels.yaml round-trip test (it
// covers the Go <-> YAML layer, not the Hugo template), so without
// this probe a typo in the partial's `index . "command-windows"` key,
// its `{{ with $cw }}` guard, or an attribute name ships green while
// the user-visible Windows command silently disappears.
//
// The page is parsed as a DOM (scripting disabled, so the <noscript>
// fallback's elements are readable) rather than pattern-matched, so
// the probe is agnostic to attribute quoting and HTML escaping, both
// of which `--minify` normalizes. channels must be the same list the
// template rendered from (see LoadChannelsFromDataFile), so rows and
// channels line up by position. The picker renders once, on the
// homepage. Probes fail closed with a single error on the first
// mismatch.
func VerifyInstallPicker(htmlDir string, channels []Channel) error {
	indexPath := filepath.Join(htmlDir, "index.html")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("read rendered homepage: %w", err)
	}
	doc, err := html.ParseWithOptions(strings.NewReader(string(data)),
		html.ParseOptionEnableScripting(false))
	if err != nil {
		return fmt.Errorf("parse rendered homepage %s: %w", indexPath, err)
	}
	rows := parseInstallRows(doc)

	if len(rows) != len(channels) {
		return fmt.Errorf("install picker: found %d install-row elements in %s, want %d (one per channel)",
			len(rows), indexPath, len(channels))
	}
	for i, c := range channels {
		r := rows[i]
		if r.cmdDefault != c.Command {
			return fmt.Errorf("install picker: row %d (channel %q) has data-cmd-default %q, want %q",
				i, c.Title, r.cmdDefault, c.Command)
		}
		if c.CommandWindows == "" {
			// A channel with no override renders neither artifact.
			if r.cmdWindows != "" {
				return fmt.Errorf("install picker: row %d (channel %q) has a stray data-cmd-windows %q",
					i, c.Title, r.cmdWindows)
			}
			if r.hasNoscript {
				return fmt.Errorf("install picker: row %d (channel %q) has a stray <noscript> fallback",
					i, c.Title)
			}
			continue
		}
		if r.cmdWindows != c.CommandWindows {
			return fmt.Errorf("install picker: row %d (channel %q) has data-cmd-windows %q, want %q",
				i, c.Title, r.cmdWindows, c.CommandWindows)
		}
		if !r.hasNoscript || r.noscriptCmd != c.CommandWindows {
			return fmt.Errorf("install picker: row %d (channel %q) has no <noscript> Windows fallback for %q",
				i, c.Title, c.CommandWindows)
		}
	}
	return nil
}

// parseInstallRows returns the install-row elements of doc, in
// document order — which the partial renders in channels.yaml order.
func parseInstallRows(doc *html.Node) []pickerRow {
	var rows []pickerRow
	forEachElement(doc, func(n *html.Node) {
		if n.Data != "div" || !hasClass(n, "install-row") {
			return
		}
		r := pickerRow{
			cmdDefault: attrVal(n, "data-cmd-default"),
			cmdWindows: attrVal(n, "data-cmd-windows"),
		}
		if ns := findElement(n, func(m *html.Node) bool { return m.Data == "noscript" }); ns != nil {
			r.hasNoscript = true
			if span := findElement(ns, func(m *html.Node) bool {
				return m.Data == "span" && hasClass(m, "cmd")
			}); span != nil {
				r.noscriptCmd = strings.TrimSpace(textContent(span))
			}
		}
		rows = append(rows, r)
	})
	return rows
}

// forEachElement calls fn on every element node in n's subtree
// (inclusive), in document order.
func forEachElement(n *html.Node, fn func(*html.Node)) {
	if n.Type == html.ElementNode {
		fn(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		forEachElement(c, fn)
	}
}

// findElement returns the first element in n's subtree (inclusive)
// for which pred is true, in document order, or nil.
func findElement(n *html.Node, pred func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && pred(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if got := findElement(c, pred); got != nil {
			return got
		}
	}
	return nil
}

// attrVal returns the value of n's key attribute, or "" if absent.
// The html parser already decodes HTML entities in attribute values.
func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// hasClass reports whether n's class attribute contains want as a
// whitespace-separated token.
func hasClass(n *html.Node, want string) bool {
	for _, c := range strings.Fields(attrVal(n, "class")) {
		if c == want {
			return true
		}
	}
	return false
}

// textContent returns the concatenated text of n's subtree, with
// HTML entities already decoded by the parser.
func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(m *html.Node) {
		if m.Type == html.TextNode {
			b.WriteString(m.Data)
		}
		for c := m.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
