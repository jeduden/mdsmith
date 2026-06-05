package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ChannelKind is the kind name registered in .mdsmith.yml that
// drives `mdsmith extract` over the per-channel source files.
const ChannelKind = "release-channel"

// ChannelDir holds one Markdown file per distribution channel —
// the single source of truth the install/release tables and the
// website install picker all derive from.
const ChannelDir = "docs/development/release-channels"

// ChannelsDataFile is the Hugo data file the install picker reads.
// It is generated, never edited by hand.
const ChannelsDataFile = "website/data/channels.yaml"

// Channel is the projected, picker-facing shape of one channel.
// The yaml tags define the website/data/channels.yaml schema the
// install-picker partial consumes.
type Channel struct {
	Title     string `yaml:"title"`
	Summary   string `yaml:"summary"`
	Mechanism string `yaml:"mechanism"`
	Artifact  string `yaml:"artifact"`
	Command   string `yaml:"command"`
	// CommandWindows is an optional Windows-specific install
	// command. Windows ships a single `.exe` asset, so a generic
	// `<os>-<arch>` `command` placeholder is not runnable there;
	// when set, the install picker shows this concrete line under
	// the Windows filter. Omitted from the data file when empty
	// (every channel but the GitHub release today).
	CommandWindows string   `yaml:"command-windows,omitempty"`
	Audience       string   `yaml:"audience"`
	Platforms      []string `yaml:"platforms"`
	URL            string   `yaml:"url"`
	Weight         int      `yaml:"weight"`
}

// channelDoc mirrors the `frontmatter` object that
// `mdsmith extract release-channel <file>` emits. Only the
// frontmatter feeds the picker; the body is documentation.
type channelDoc struct {
	Frontmatter struct {
		Title          string   `json:"title"`
		Summary        string   `json:"summary"`
		Mechanism      string   `json:"mechanism"`
		Artifact       string   `json:"artifact"`
		Command        string   `json:"command"`
		CommandWindows string   `json:"command-windows"`
		Audience       string   `json:"audience"`
		Platforms      []string `json:"platforms"`
		ChannelURL     string   `json:"channelurl"`
		Weight         int      `json:"weight"`
	} `json:"frontmatter"`
}

// channelsExtractAll is the package-level seam tests stub so they
// run without the real mdsmith binary. Production builds mdsmith
// once and projects every listed file through it.
var channelsExtractAll = extractAllChannels

// extractAllChannels builds the mdsmith binary once, then runs
// `extract release-channel <file> --format json` per file and
// returns the raw JSON keyed by the file's slash path. Building
// once avoids a per-file `go run` relink (12 files today, growing).
func extractAllChannels(root string, rels []string) (map[string][]byte, error) {
	bin, cleanup, err := buildMdsmith(root)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	out := make(map[string][]byte, len(rels))
	for _, rel := range rels {
		b, err := runExtract(bin, root, rel)
		if err != nil {
			return nil, err
		}
		out[rel] = b
	}
	return out, nil
}

// buildMdsmith compiles ./cmd/mdsmith into a temp dir and returns
// the binary path plus a cleanup func.
func buildMdsmith(root string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "mdsmith-extract-*")
	if err != nil {
		return "", nil, fmt.Errorf("tempdir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	bin := filepath.Join(dir, mdsmithBinName(runtime.GOOS))
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/mdsmith") //nolint:gosec // CI-only; constant args
	cmd.Dir = root
	if combined, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("build mdsmith: %w (%s)", err, combined)
	}
	return bin, cleanup, nil
}

// mdsmithBinName is the built binary's filename for a given GOOS;
// Windows needs the .exe suffix for exec.Command to launch it.
func mdsmithBinName(goos string) string {
	if goos == "windows" {
		return "mdsmith.exe"
	}
	return "mdsmith"
}

func runExtract(bin, root, rel string) ([]byte, error) {
	cmd := exec.Command(bin, "extract", ChannelKind, rel, "--format", "json") //nolint:gosec // CI-only; dir-listed name
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("mdsmith extract %s: %w (stderr: %s)",
				rel, err, ee.Stderr)
		}
		return nil, fmt.Errorf("mdsmith extract %s: %w", rel, err)
	}
	return out, nil
}

