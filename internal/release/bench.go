// Package release: the cross-tool benchmark harness.
//
// This file ports docs/research/benchmarks/run.sh into the
// mdsmith-release Go CLI so the local hand-refresh and the
// post-merge `benchmark.yml` workflow run byte-identical logic
// (see docs/development/release-tooling.md: workflow runtime
// logic goes through this binary, not inline shell).
//
// What Go owns now that the shell script delegated or skipped:
//
//   - Integrity. Every comparison binary is fetched at a pinned
//     version and verified by SHA-256 before it runs. run.sh
//     pinned hyperfine/mado/panache by release tag but never
//     checksummed them, and pulled rumdl (uv) and
//     markdownlint-cli2 (npm) unpinned. A tampered or
//     silently-rebuilt tarball now fails the run loudly.
//   - markdownlint-cli2 installs from a committed lockfile via
//     `npm ci` (docs/research/benchmarks/npm/), not an unpinned
//     `npm i`.
//
// What is unchanged on purpose: the corpora construction, the
// exact hyperfine flags/commands, the JSON promoted into
// docs/research/benchmarks/data/, and the fragment regeneration
// — the last still shells out to the existing gen_fragments.py
// so the `bench-fragments` drift gate keeps validating the
// committed snapshot against the committed JSON with one
// generator.
package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// benchDirRel is the benchmark harness directory relative to the
// repo root. The committed JSON snapshot, the fragment files, the
// parity config, the npm lockfile, and gen_fragments.py all live
// under it.
const benchDirRel = "docs/research/benchmarks"

// defaultBenchWorkdir is where the harness caches the built
// mdsmith binary, the fetched tool binaries, the corpora, and the
// raw hyperfine output. Matches run.sh's historical default so a
// local rerun reuses the same cache.
const defaultBenchWorkdir = "/tmp/mdsmith-bench"

// benchTool is one pinned, integrity-verified comparison binary
// fetched from a GitHub release tarball. Name is BOTH the
// hyperfine `--command-name` and the executable's basename inside
// the archive — true for all four tools (hyperfine, mado,
// panache, rumdl), whose tarballs differ only in directory
// nesting. URL embeds the version tag so a version bump that
// forgets to refresh the pin trips validateBenchManifest. SHA256
// is the lowercase-hex digest of the downloaded `.tar.gz`.
type benchTool struct {
	Name    string
	Version string
	URL     string
	SHA256  string
}

// benchTools is the pinned manifest. rumdl moved here from
// run.sh's unpinned `uv tool install rumdl`: its GitHub release
// ships a linux-gnu tarball with a companion `.sha256`, so it
// joins the same fetch+verify path as the other three rather
// than needing a separate uv install with no integrity. The
// digests were recorded by downloading each tarball once;
// rumdl's was cross-checked against the publisher's `.sha256`.
// markdownlint-cli2 is intentionally absent — it installs from a
// committed npm lockfile via `npm ci`, not a tarball.
func benchTools() []benchTool {
	return []benchTool{
		{
			Name:    "hyperfine",
			Version: "v1.20.0",
			URL: "https://github.com/sharkdp/hyperfine/releases/download/" +
				"v1.20.0/hyperfine-v1.20.0-x86_64-unknown-linux-musl.tar.gz",
			SHA256: "3285ec7959285288137043dd81dce0dde056227018a8277532d9a364b4f03c2b",
		},
		{
			Name:    "mado",
			Version: "v0.3.0",
			URL: "https://github.com/akiomik/mado/releases/download/" +
				"v0.3.0/mado-Linux-gnu-x86_64.tar.gz",
			SHA256: "aad845cd23c8c0417cdf87b8376b75e131c38e1cb890124790567735306de6d7",
		},
		{
			Name:    "panache",
			Version: "v2.46.0",
			URL: "https://github.com/jolars/panache/releases/download/" +
				"v2.46.0/panache-x86_64-unknown-linux-gnu.tar.gz",
			SHA256: "898d5cc90df921745cca2c9e058bc2b99aeebeac9e16a1a9a5f8bd1b7fb655b5",
		},
		{
			Name:    "rumdl",
			Version: "v0.1.93",
			URL: "https://github.com/rvben/rumdl/releases/download/" +
				"v0.1.93/rumdl-v0.1.93-x86_64-unknown-linux-gnu.tar.gz",
			SHA256: "033f48f4601b6533dfcc48112621defc6097a6f5609d187fa8149f94789d3f59",
		},
	}
}

