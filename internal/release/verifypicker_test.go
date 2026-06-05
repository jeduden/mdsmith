package release

import (
	"html"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// samplePickerChannels is the corpus the picker probe tests
// render and verify against: one toolchain channel with no
// Windows override (Go) and one binary-download channel that
// carries a command-windows plus its <noscript> fallback
// (GitHub Releases, whose default command uses the `<os>-<arch>`
// placeholder so its angle brackets exercise HTML escaping).
func samplePickerChannels() []Channel {
	return []Channel{
		{
			Title:     "Go",
			Command:   "go install github.com/jeduden/mdsmith@latest",
			Platforms: []string{"linux", "macos", "windows"},
		},
		{
			Title:          "GitHub Releases",
			Command:        "curl -LO https://example.test/mdsmith-<os>-<arch>",
			CommandWindows: "Invoke-WebRequest https://example.test/mdsmith-windows-amd64.exe -OutFile mdsmith.exe",
			Platforms:      []string{"linux", "macos", "windows"},
		},
	}
}

// pickerPage renders the install-picker fragment exactly as
// website/layouts/partials/install-picker.html emits it (one
// install-row per channel, data-cmd-default always, data-cmd-windows
// and a noscript install-cmd-noscript line only when the channel
// declares command-windows), with HTML escaping applied the way
// Hugo's html/template does. Each test starts here and mutates one
// thing so each failure isolates a single probe.
func pickerPage(channels []Channel) string {
	var b strings.Builder
	b.WriteString("<html><body><div class=\"install-picker\" data-install-picker>\n")
	for _, c := range channels {
		def := html.EscapeString(c.Command)
		b.WriteString(`<div class="install-row" data-platforms="` +
			html.EscapeString(strings.Join(c.Platforms, " ")) +
			`" data-cmd-default="` + def + `"`)
		if c.CommandWindows != "" {
			b.WriteString(` data-cmd-windows="` + html.EscapeString(c.CommandWindows) + `"`)
		}
		b.WriteString(">\n")
		b.WriteString(`  <code class="install-cmd"><span class="prompt">$</span> <span class="cmd">` +
			def + `</span></code>` + "\n")
		b.WriteString(`  <button class="install-copy" data-copy="` + def + `">copy</button>` + "\n")
		if c.CommandWindows != "" {
			b.WriteString(noscriptLine(c.CommandWindows))
		}
		b.WriteString("</div>\n")
	}
	b.WriteString("</div></body></html>\n")
	return b.String()
}

// noscriptLine is the exact rendered <noscript> fallback line for
// a Windows override; tests reuse it to delete or relocate it.
func noscriptLine(commandWindows string) string {
	win := html.EscapeString(commandWindows)
	return `  <noscript><code class="install-cmd install-cmd-noscript">Windows: ` +
		`<span class="prompt">$</span> <span class="cmd">` + win + `</span></code></noscript>` + "\n"
}

func writePickerSite(t *testing.T, page string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "index.html"), page)
	return root
}

func TestVerifyInstallPicker_Passes(t *testing.T) {
	chs := samplePickerChannels()
	root := writePickerSite(t, pickerPage(chs))
	require.NoError(t, VerifyInstallPicker(root, chs))
}

func TestVerifyInstallPicker_FailsOnMissingHomepage(t *testing.T) {
	err := VerifyInstallPicker(t.TempDir(), samplePickerChannels())
	require.Error(t, err)
}