// channelFiles lists the channel source basenames (sorted),
// excluding the proto.md schema file.
func channelFiles(root string) ([]string, error) {
	dir := filepath.Join(root, filepath.FromSlash(ChannelDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read channel dir: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || name == "proto.md" {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)
	return files, nil
}

// LoadChannels projects every channel file through
// `mdsmith extract` and returns them sorted by ascending weight.
// Extraction is schema-gated, so a non-conformant channel file
// fails here with the same message `mdsmith check` would print.
func LoadChannels(root string) ([]Channel, error) {
	files, err := channelFiles(root)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no channel files found under %s", ChannelDir)
	}
	rels := make([]string, len(files))
	for i, name := range files {
		rels[i] = filepath.ToSlash(filepath.Join(ChannelDir, name))
	}
	raw, err := channelsExtractAll(root, rels)
	if err != nil {
		return nil, err
	}
	chs := make([]Channel, 0, len(rels))
	for _, rel := range rels {
		out, ok := raw[rel]
		if !ok {
			return nil, fmt.Errorf("extract: no output for %s", rel)
		}
		var doc channelDoc
		if err := json.Unmarshal(out, &doc); err != nil {
			return nil, fmt.Errorf("decode %s json: %w", rel, err)
		}
		f := doc.Frontmatter
		ch := Channel{
			Title:          f.Title,
			Summary:        f.Summary,
			Mechanism:      f.Mechanism,
			Artifact:       f.Artifact,
			Command:        f.Command,
			CommandWindows: f.CommandWindows,
			Audience:       f.Audience,
			Platforms:      f.Platforms,
			URL:            f.ChannelURL,
			Weight:         f.Weight,
		}
		if err := ch.validate(rel); err != nil {
			return nil, err
		}
		chs = append(chs, ch)
	}
	sort.SliceStable(chs, func(i, j int) bool {
		return chs[i].Weight < chs[j].Weight
	})
	return chs, nil
}

// validate fails fast if a required projected field is empty. The
// proto.md schema's CUE constraints catch the same condition under
// `mdsmith check`; this keeps sync-channels self-contained.
func (c Channel) validate(src string) error {
	var missing []string
	for _, p := range []struct{ name, value string }{
		{"title", c.Title},
		{"summary", c.Summary},
		{"mechanism", c.Mechanism},
		{"artifact", c.Artifact},
		{"command", c.Command},
		{"audience", c.Audience},
		{"url", c.URL},
	} {
		if strings.TrimSpace(p.value) == "" {
			missing = append(missing, p.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("channel %s: empty field(s): %s",
			src, strings.Join(missing, ", "))
	}
	if c.Weight < 1 {
		return fmt.Errorf("channel %s: weight must be >= 1, got %d",
			src, c.Weight)
	}
	return nil
}

// channelsHeader is the do-not-edit banner the data file carries.
const channelsHeader = "# Generated by `mdsmith-release sync-channels` from\n" +
	"# docs/development/release-channels/*.md — do not edit by hand.\n"

// RenderChannelsYAML marshals channels into the website data file
// body, prefixed with the generated-file banner. A Channel holds
// only strings, ints, and string slices, so yaml.Marshal cannot
// fail; per the repo's "defensive branch only when drivable" rule
// we do not wrap an unreachable error path.
func RenderChannelsYAML(chs []Channel) []byte {
	body, _ := yaml.Marshal(chs) //nolint:errcheck // marshal of plain scalars cannot fail
	return append([]byte(channelsHeader), body...)
}

// channelsDataPath is the absolute data-file path under root.
func channelsDataPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(ChannelsDataFile))
}

// WriteChannelsData regenerates ChannelsDataFile from pre-loaded
// channels and reports whether the on-disk file changed. Splitting
// the write from LoadChannels keeps the apply path unit-testable
// without the `mdsmith extract` shell-out (the sync-messaging
// split is the precedent).
func WriteChannelsData(root string, chs []Channel) (bool, error) {
	out := RenderChannelsYAML(chs)
	path := channelsDataPath(root)
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read %s: %w", ChannelsDataFile, err)
	}
	if bytes.Equal(old, out) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("mkdir website data: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", ChannelsDataFile, err)
	}
	return true, nil
}

// CheckChannelsData reports whether ChannelsDataFile is out of date
// with respect to the pre-loaded channels. A missing file counts as
// drift.
func CheckChannelsData(root string, chs []Channel) (bool, error) {
	out := RenderChannelsYAML(chs)
	old, err := os.ReadFile(channelsDataPath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read %s: %w", ChannelsDataFile, err)
	}
	return !bytes.Equal(old, out), nil
}

// LoadChannelsFromDataFile reads the generated channels.yaml under
// root and unmarshals it into the channel list the install picker
// renders from. Unlike LoadChannels, which re-derives the list from
// the per-channel docs frontmatter, this returns exactly what the
// Hugo template consumed, so a render probe compares the page against
// its true input rather than a parallel source.
func LoadChannelsFromDataFile(root string) ([]Channel, error) {
	path := filepath.Join(root, ChannelsDataFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ChannelsDataFile, err)
	}
	var chs []Channel
	if err := yaml.Unmarshal(data, &chs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ChannelsDataFile, err)
	}
	return chs, nil
}