// benchJSONNames are the four hyperfine JSON exports the harness
// promotes into docs/research/benchmarks/data/. gen_fragments.py
// reads exactly these names; the `bench-fragments` gate diffs the
// fragments regenerated from the committed copies.
var benchJSONNames = []string{
	"corpus_repo",
	"corpus_repo_mdl",
	"corpus_neutral",
	"corpus_neutral_mdl",
}

// sha256Hex matches a lowercase 64-hex-character SHA-256 digest.
var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// validateBenchManifest enforces the invariants every pinned
// entry must hold so a future edit cannot land a half-specified
// or unpinned tool. It is covered by a unit test and also runs at
// the top of Bench so a bad manifest fails before any download.
func validateBenchManifest(tools []benchTool) error {
	if len(tools) == 0 {
		return fmt.Errorf("bench manifest is empty")
	}
	seen := make(map[string]struct{}, len(tools))
	for _, t := range tools {
		switch {
		case t.Name == "":
			return fmt.Errorf("bench manifest entry has empty name")
		case t.Version == "":
			return fmt.Errorf("%s: empty version", t.Name)
		case t.URL == "":
			return fmt.Errorf("%s: empty url", t.Name)
		case t.SHA256 == "":
			return fmt.Errorf("%s: empty sha256", t.Name)
		}
		if _, dup := seen[t.Name]; dup {
			return fmt.Errorf("%s: duplicate manifest entry", t.Name)
		}
		seen[t.Name] = struct{}{}
		if !strings.HasPrefix(t.URL, "https://github.com/") {
			return fmt.Errorf("%s: url is not a github.com release URL: %s", t.Name, t.URL)
		}
		if !strings.HasSuffix(t.URL, ".tar.gz") {
			return fmt.Errorf("%s: url is not a .tar.gz: %s", t.Name, t.URL)
		}
		// The release tag segment must carry the pinned version,
		// so bumping Version without re-pinning the URL (or vice
		// versa) is a loud failure rather than a silent
		// version/integrity skew.
		if !strings.Contains(t.URL, "/download/"+t.Version+"/") {
			return fmt.Errorf("%s: url does not pin version %s: %s", t.Name, t.Version, t.URL)
		}
		if !sha256Hex.MatchString(t.SHA256) {
			return fmt.Errorf("%s: sha256 is not 64 lowercase hex chars: %q", t.Name, t.SHA256)
		}
	}
	return nil
}

// verifyChecksum reports whether data hashes to the pinned
// lowercase-hex SHA-256 want. A mismatch returns an error naming
// both digests so a tampered or silently-rebuilt download fails
// the run loudly (and legibly in CI logs). Pure and unit-tested.
func verifyChecksum(data []byte, want string) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != strings.ToLower(want) {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, want)
	}
	return nil
}

// extractTarGzBinary pulls the single executable named binName
// out of a gzip-compressed tar archive and writes it to dstPath
// with 0o755. The four tool tarballs nest the binary differently
// (hyperfine under a versioned dir, panache under "./", mado and
// rumdl at the root), so the match is on path.Base of a regular
// file rather than a hardcoded member path — the same robustness
// run.sh got from `find -name <tool> -type f`.
func extractTarGzBinary(r io.Reader, binName, dstPath string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("binary %q not found in archive", binName)
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || path.Base(hdr.Name) != binName {
			continue
		}
		//nolint:gosec // CI-only; dstPath is harness-controlled
		out, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return fmt.Errorf("create %s: %w", dstPath, err)
		}
		// Cap the copy so a crafted archive member cannot fill
		// the runner disk. 64 MiB dwarfs every real binary here
		// (the largest tarball is ~5 MB compressed).
		if _, err := io.CopyN(out, tr, 64<<20); err != nil && err != io.EOF {
			_ = out.Close()
			return fmt.Errorf("extract %s: %w", binName, err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("close %s: %w", dstPath, err)
		}
		return nil
	}
}

