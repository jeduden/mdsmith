package secreview

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Constraints is the machine-checkable subset of a case rubric: the
// severities that must not appear, and an optional required finding at or
// above a severity floor (optionally pinned to a primary-location file).
type Constraints struct {
	// ForbidSeverities lists severities that fail the grade on sight.
	ForbidSeverities []string
	// RequireMinSeverity, when set, demands at least one finding ranked at
	// or above it. Empty means no required-finding check.
	RequireMinSeverity string
	// RequireLocationFile, when set, narrows the required finding to ones
	// whose PRIMARY location file equals it (related_locations do not count).
	RequireLocationFile string
}

// normSeverity lowercases value and verifies it is one of the five
// severities, returning a clean error (mentioning what) otherwise.
func normSeverity(value, what string) (string, error) {
	sev := strings.ToLower(value)
	if !SeverityKnown(sev) {
		return "", fmt.Errorf("%s: unknown severity %q (want one of %s)",
			what, value, strings.Join(knownSeverityList, ", "))
	}
	return sev, nil
}

// BuildConstraints validates and normalizes raw rubric inputs into
// Constraints. It mirrors grade.py's build_constraints: a required location
// file needs a min severity, and a rubric with no forbid and no required
// finding is rejected as vacuous so a typo can never assert nothing.
func BuildConstraints(forbid []string, reqMin, reqFile string) (Constraints, error) {
	normForbid := make([]string, 0, len(forbid))
	for _, s := range forbid {
		ns, err := normSeverity(s, "forbid_severities")
		if err != nil {
			return Constraints{}, err
		}
		normForbid = append(normForbid, ns)
	}
	var normMin string
	if reqMin != "" {
		ns, err := normSeverity(reqMin, "require min_severity")
		if err != nil {
			return Constraints{}, err
		}
		normMin = ns
	}
	if reqFile != "" && normMin == "" {
		return Constraints{}, fmt.Errorf("a required location_file needs a min_severity")
	}
	if len(normForbid) == 0 && normMin == "" {
		return Constraints{}, fmt.Errorf("vacuous rubric: set forbid_severities and/or require_finding")
	}
	return Constraints{
		ForbidSeverities:    normForbid,
		RequireMinSeverity:  normMin,
		RequireLocationFile: reqFile,
	}, nil
}

// ValidateFindings enforces the same strictness as render_findings.py
// and normalizes in place: every finding must carry a non-empty id and a
// non-empty severity that is one of the five, and the severity is
// lowercased so render and grade both see a canonical value. The Python
// load() did `f["severity"] = sev`; without this a "Critical" finding
// passes validation but renders with a blank SARIF level. A non-empty id
// is required because buildSARIF keys rules by id, so two empty-id
// findings would otherwise collapse into one mislabeled rule.
func ValidateFindings(fs []Finding) error {
	for i := range fs {
		f := &fs[i]
		if f.ID == "" {
			return fmt.Errorf("finding #%d has no id", i)
		}
		if f.Severity == "" {
			return fmt.Errorf("finding %q has no severity", f.ID)
		}
		sev, err := normSeverity(f.Severity, fmt.Sprintf("finding %q", f.ID))
		if err != nil {
			return err
		}
		f.Severity = sev
	}
	return nil
}

// Grade evaluates findings against c and returns a list of failure
// strings; an empty slice means the rubric passed. It mirrors grade.py's
// evaluate: one failure per finding in a forbidden severity, plus one
// failure if no finding satisfies the required-finding floor (and file).
func Grade(fs []Finding, c Constraints) []string {
	var failures []string
	failures = append(failures, forbidFailures(fs, c.ForbidSeverities)...)
	if f := requireFailure(fs, c); f != "" {
		failures = append(failures, f)
	}
	return failures
}

// forbidFailures returns one failure per finding whose severity is in the
// forbidden set.
func forbidFailures(fs []Finding, forbid []string) []string {
	if len(forbid) == 0 {
		return nil
	}
	banned := make(map[string]bool, len(forbid))
	for _, s := range forbid {
		banned[s] = true
	}
	var out []string
	for i := range fs {
		f := &fs[i]
		if banned[strings.ToLower(f.Severity)] {
			out = append(out, fmt.Sprintf("forbidden severity %q in finding %s (%q)",
				f.Severity, orDefault(f.ID, "?"), f.Title))
		}
	}
	return out
}

// requireFailure returns a failure string if the required-finding check is
// active and unmet, else "". A finding counts only if it ranks at or above
// the floor and, when a file is required, its PRIMARY location matches it.
func requireFailure(fs []Finding, c Constraints) string {
	if c.RequireMinSeverity == "" {
		return ""
	}
	floor := severityRank[c.RequireMinSeverity]
	for i := range fs {
		f := &fs[i]
		if severityRank[strings.ToLower(f.Severity)] < floor {
			continue
		}
		if c.RequireLocationFile != "" && primaryFile(f) != c.RequireLocationFile {
			continue
		}
		return ""
	}
	where := ""
	if c.RequireLocationFile != "" {
		where = " at " + c.RequireLocationFile
	}
	return fmt.Sprintf("no finding of severity >= %s%s (the regression must be caught here)",
		c.RequireMinSeverity, where)
}

// primaryFile returns the finding's PRIMARY location file (empty when it
// has no primary location). require_finding matches only this, never a
// related_locations entry.
func primaryFile(f *Finding) string {
	if f.Location == nil {
		return ""
	}
	return f.Location.File
}

// LoadReport reads and unmarshals a findings.json from path, returning a
// clean error (never a panic) on a missing file or bad JSON, then
// validates every finding's severity.
func LoadReport(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	if err := ValidateFindings(r.Findings); err != nil {
		return nil, err
	}
	return &r, nil
}
