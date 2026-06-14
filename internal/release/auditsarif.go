package release

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// auditDateLayout is the leading date format every audit directory
// under docs/security/ is named with (YYYY-MM-DD-<slug>).
const auditDateLayout = "2006-01-02"

// SelectAuditSarifs returns the basenames of the audit directories
// directly under securityDir that share the most recent audit date and
// carry a findings.sarif. The security-audit-sarif workflow feeds the
// result into its upload matrix — one code-scanning category per
// directory.
//
// A directory qualifies when its name begins with a valid YYYY-MM-DD
// date (followed by end-of-name or a '-' slug separator), that date is
// not in the future relative to now, and it contains a regular
// findings.sarif file. "Most recent" is the maximum qualifying date;
// every directory on that date is returned, since one date can hold
// more than one audit (e.g. a full-repo audit beside a narrower-scope
// one). The result is sorted. It is empty — not an error — when
// securityDir is absent or holds no qualifying directory, so the
// caller can skip the upload rather than fail the run. A genuine read
// error (e.g. permissions) is returned so the workflow fails loudly
// instead of silently uploading nothing.
func SelectAuditSarifs(securityDir string, now time.Time) ([]string, error) {
	entries, err := os.ReadDir(securityDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read audit directory %s: %w", securityDir, err)
	}

	y, mo, d := now.UTC().Date()
	today := time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)

	type audit struct {
		name string
		date time.Time
	}
	var cands []audit
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		date, ok := leadingAuditDate(e.Name())
		if !ok || date.After(today) {
			continue
		}
		sarif := filepath.Join(securityDir, e.Name(), "findings.sarif")
		info, statErr := os.Stat(sarif)
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		cands = append(cands, audit{name: e.Name(), date: date})
	}
	if len(cands) == 0 {
		return nil, nil
	}

	newest := cands[0].date
	for _, c := range cands[1:] {
		if c.date.After(newest) {
			newest = c.date
		}
	}
	var out []string
	for _, c := range cands {
		if c.date.Equal(newest) {
			out = append(out, c.name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// leadingAuditDate parses the YYYY-MM-DD date a qualifying audit
// directory name starts with. It rejects names whose first ten bytes
// are not a real calendar date, and names where the date is run into
// the slug without a '-' separator (e.g. "2026-06-12x"), so only
// genuine audit directories match.
func leadingAuditDate(name string) (time.Time, bool) {
	if len(name) < len(auditDateLayout) {
		return time.Time{}, false
	}
	if len(name) > len(auditDateLayout) && name[len(auditDateLayout)] != '-' {
		return time.Time{}, false
	}
	t, err := time.Parse(auditDateLayout, name[:len(auditDateLayout)])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