// promoteBenchJSON copies the four hyperfine JSON exports from the
// raw output directory into docs/research/benchmarks/data/, the
// committed source of truth gen_fragments.py reads. A missing
// export is a hard error (the run did not produce what the
// fragments depend on) rather than a silent stale promote.
func (t *Toolkit) promoteBenchJSON(outDir, dataDir string) error {
	if err := t.fs.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dataDir, err)
	}
	for _, name := range benchJSONNames {
		src := filepath.Join(outDir, name+".json")
		data, err := t.fs.ReadFile(src)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("benchmark output %s.json not found in %s", name, outDir)
			}
			return fmt.Errorf("read %s: %w", src, err)
		}
		dst := filepath.Join(dataDir, name+".json")
		if err := t.fs.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
	}
	return nil
}

// Bench runs the full cross-tool benchmark from the repo root:
// build mdsmith, fetch + integrity-verify the pinned comparison
// binaries, build the two corpora, drive hyperfine, promote the
// JSON into the committed data dir, and regenerate the doc
// fragments via gen_fragments.py. workdir caches every heavy
// artifact (built binaries, corpora) so a local rerun is cheap;
// CI passes a fresh /tmp path so every run is cold.
//
// It deliberately does NOT run `mdsmith fix` to refresh the
// <?include?> bodies — run.sh never did either, and the
// benchmark.yml workflow owns that normalization before
// publishing to the assets branch.
func (t *Toolkit) Bench(root, workdir string) error {
	if err := validateBenchManifest(benchTools()); err != nil {
		return fmt.Errorf("bench manifest: %w", err)
	}
	if workdir == "" {
		workdir = defaultBenchWorkdir
	}
	binDir := filepath.Join(workdir, "bin")
	outDir := filepath.Join(workdir, "out")
	for _, d := range []string{binDir, outDir} {
		if err := t.fs.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	mdsmithBin := filepath.Join(binDir, "mdsmith")
	if !t.exists(mdsmithBin) {
		fmt.Println("bench: building mdsmith")
		if err := t.runner.RunCommand(root, "go", "build", "-o", mdsmithBin, "./cmd/mdsmith"); err != nil {
			return fmt.Errorf("build mdsmith: %w", err)
		}
	}

	for _, tool := range benchTools() {
		dst := filepath.Join(binDir, tool.Name)
		if t.exists(dst) {
			continue
		}
		if err := t.fetchTool(tool, dst); err != nil {
			return err
		}
	}

	mdlBin, err := t.installMarkdownlint(root, workdir)
	if err != nil {
		return err
	}

	if err := t.buildCorpora(root, workdir); err != nil {
		return err
	}

	if err := t.runHyperfine(binDir, outDir, workdir, mdsmithBin, mdlBin, root); err != nil {
		return err
	}

	dataDir := filepath.Join(root, benchDirRel, "data")
	if err := t.promoteBenchJSON(outDir, dataDir); err != nil {
		return err
	}

	fmt.Println("bench: regenerating fragments via gen_fragments.py")
	gen := filepath.Join(root, benchDirRel, "gen_fragments.py")
	if err := t.runner.RunCommand(root, "python3", gen, dataDir, filepath.Join(root, benchDirRel)); err != nil {
		return fmt.Errorf("gen_fragments.py: %w", err)
	}
	return nil
}

// exists reports whether name is present (file or dir). Used for
// the skip-if-cached checks that make a local rerun cheap.
func (t *Toolkit) exists(name string) bool {
	_, err := t.fs.Stat(name)
	return err == nil
}

// fetchTool downloads one pinned tarball, fails loudly on a
// SHA-256 mismatch, and extracts its binary to dst.
func (t *Toolkit) fetchTool(tool benchTool, dst string) error {
	fmt.Printf("bench: fetching %s %s\n", tool.Name, tool.Version)
	status, body, err := t.http.Get(tool.URL)
	if err != nil {
		return fmt.Errorf("download %s: %w", tool.Name, err)
	}
	if status != 200 {
		return fmt.Errorf("download %s: %s returned HTTP %d", tool.Name, tool.URL, status)
	}
	if err := verifyChecksum(body, tool.SHA256); err != nil {
		return fmt.Errorf("%s: %w", tool.Name, err)
	}
	if err := extractTarGzBinary(bytes.NewReader(body), tool.Name, dst); err != nil {
		return fmt.Errorf("%s: %w", tool.Name, err)
	}
	return nil
}

// installMarkdownlint stages the committed npm lockfile into the
// workdir and runs `npm ci`, returning the path to the resolved
// markdownlint-cli2 binary. `npm ci` (not `npm i`) installs the
// exact tree the committed package-lock.json pins, so the
// benchmark sees the same markdownlint-cli2 every run.
func (t *Toolkit) installMarkdownlint(root, workdir string) (string, error) {
	npmDir := filepath.Join(workdir, "npm")
	if err := t.fs.MkdirAll(npmDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", npmDir, err)
	}
	srcDir := filepath.Join(root, benchDirRel, "npm")
	for _, f := range []string{"package.json", "package-lock.json"} {
		data, err := t.fs.ReadFile(filepath.Join(srcDir, f))
		if err != nil {
			return "", fmt.Errorf("read pinned %s: %w", f, err)
		}
		if err := t.fs.WriteFile(filepath.Join(npmDir, f), data, 0o644); err != nil {
			return "", fmt.Errorf("stage %s: %w", f, err)
		}
	}
	bin := filepath.Join(npmDir, "node_modules", ".bin", "markdownlint-cli2")
	if t.exists(bin) {
		return bin, nil
	}
	fmt.Println("bench: npm ci markdownlint-cli2")
	if err := t.runner.RunCommand(npmDir, "npm", "ci"); err != nil {
		return "", fmt.Errorf("npm ci: %w", err)
	}
	return bin, nil
}

// repoCorpusSkip drops generated/bad fixtures from corpus A so the
// repo corpus is the Markdown a user would actually lint. Matches
// run.sh's `grep -vE '^(demo/|internal/rules/[^/]*/bad/)'`.
var repoCorpusSkip = regexp.MustCompile(`^(demo/|internal/rules/[^/]*/bad/)`)

// buildCorpora materializes the two benchmark corpora under
// workdir, skipping each if already present (cheap local rerun):
//
//   - corpus_repo: this repo's own tracked Markdown, fixtures
//     dropped (git ls-files, filtered).
//   - corpus_neutral: third-party prose — the Rust Book and Rust
//     Reference `src/` trees, shallow-cloned.
func (t *Toolkit) buildCorpora(root, workdir string) error {
	repoCorpus := filepath.Join(workdir, "corpus_repo")
	if !t.exists(repoCorpus) {
		fmt.Println("bench: building corpus_repo")
		lsArgs := []string{"-C", root, "ls-files", "*.md", "*.markdown"}
		out, err := exec.Command("git", lsArgs...).Output() //nolint:gosec // CI-only; args constant
		if err != nil {
			return fmt.Errorf("git ls-files: %w", err)
		}
		for _, rel := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if rel == "" || repoCorpusSkip.MatchString(rel) {
				continue
			}
			if err := t.copyInto(filepath.Join(root, rel), filepath.Join(repoCorpus, rel)); err != nil {
				return err
			}
		}
	}

	neutralCorpus := filepath.Join(workdir, "corpus_neutral")
	if !t.exists(neutralCorpus) {
		fmt.Println("bench: building corpus_neutral (cloning Rust Book + Reference)")
		clones := []struct{ url, dir string }{
			{"https://github.com/rust-lang/book.git", "rust-book"},
			{"https://github.com/rust-lang/reference.git", "rust-ref"},
		}
		for _, c := range clones {
			dir := filepath.Join(workdir, c.dir)
			if t.exists(dir) {
				continue
			}
			if err := t.runner.RunCommand(workdir, "git", "clone", "--depth", "1", "-q", c.url, c.dir); err != nil {
				return fmt.Errorf("clone %s: %w", c.url, err)
			}
		}
		for _, src := range []string{
			filepath.Join(workdir, "rust-book", "src"),
			filepath.Join(workdir, "rust-ref", "src"),
		} {
			if err := t.copyMarkdownTree(src, neutralCorpus); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyInto copies one file, creating parent directories. Used for
// the repo corpus where each path is reproduced verbatim under
// corpus_repo/.
func (t *Toolkit) copyInto(src, dst string) error {
	data, err := t.fs.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := t.fs.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := t.fs.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

// copyMarkdownTree walks srcRoot and copies every *.md file into
// dstRoot, reproducing the source path under it (leading
// separator stripped) — the same layout run.sh's
// `find … | tar … | tar -x` produced, so hyperfine walks an
// identical corpus.
func (t *Toolkit) copyMarkdownTree(srcRoot, dstRoot string) error {
	return filepath.WalkDir(srcRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		rel := strings.TrimPrefix(filepath.ToSlash(p), "/")
		return t.copyInto(p, filepath.Join(dstRoot, filepath.FromSlash(rel)))
	})
}

// runHyperfine drives the two benchmark passes per corpus with
// the exact flags run.sh used. The comparison command strings use
// absolute binary paths rather than bare names: with `-N`
// hyperfine tokenizes the string itself (no PATH-providing
// shell), and the JSON `command` field is the `--command-name`,
// not the executable path, so the promoted JSON — and therefore
// every regenerated fragment — is identical to run.sh's.
func (t *Toolkit) runHyperfine(binDir, outDir, workdir, mdsmithBin, mdlBin, root string) error {
	hyperfine := filepath.Join(binDir, "hyperfine")
	parity := filepath.Join(root, benchDirRel, "bench-parity.mdsmith.yml")
	mado := filepath.Join(binDir, "mado")
	rumdl := filepath.Join(binDir, "rumdl")
	panache := filepath.Join(binDir, "panache")
	for _, corpus := range []string{"corpus_repo", "corpus_neutral"} {
		cpath := filepath.Join(workdir, corpus)
		fmt.Printf("bench: hyperfine over %s\n", corpus)
		if err := t.runner.RunCommand("", hyperfine,
			"-i", "--warmup", "3", "--runs", "10", "-N",
			"--command-name", "mado", mado+" check "+cpath,
			"--command-name", "rumdl", rumdl+" check --no-cache "+cpath,
			"--command-name", "panache", panache+" lint --no-cache "+cpath,
			"--command-name", "mdsmith-parity", mdsmithBin+" check -c "+parity+" "+cpath,
			"--command-name", "mdsmith", mdsmithBin+" check "+cpath,
			"--export-json", filepath.Join(outDir, corpus+".json"),
			"--export-markdown", filepath.Join(outDir, corpus+".md"),
		); err != nil {
			return fmt.Errorf("hyperfine %s: %w", corpus, err)
		}
		if err := t.runner.RunCommand("", hyperfine,
			"-i", "--warmup", "2", "--runs", "6", "-N",
			"--command-name", "markdownlint-cli2", mdlBin+" '"+cpath+"/**/*.md'",
			"--export-json", filepath.Join(outDir, corpus+"_mdl.json"),
		); err != nil {
			return fmt.Errorf("hyperfine %s markdownlint: %w", corpus, err)
		}
	}
	return nil
}

// Bench delegates to a default-OS Toolkit (see Stamp).
func Bench(root, workdir string) error {
	return New().Bench(root, workdir)
}
