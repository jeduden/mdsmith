package corpus

import "time"

// Category is the taxonomy label for a corpus record.
type Category string

// Category constants for corpus taxonomy labels.
const (
	CategoryReference Category = "reference"
	CategoryOther     Category = "other"
)

// Split constants for deterministic train/test partition labels.
const (
	SplitTrain = "train"
	SplitTest  = "test"
)

// Config defines corpus collection settings.
type Config struct {
	DatasetVersion    string         `yaml:"dataset_version" json:"dataset_version"`
	CollectedAt       string         `yaml:"collected_at" json:"collected_at"`
	MinWords          int            `yaml:"min_words" json:"min_words"`
	MinChars          int            `yaml:"min_chars" json:"min_chars"`
	TestFraction      float64        `yaml:"test_fraction" json:"test_fraction"`
	LicenseAllowlist  []string       `yaml:"license_allowlist" json:"license_allowlist"`
	QASampleLimit     int            `yaml:"qa_sample_limit" json:"qa_sample_limit"`
	Sources           []SourceConfig `yaml:"sources" json:"sources"`
	ResolvedFromLocal bool           `yaml:"-" json:"-"`
}

// SourceConfig defines one configured source repository.
type SourceConfig struct {
	Name        string            `yaml:"name" json:"name"`
	Repository  string            `yaml:"repository" json:"repository"`
	Root        string            `yaml:"root" json:"root"`
	CommitSHA   string            `yaml:"commit_sha" json:"commit_sha"`
	License     string            `yaml:"license" json:"license"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

// Record contains one collected markdown record.
type Record struct {
	RecordID       string    `json:"record_id"`
	Source         string    `json:"source"`
	Repository     string    `json:"repository,omitempty"`
	CommitSHA      string    `json:"commit_sha,omitempty"`
	License        string    `json:"license,omitempty"`
	Path           string    `json:"path"`
	Category       Category  `json:"category"`
	Split          string    `json:"split,omitempty"`
	Words          int       `json:"words"`
	Chars          int       `json:"chars"`
	ContentSHA256  string    `json:"content_sha256,omitempty"`
	CollectedAt    string    `json:"collected_at,omitempty"`
	SourceResolved string    `json:"-"`
	RawContent     string    `json:"-"`
	CollectedTime  time.Time `json:"-"`
}

// BuildResult contains build artifacts.
type BuildResult struct {
	Manifest []Record
	Report   BuildReport
	QASample []QASampleRecord
}

// SplitSummary holds train/test counts.
type SplitSummary struct {
	Train int `json:"train"`
	Test  int `json:"test"`
}

// MetricSummary provides build-level aggregate metrics.
type MetricSummary struct {
	AvgWords float64 `json:"avg_words"`
	AvgChars float64 `json:"avg_chars"`
}

// BuildReport summarizes corpus build output.
type BuildReport struct {
	DatasetVersion string           `json:"dataset_version"`
	CollectedAt    string           `json:"collected_at"`
	FilesCollected int              `json:"files_collected"`
	FilesKept      int              `json:"files_kept"`
	FilesDeduped   int              `json:"files_deduped"`
	Taxonomy       map[Category]int `json:"taxonomy"`
	Split          SplitSummary     `json:"split"`
	Metrics        MetricSummary    `json:"metrics"`
}

// QASampleRecord is a row in the annotation sample.
type QASampleRecord struct {
	RecordID          string   `json:"record_id"`
	PredictedCategory Category `json:"predicted_category"`
	Source            string   `json:"source"`
	Path              string   `json:"path"`
}

// QAAnnotation stores one manual annotation.
type QAAnnotation struct {
	RecordID       string   `json:"record_id"`
	ActualCategory Category `json:"actual_category"`
}

// QACategoryMetrics provides one-vs-rest metrics.
type QACategoryMetrics struct {
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
	Support   int     `json:"support"`
}

// QAReport summarizes annotation quality.
type QAReport struct {
	Total      int                            `json:"total"`
	Annotated  int                            `json:"annotated"`
	Accuracy   float64                        `json:"accuracy"`
	Kappa      *float64                       `json:"kappa,omitempty"`
	Categories map[Category]QACategoryMetrics `json:"categories"`
}

// DriftReport summarizes changes between two build reports.
type DriftReport struct {
	BaselineVersion  string           `json:"baseline_version"`
	CandidateVersion string           `json:"candidate_version"`
	FilesKeptDelta   int              `json:"files_kept_delta"`
	TaxonomyDeltas   map[Category]int `json:"taxonomy_deltas"`
	MetricDeltas     MetricSummary    `json:"metric_deltas"`
}

// MeasureCategoryStats holds measurement summary by category.
type MeasureCategoryStats struct {
	Count    int     `json:"count"`
	AvgWords float64 `json:"avg_words"`
	AvgChars float64 `json:"avg_chars"`
}

// MeasureReport is written by the measure subcommand.
type MeasureReport struct {
	CorpusPath string                            `json:"corpus_path"`
	Total      int                               `json:"total"`
	Categories map[Category]MeasureCategoryStats `json:"categories"`
}