func TestVerifyInstallPicker_FailsOnRowCountMismatch(t *testing.T) {
	chs := samplePickerChannels()
	// Render only the first channel, verify against both.
	root := writePickerSite(t, pickerPage(chs[:1]))
	err := VerifyInstallPicker(root, chs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install-row")
}

// A single-token command renders as an unquoted attribute under
// `hugo --minify` (`data-cmd-default=mdsmith`); the DOM parse must
// still read it, where a quoted-only matcher would miss the row.
func TestVerifyInstallPicker_AcceptsUnquotedSpacelessCommand(t *testing.T) {
	chs := []Channel{{Title: "Bare", Command: "mdsmith", Platforms: []string{"linux"}}}
	page := `<html><body><div class="install-picker">` +
		`<div class="install-row" data-platforms=linux data-cmd-default=mdsmith>` +
		`<code class="install-cmd"><span class="cmd">mdsmith</span></code>` +
		`</div></div></body></html>`
	root := writePickerSite(t, page)
	require.NoError(t, VerifyInstallPicker(root, chs))
}

// Each channel's <noscript> fallback is checked in its own row, so a
// match in a different channel's block does not satisfy it: channel A
// here has its command as a substring of channel B's command, and
// dropping A's own fallback must still fail.
func TestVerifyInstallPicker_NoscriptIsRowScoped(t *testing.T) {
	chs := []Channel{
		{Title: "A", Command: "a", CommandWindows: "x.exe", Platforms: []string{"windows"}},
		{Title: "B", Command: "b", CommandWindows: "run x.exe now", Platforms: []string{"windows"}},
	}
	page := strings.Replace(pickerPage(chs), noscriptLine("x.exe"), "", 1)
	root := writePickerSite(t, page)
	err := VerifyInstallPicker(root, chs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"A"`)
	assert.Contains(t, err.Error(), "noscript")
}

func TestVerifyInstallPicker_FailsOnWrongDefaultCommand(t *testing.T) {
	chs := samplePickerChannels()
	page := strings.ReplaceAll(pickerPage(chs),
		html.EscapeString(chs[0].Command), "go install WRONG")
	root := writePickerSite(t, page)
	err := VerifyInstallPicker(root, chs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data-cmd-default")
}

func TestVerifyInstallPicker_FailsOnMissingWindowsAttr(t *testing.T) {
	chs := samplePickerChannels()
	// Keep the channel declaring command-windows but drop the
	// rendered attribute (the `{{ with $cw }}` guard regressed).
	page := strings.Replace(pickerPage(chs),
		` data-cmd-windows="`+html.EscapeString(chs[1].CommandWindows)+`"`, "", 1)
	root := writePickerSite(t, page)
	err := VerifyInstallPicker(root, chs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data-cmd-windows")
}

func TestVerifyInstallPicker_FailsOnMissingNoscript(t *testing.T) {
	chs := samplePickerChannels()
	// Attribute present, but the no-JS fallback line is gone.
	page := strings.Replace(pickerPage(chs), noscriptLine(chs[1].CommandWindows), "", 1)
	root := writePickerSite(t, page)
	err := VerifyInstallPicker(root, chs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "noscript")
}

func TestVerifyInstallPicker_FailsOnStrayWindowsAttr(t *testing.T) {
	chs := samplePickerChannels()
	// A channel with no command-windows must not render the
	// attribute; inject a stray one onto the Go row.
	page := strings.Replace(pickerPage(chs),
		`data-cmd-default="`+html.EscapeString(chs[0].Command)+`"`,
		`data-cmd-default="`+html.EscapeString(chs[0].Command)+`" data-cmd-windows="rogue"`, 1)
	root := writePickerSite(t, page)
	err := VerifyInstallPicker(root, chs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data-cmd-windows")
}

// A channel without command-windows must render no <noscript>
// fallback; a stray one fails closed.
func TestVerifyInstallPicker_FailsOnStrayNoscript(t *testing.T) {
	chs := []Channel{{Title: "Go", Command: "go install x", Platforms: []string{"linux"}}}
	page := `<html><body><div class="install-picker">` +
		`<div class="install-row" data-cmd-default="go install x">` +
		`<noscript><code class="install-cmd install-cmd-noscript">` +
		`<span class="cmd">x</span></code></noscript>` +
		`</div></div></body></html>`
	root := writePickerSite(t, page)
	err := VerifyInstallPicker(root, chs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stray <noscript>")
}
