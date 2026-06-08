package secreview

// SARIF 2.1.0 output structs. Field declaration order is load-bearing:
// encoding/json marshals struct fields in order, and the JSON key order
// here is chosen to mirror render_findings.py's build_sarif exactly.

// sarifDoc is the top-level SARIF document.
type sarifDoc struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

// sarifRun is one analysis run: a tool driver plus its results.
type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

// sarifTool wraps the driver metadata.
type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

// sarifDriver names the analysis tool and lists its rules.
type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

// sarifRule describes one unique finding id.
type sarifRule struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	ShortDescription     sarifText      `json:"shortDescription"`
	FullDescription      sarifText      `json:"fullDescription"`
	DefaultConfiguration sarifLevelCfg  `json:"defaultConfiguration"`
	Properties           sarifRuleProps `json:"properties"`
}

// sarifText is the {"text": ...} shape used for descriptions and messages.
type sarifText struct {
	Text string `json:"text"`
}

// sarifLevelCfg carries a rule's default SARIF level.
type sarifLevelCfg struct {
	Level string `json:"level"`
}

// sarifRuleProps holds a rule's security-severity score and tags.
type sarifRuleProps struct {
	SecuritySeverity string   `json:"security-severity"`
	Tags             []string `json:"tags"`
}

// sarifResult is one finding occurrence.
type sarifResult struct {
	RuleID     string           `json:"ruleId"`
	RuleIndex  int              `json:"ruleIndex"`
	Level      string           `json:"level"`
	Message    sarifText        `json:"message"`
	Locations  []sarifLocation  `json:"locations"`
	Properties sarifResultProps `json:"properties"`
}

// sarifLocation wraps one physical location.
type sarifLocation struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

// sarifPhysical is an artifact location plus an optional region.
type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           *sarifRegion  `json:"region,omitempty"`
}

// sarifArtifact names a file by URI.
type sarifArtifact struct {
	URI string `json:"uri"`
}

// sarifRegion is a line span within an artifact.
type sarifRegion struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
}

// sarifResultProps holds a result's confidence and severity.
type sarifResultProps struct {
	Confidence string `json:"confidence"`
	Severity   string `json:"severity"`
}

// buildSARIF converts a report to a SARIF document, mirroring
// render_findings.py's build_sarif. Findings are iterated in their
// original order (not sorted), one rule per unique id.
func buildSARIF(r *Report) sarifDoc {
	rules, ruleIndex := buildRules(r.Findings)
	results := make([]sarifResult, 0, len(r.Findings))
	for i := range r.Findings {
		results = append(results, buildResult(&r.Findings[i], ruleIndex, r.Target.Repo))
	}
	return sarifDoc{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "mdsmith-security-review",
				InformationURI: "https://github.com/jeduden/mdsmith",
				Rules:          rules,
			}},
			Results: results,
		}},
	}
}

// buildRules emits one rule per unique finding id (first occurrence wins)
// and returns the id->index map used to set each result's ruleIndex.
func buildRules(findings []Finding) ([]sarifRule, map[string]int) {
	rules := make([]sarifRule, 0, len(findings))
	ruleIndex := make(map[string]int, len(findings))
	for i := range findings {
		f := &findings[i]
		if _, seen := ruleIndex[f.ID]; seen {
			continue
		}
		ruleIndex[f.ID] = len(rules)
		rules = append(rules, sarifRule{
			ID:                   f.ID,
			Name:                 orDefault(f.Surface, "security"),
			ShortDescription:     sarifText{Text: orDefault(f.Title, f.ID)},
			FullDescription:      sarifText{Text: f.Description},
			DefaultConfiguration: sarifLevelCfg{Level: sarifLevel[f.Severity]},
			Properties:           ruleProps(f),
		})
	}
	return rules, ruleIndex
}

// ruleProps builds a rule's properties: the security-severity score and
// the tags list (always "security", plus the CWE when present).
func ruleProps(f *Finding) sarifRuleProps {
	tags := []string{"security"}
	if f.CWE != "" {
		tags = append(tags, f.CWE)
	}
	return sarifRuleProps{SecuritySeverity: securitySeverity[f.Severity], Tags: tags}
}

// buildResult builds one SARIF result for a finding. Locations skip any
// position with no file or no startLine; if none remain it falls back to a
// single location naming the target repo (or "unknown").
func buildResult(f *Finding, ruleIndex map[string]int, repo string) sarifResult {
	locs := physicalLocations(f)
	if len(locs) == 0 {
		locs = []sarifLocation{{PhysicalLocation: sarifPhysical{
			ArtifactLocation: sarifArtifact{URI: orDefault(repo, "unknown")},
		}}}
	}
	return sarifResult{
		RuleID:    f.ID,
		RuleIndex: ruleIndex[f.ID],
		Level:     sarifLevel[f.Severity],
		Message:   sarifText{Text: orDefault(f.Title, f.ID)},
		Locations: locs,
		Properties: sarifResultProps{
			Confidence: orDefault(f.Confidence, "unspecified"),
			Severity:   f.Severity,
		},
	}
}

// physicalLocations gathers the primary location followed by the related
// locations, dropping any with no file. A located finding that omits its
// startLine is still emitted (artifactLocation without a region), matching
// render_findings.py — only a missing file drops the location.
func physicalLocations(f *Finding) []sarifLocation {
	out := make([]sarifLocation, 0, 1+len(f.RelatedLocations))
	if loc := f.Location; loc != nil && loc.File != "" {
		out = append(out, physical(loc))
	}
	for i := range f.RelatedLocations {
		loc := &f.RelatedLocations[i]
		if loc.File == "" {
			continue
		}
		out = append(out, physical(loc))
	}
	return out
}

// physical builds a SARIF location for a position that has a file. The
// region is included only when startLine is set, so a file-only location
// emits just the artifactLocation.
func physical(loc *Location) sarifLocation {
	phys := sarifPhysical{ArtifactLocation: sarifArtifact{URI: loc.File}}
	if loc.StartLine != 0 {
		end := loc.EndLine
		if end == 0 {
			end = loc.StartLine
		}
		phys.Region = &sarifRegion{StartLine: loc.StartLine, EndLine: end}
	}
	return sarifLocation{PhysicalLocation: phys}
}

// orDefault returns s, or def when s is empty.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
