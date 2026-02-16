package corpus

// Category is a taxonomy label used for each corpus record.
type Category string

// Taxonomy category constants.
const (
	CategoryAgentControl    Category = "agent-control"
	CategoryTutorial        Category = "tutorial"
	CategoryHowTo           Category = "how-to"
	CategoryReference       Category = "reference"
	CategoryExplanation     Category = "explanation"
	CategoryADR             Category = "adr"
	CategoryRFC             Category = "rfc"
	CategoryDesignProposal  Category = "design-proposal"
	CategoryRunbook         Category = "runbook"
	CategoryPostmortem      Category = "postmortem"
	CategoryChangelog       Category = "changelog"
	CategoryProjectDocs     Category = "project-docs"
	CategoryAPICLIConfig    Category = "api-cli-config"
	CategoryTroubleshooting Category = "troubleshooting"
)

// AllCategories returns the full taxonomy scope.
func AllCategories() []Category {
	return []Category{
		CategoryAgentControl,
		CategoryTutorial,
		CategoryHowTo,
		CategoryReference,
		CategoryExplanation,
		CategoryADR,
		CategoryRFC,
		CategoryDesignProposal,
		CategoryRunbook,
		CategoryPostmortem,
		CategoryChangelog,
		CategoryProjectDocs,
		CategoryAPICLIConfig,
		CategoryTroubleshooting,
	}
}

// Split label constants.
const (
	SplitTrain = "train"
	SplitDev   = "dev"
	SplitTest  = "test"
)

// BalanceRange defines acceptable category share bounds.
type BalanceRange struct {
	Min float64 `yaml:"min" json:"min"`
	Max float64 `yaml:"max" json:"max"`
}

// QualityPolicy defines repository-level source gate thresholds.
type QualityPolicy struct {
	MinStars            int  `yaml:"min_stars" json:"min_stars"`
	MinRecentCommits90D int  `yaml:"min_recent_commits_90d" json:"min_recent_commits_90d"`
	RequireCI           bool `yaml:"require_ci" json:"require_ci"`
}

// SourceQuality stores repository quality metadata for a source.
type SourceQuality struct {
	Stars            int  `yaml:"stars" json:"stars"`
	RecentCommits90D int  `yaml:"recent_commits_90d" json:"recent_commits_90d"`
	Archived         bool `yaml:"archived" json:"archived"`
	HasCI            bool `yaml:"has_ci" json:"has_ci"`
}

// SourceConfig defines one repository source to collect from.
type SourceConfig struct {
	Name          string        `yaml:"name" json:"name"`
	Repository    string        `yaml:"repository" json:"repository"`
	RepositoryURL string        `yaml:"repository_url" json:"repository_url"`
	Root          string        `yaml:"root" json:"root"`
	CommitSHA     string        `yaml:"commit_sha" json:"commit_sha"`
	License       string        `yaml:"license" json:"license"`
	Include       []string      `yaml:"include" json:"include"`
	Exclude       []string      `yaml:"exclude" json:"exclude"`
	Quality       SourceQuality `yaml:"quality" json:"quality"`
}

// BuildConfig controls the corpus build pipeline.
type BuildConfig struct {
	DatasetVersion         string                    `yaml:"dataset_version" json:"dataset_version"`
	CollectedAt            string                    `yaml:"collected_at" json:"collected_at"`
	Seed                   int64                     `yaml:"seed" json:"seed"`
	MinWords               int                       `yaml:"min_words" json:"min_words"`
	MinChars               int                       `yaml:"min_chars" json:"min_chars"`
	NearDuplicateThreshold float64                   `yaml:"near_duplicate_threshold" json:"near_duplicate_threshold"`
	MaxReadmeShare         float64                   `yaml:"max_readme_share" json:"max_readme_share"`
	QASamplePerCategory    int                       `yaml:"qa_sample_per_category" json:"qa_sample_per_category"`
	LicenseAllowlist       []string                  `yaml:"license_allowlist" json:"license_allowlist"`
	Policy                 QualityPolicy             `yaml:"policy" json:"policy"`
	Balance                map[Category]BalanceRange `yaml:"balance" json:"balance"`
	Sources                []SourceConfig            `yaml:"sources" json:"sources"`
}

