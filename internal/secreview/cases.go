package secreview

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// baselineRefPattern matches a 40-character lowercase-hex git SHA, the
// required shape of cases.yaml's baseline_ref.
var baselineRefPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// validModes is the set of review modes a case may declare.
var validModes = map[string]bool{"audit": true, "pr": true}

// RequireFinding is the require_finding block of a case's grade rubric: a
// minimum severity and an optional sink file the finding must point at.
type RequireFinding struct {
	MinSeverity  string `yaml:"min_severity"`
	LocationFile string `yaml:"location_file"`
}

// GradeSpec is a case's machine-checkable grade rubric, the YAML form of
// Constraints.
type GradeSpec struct {
	ForbidSeverities []string        `yaml:"forbid_severities"`
	RequireFinding   *RequireFinding `yaml:"require_finding"`
}

// Expect holds the human-judged must / must_not assertion lists for a case.
type Expect struct {
	Must    []string `yaml:"must"`
	MustNot []string `yaml:"must_not"`
}

// Case is one eval case from cases.yaml.
type Case struct {
	ID     string    `yaml:"id"`
	Mode   string    `yaml:"mode"`
	Prompt string    `yaml:"prompt"`
	Setup  string    `yaml:"setup"`
	Expect Expect    `yaml:"expect"`
	Grade  GradeSpec `yaml:"grade"`
}

// Spec is the whole cases.yaml document: the calibration baseline, the
// cases, and a free-text grading note.
type Spec struct {
	BaselineRef string `yaml:"baseline_ref"`
	Cases       []Case `yaml:"cases"`
	GradingNote string `yaml:"grading_note"`
}

// LoadSpec reads and decodes cases.yaml from path with KnownFields(true),
// so any unknown or typo'd key anywhere in the document (for example
// forbid_severity instead of forbid_severities) is a hard error rather
// than a silently ignored, vacuous rubric.
func LoadSpec(path string) (*Spec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var spec Spec
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &spec, nil
}

// Validate checks the spec's structural invariants: a 40-hex baseline_ref,
// at least one case, unique ids, a known mode, non-empty prompt/setup and
// must/must_not lists, and a non-vacuous grade rubric per case.
func (s *Spec) Validate() error {
	if !baselineRefPattern.MatchString(s.BaselineRef) {
		return fmt.Errorf("baseline_ref must be a 40-hex SHA, got %q", s.BaselineRef)
	}
	if len(s.Cases) == 0 {
		return fmt.Errorf("cases.yaml has no cases")
	}
	seen := make(map[string]bool, len(s.Cases))
	for i := range s.Cases {
		if err := s.Cases[i].validate(seen); err != nil {
			return err
		}
	}
	return nil
}

// validate checks one case and records its id in seen for the
// duplicate-id check.
func (c *Case) validate(seen map[string]bool) error {
	if c.ID == "" {
		return fmt.Errorf("case has no id")
	}
	if seen[c.ID] {
		return fmt.Errorf("duplicate case id %q", c.ID)
	}
	seen[c.ID] = true
	if !validModes[c.Mode] {
		return fmt.Errorf("%s: bad mode %q", c.ID, c.Mode)
	}
	if c.Prompt == "" {
		return fmt.Errorf("%s: empty prompt", c.ID)
	}
	if c.Setup == "" {
		return fmt.Errorf("%s: empty setup", c.ID)
	}
	if len(c.Expect.Must) == 0 {
		return fmt.Errorf("%s: expect.must must be a non-empty list", c.ID)
	}
	if len(c.Expect.MustNot) == 0 {
		return fmt.Errorf("%s: expect.must_not must be a non-empty list", c.ID)
	}
	if _, err := ConstraintsForCase(*c); err != nil {
		return fmt.Errorf("%s: %w", c.ID, err)
	}
	return nil
}

// ConstraintsForCase translates a case's GradeSpec into validated
// Constraints, so an empty or typo-flattened grade block surfaces as a
// vacuous-rubric error.
func ConstraintsForCase(c Case) (Constraints, error) {
	reqMin, reqFile := "", ""
	if rf := c.Grade.RequireFinding; rf != nil {
		reqMin, reqFile = rf.MinSeverity, rf.LocationFile
	}
	return BuildConstraints(c.Grade.ForbidSeverities, reqMin, reqFile)
}
