package release

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// pickerDefaultRe and pickerWindowsRe extract the install picker's
// command attributes from rendered HTML. Channel commands always
// contain spaces (a URL, flags), so Hugo keeps the surrounding
// double quotes even under `--minify`; the captured value is then
// HTML-unescaped before comparison, so the probe is agnostic to
// whether the renderer left `<` as `&lt;` (default) or as `<`
// (minify normalizes it inside attribute values).
var (
	pickerDefaultRe = regexp.MustCompile(`data-cmd-default="([^"]*)"`)
	pickerWindowsRe = regexp.MustCompile(`data-cmd-windows="([^"]*)"`)
)

// VerifyInstallPicker probes the rendered homepage HTML produced
// by `hugo --minify ...` and asserts that the install picker
// (website/layouts/partials/install-picker.html) matches the
// channel source of truth in channels. The partial emits one
// `install-row` per channel, each carrying a `data-cmd-default`
// attribute equal to the channel `command`. For the channels that
// declare a Windows-specific `command-windows`, the row also
// carries a `data-cmd-windows` attribute (the filter JS swaps to
// it under the Windows chip) plus a <noscript> fallback (class
// `install-cmd-noscript`) that surfaces the same command as text
// when JavaScript is off.
//
// None of this is visible to the channels.yaml round-trip test
// (it covers the Go <-> YAML layer, not the Hugo template), so
// without this probe a typo in the partial's `index . "command-
// windows"` key, its `{{ with $cw }}` guard, or an attribute name
// ships green while the user-visible Windows command silently
// disappears.
//
// htmlDir is the Hugo output root (`public/` under website/). The
// picker renders once, on the homepage. Probes fail closed with a
// single returned error describing the first mismatch.
func VerifyInstallPicker(htmlDir string, channels []Channel) error {
	indexPath := filepath.Join(htmlDir, "index.html")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("read rendered homepage: %w", err)
	}
	page := string(data)

	// One row per channel, each with a data-cmd-default attribute.
	defaults := unescapedMatches(pickerDefaultRe, page)
	if len(defaults) != len(channels) {
		return fmt.Errorf("install picker: found %d data-cmd-default rows in %s, want %d (one per channel)",
			len(defaults), indexPath, len(channels))
	}
	wins := unescapedMatches(pickerWindowsRe, page)

	windows := 0
	for _, c := range channels {
		if !contains(defaults, c.Command) {
			return fmt.Errorf("install picker: no row with data-cmd-default %q (channel %q) in %s",
				c.Command, c.Title, indexPath)
		}
		if c.CommandWindows == "" {
			continue
		}
		windows++
		if !contains(wins, c.CommandWindows) {
			return fmt.Errorf("install picker: channel %q declares command-windows but no data-cmd-windows %q in %s",
				c.Title, c.CommandWindows, indexPath)
		}
		if !containsNoscriptCommand(page, c.CommandWindows) {
			return fmt.Errorf("install picker: channel %q has no <noscript> Windows fallback in %s",
				c.Title, indexPath)
		}
	}

	// No stray Windows artifacts beyond the channels that declare
	// an override — a channel without command-windows must render
	// neither the attribute nor the fallback.
	if len(wins) != windows {
		return fmt.Errorf("install picker: %d data-cmd-windows attributes in %s, want %d (channels with an override)",
			len(wins), indexPath, windows)
	}
	if got := strings.Count(page, "install-cmd-noscript"); got != windows {
		return fmt.Errorf("install picker: %d <noscript> fallbacks in %s, want %d (channels with an override)",
			got, indexPath, windows)
	}
	return nil
}

// unescapedMatches returns the HTML-unescaped first capture group
// of every match of re in page.
func unescapedMatches(re *regexp.Regexp, page string) []string {
	ms := re.FindAllStringSubmatch(page, -1)
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, html.UnescapeString(m[1]))
	}
	return out
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// containsNoscriptCommand reports whether cmd appears inside a
// <noscript> install-cmd line. It scopes the search to each
// <noscript>...</noscript> region and unescapes the block, so a
// match in the row's data-cmd-windows attribute does not count —
// the point is to prove the no-JS fallback text is present, not
// just the attribute.
func containsNoscriptCommand(page, cmd string) bool {
	rest := page
	for {
		open := strings.Index(rest, "<noscript")
		if open < 0 {
			return false
		}
		rest = rest[open:]
		end := strings.Index(rest, "</noscript>")
		if end < 0 {
			return false
		}
		block := rest[:end]
		if strings.Contains(block, "install-cmd-noscript") &&
			strings.Contains(html.UnescapeString(block), cmd) {
			return true
		}
		rest = rest[end+len("</noscript>"):]
	}
}