// ManifestRecord is one normalized row in the frozen corpus manifest.
type ManifestRecord struct {
	RecordID      string   `json:"record_id"`
	Category      Category `json:"category"`
	Split         string   `json:"split"`
	SourceName    string   `json:"source_name"`
	Repository    string   `json:"repository"`
	RepositoryURL string   `json:"repository_url,omitempty"`
	Path          string   `json:"path"`
	CommitSHA     string   `json:"commit_sha"`
	License       string   `json:"license"`
	CollectedAt   string   `json:"collected_at"`
	WordCount     int      `json:"word_count"`
	CharCount     int      `json:"char_count"`
	ContentSHA256 string   `json:"content_sha256"`
	IsReadme      bool     `json:"is_readme"`
}

// QASampleRecord is a stratified manual QA sample row.
type QASampleRecord struct {
	RecordID          string   `json:"record_id"`
	PredictedCategory Category `json:"predicted_category"`
	SourceName        string   `json:"source_name"`
	Path              string   `json:"path"`
}

// BuildReport captures build-level quality and balance statistics.
type BuildReport struct {
	DatasetVersion     string                    `json:"dataset_version"`
	CollectedAt        string                    `json:"collected_at"`
	SourcesConsidered  int                       `json:"sources_considered"`
	SourcesIncluded    int                       `json:"sources_included"`
	FilesScanned       int                       `json:"files_scanned"`
	FilesKept          int                       `json:"files_kept"`
	FilteredByPolicy   int                       `json:"filtered_by_policy"`
	FilteredGenerated  int                       `json:"filtered_generated"`
	FilteredLowSignal  int                       `json:"filtered_low_signal"`
	DroppedExactDupes  int                       `json:"dropped_exact_duplicates"`
	DroppedNearDupes   int                       `json:"dropped_near_duplicates"`
	DroppedReadmes     int                       `json:"dropped_readmes"`
	DroppedByBalancing int                       `json:"dropped_by_balancing"`
	CategoryCounts     map[Category]int          `json:"category_counts"`
	SplitCounts        map[string]int            `json:"split_counts"`
	BalanceRanges      map[Category]BalanceRange `json:"balance_ranges"`
	BalanceViolations  []string                  `json:"balance_violations,omitempty"`
	ReadmeShare        float64                   `json:"readme_share"`
}

// BuildOutput is the full output of the build pipeline.
type BuildOutput struct {
	Manifest []ManifestRecord
	QASample []QASampleRecord
	Report   BuildReport
}

// QAAnnotation captures one human-annotated label decision.
type QAAnnotation struct {
	RecordID       string
	ActualCategory Category
}

// QACategoryMetrics provides per-class precision and recall.
type QACategoryMetrics struct {
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	Support   int     `json:"support"`
}

// QAReport summarizes manual QA agreement and label quality.
type QAReport struct {
	Total          int                            `json:"total"`
	Agreement      float64                        `json:"agreement"`
	PerCategory    map[Category]QACategoryMetrics `json:"per_category"`
	ConfusionCases []string                       `json:"confusion_cases,omitempty"`
}

// DriftCategoryDelta describes count and share shifts per category.
type DriftCategoryDelta struct {
	BaselineCount  int     `json:"baseline_count"`
	CandidateCount int     `json:"candidate_count"`
	DeltaCount     int     `json:"delta_count"`
	BaselineShare  float64 `json:"baseline_share"`
	CandidateShare float64 `json:"candidate_share"`
	DeltaShare     float64 `json:"delta_share"`
}

// DriftReport summarizes corpus drift between two reports.
type DriftReport struct {
	BaselineVersion  string                          `json:"baseline_version"`
	CandidateVersion string                          `json:"candidate_version"`
	BaselineTotal    int                             `json:"baseline_total"`
	CandidateTotal   int                             `json:"candidate_total"`
	DeltaTotal       int                             `json:"delta_total"`
	ReadmeShareDelta float64                         `json:"readme_share_delta"`
	ByCategory       map[Category]DriftCategoryDelta `json:"by_category"`
}
