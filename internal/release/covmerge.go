package release

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// covBlock is one coverage record's identity. The trailing hit
// count is accumulated separately so duplicate records (the same
// block measured by both the unit-test profile and the e2e binary
// profile) are merged by summing hits instead of last-write-wins.
type covBlock struct {
	// key is the full record minus the trailing hit count, e.g.
	// "cmd/mdsmith/extract.go:26.31,30.3 3". Identical source
	// produces identical keys across profiles.
	key string
	// startKey is "file:000000026" for a stable emission order.
	startKey string
}

// MergeCoverage merges Go coverage profiles by summing per-block
// hit counts and writes the result to output.
//
// The naive `cat a.cov b.cov` the CI used before left duplicate
// records for cmd/mdsmith files (one from the per-package unit
// run, one from the e2e subprocess binary). Codecov's Go parser
// is last-write-wins on duplicate blocks, so the e2e profile's
// zero counts for lines exercised only by in-process tests
// clobbered real coverage and made the patch percentage swing
// with e2e flush timing. Summing is the correct merge: a block is
// covered if any profile observed a hit.
//
// All inputs must share one mode line. `set` mode counts are
// boolean, so they are OR-ed (max); `count` / `atomic` hits are
// summed.
func MergeCoverage(inputs []string, output string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("no input coverage profiles")
	}
	mode := ""
	counts := make(map[string]int)
	meta := make(map[string]covBlock)

	for _, in := range inputs {
		m, err := accumulateProfile(in, counts, meta)
		if err != nil {
			return err
		}
		if mode == "" {
			mode = m
		} else if m != mode {
			return fmt.Errorf("coverage mode mismatch: %q vs %q (%s)", mode, m, in)
		}
	}
	// mode is non-empty here: len(inputs) >= 1 and accumulateProfile
	// errors out (returning above) for any input lacking a mode line.

	blocks := make([]covBlock, 0, len(meta))
	for _, b := range meta {
		blocks = append(blocks, b)
	}
	sort.Slice(blocks, func(i, j int) bool {
		return lessBlock(blocks[i], blocks[j])
	})

	var b strings.Builder
	fmt.Fprintf(&b, "mode: %s\n", mode)
	for _, blk := range blocks {
		fmt.Fprintf(&b, "%s %d\n", blk.key, counts[blk.key])
	}
	return os.WriteFile(output, []byte(b.String()), 0o644)
}

// accumulateProfile folds one profile's records into counts/meta.
// Returns the profile's mode. Blank lines are tolerated; a
// malformed record line is a hard error so a truncated profile
// never silently undercounts.
func accumulateProfile(
	path string, counts map[string]int, meta map[string]covBlock,
) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	mode := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "mode: ") {
			mode = strings.TrimSpace(strings.TrimPrefix(line, "mode: "))
			continue
		}
		key, hits, err := parseCovLine(line)
		if err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		if mode == "set" {
			if hits > 0 {
				counts[key] = 1
			} else if _, seen := counts[key]; !seen {
				counts[key] = 0
			}
		} else {
			counts[key] += hits
		}
		if _, ok := meta[key]; !ok {
			meta[key] = covBlock{key: key, startKey: covStartKey(key)}
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	if mode == "" {
		return "", fmt.Errorf("%s: missing mode line", path)
	}
	return mode, nil
}

// parseCovLine splits "file.go:s.c,e.c numStmt count" into the
// key (everything but the trailing count) and the hit count. The
// statement count stays inside key so identical blocks across
// profiles collide exactly.
func parseCovLine(line string) (key string, hits int, err error) {
	lastSp := strings.LastIndexByte(line, ' ')
	if lastSp < 0 {
		return "", 0, fmt.Errorf("malformed coverage line %q", line)
	}
	hits, err = strconv.Atoi(line[lastSp+1:])
	if err != nil {
		return "", 0, fmt.Errorf("bad hit count in %q: %w", line, err)
	}
	key = line[:lastSp]
	if strings.LastIndexByte(key, ' ') < 0 {
		return "", 0, fmt.Errorf("malformed coverage line %q", line)
	}
	return key, hits, nil
}

// lessBlock orders blocks by startKey (file then start line) and
// breaks ties with the full record key, so blocks sharing a
// file:line (multiple statements on one source line) emit in a
// deterministic order rather than inheriting Go's randomized map
// iteration.
func lessBlock(a, b covBlock) bool {
	if a.startKey != b.startKey {
		return a.startKey < b.startKey
	}
	return a.key < b.key
}

// covStartKey returns a left-padded "file:line" sort key so the
// merged profile is emitted in a stable order regardless of input
// order or Go map iteration.
func covStartKey(key string) string {
	colon := strings.LastIndexByte(key, ':')
	if colon < 0 {
		return key
	}
	file := key[:colon]
	rest := key[colon+1:]
	// A well-formed record's coordinates are `startLine.startCol,…`.
	// Bail out (returning the raw key as its own sort value) on any
	// shape that lacks the dot or comma rather than slicing with a
	// -1 index and panicking — covStartKey runs on CI-supplied
	// profiles and must degrade, not crash, on a malformed line.
	dot := strings.IndexByte(rest, '.')
	comma := strings.IndexByte(rest, ',')
	if dot < 0 || comma < 0 || dot > comma {
		return key
	}
	n, err := strconv.Atoi(rest[:dot])
	if err != nil {
		return key
	}
	return fmt.Sprintf("%s:%09d", file, n)
}
