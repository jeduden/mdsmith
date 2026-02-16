package classifier

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Embedded artifact metadata used for checksum and loading validation.
const (
	EmbeddedArtifactPath   = "data/cue-linear-v1.json"
	EmbeddedArtifactSHA256 = "a17544b94507ad05e5d9db33078ca7a63d3fccd94b1d091ee1c85a88bbc81e44"
)

//go:embed data/cue-linear-v1.json
var embeddedArtifact []byte

var wordPattern = regexp.MustCompile(`[a-z0-9']+`)

var fillerWords = wordSet(
	"actually",
	"basically",
	"just",
	"really",
	"very",
)

var modalWords = wordSet(
	"may",
	"might",
	"could",
	"potentially",
	"possibly",
	"perhaps",
	"probably",
)

var vagueWords = wordSet(
	"many",
	"various",
	"several",
	"some",
	"often",
	"generally",
	"typically",
	"usually",
	"approach",
	"things",
)

var actionWords = wordSet(
	"run",
	"set",
	"retry",
	"validate",
	"check",
	"checks",
	"add",
	"update",
	"publish",
	"accepts",
	"use",
	"write",
	"build",
	"test",
	"deploy",
	"verify",
	"fix",
)

var stopWords = wordSet(
	"a",
	"an",
	"and",
	"are",
	"as",
	"at",
	"be",
	"before",
	"for",
	"from",
	"in",
	"is",
	"it",
	"of",
	"on",
	"or",
	"so",
	"that",
	"the",
	"their",
	"them",
	"this",
	"to",
	"under",
	"we",
	"with",
	"you",
	"your",
)

var hedgePhrases = []string{
	"i think",
	"it seems",
	"it appears",
	"kind of",
	"sort of",
	"in my opinion",
	"you might want to",
	"may potentially",
}

var verbosePhrases = []string{
	"in order to",
	"due to the fact that",
	"at this point in time",
	"for the purpose of",
	"it should be noted that",
}

type artifact struct {
	ModelID   string             `json:"model_id"`
	Version   string             `json:"version"`
	Threshold float64            `json:"threshold"`
	Intercept float64            `json:"intercept"`
	Weights   map[string]float64 `json:"weights"`
}

// Model is a deterministic linear classifier loaded from an embedded artifact.
type Model struct {
	artifact artifact
}

// Result is the classifier decision contract for one paragraph.
type Result struct {
	Label          string
	RiskScore      float64
	Threshold      float64
	ModelID        string
	Backend        string
	Version        string
	TriggeredCues  []string
	FeatureSummary map[string]float64
}

// EmbeddedArtifactBytes returns the embedded artifact size in bytes.
func EmbeddedArtifactBytes() int {
	return len(embeddedArtifact)
}

// LoadEmbedded loads and verifies the embedded model artifact.
func LoadEmbedded() (*Model, error) {
	sum := sha256.Sum256(embeddedArtifact)
	got := hex.EncodeToString(sum[:])
	if got != EmbeddedArtifactSHA256 {
		return nil, fmt.Errorf(
			"classifier: embedded artifact checksum mismatch: got %s want %s",
			got,
			EmbeddedArtifactSHA256,
		)
	}

	var a artifact
	if err := json.Unmarshal(embeddedArtifact, &a); err != nil {
		return nil, fmt.Errorf("classifier: decode embedded artifact: %w", err)
	}
	if err := validateArtifact(a); err != nil {
		return nil, err
	}

	return &Model{artifact: a}, nil
}

// ModelID returns the artifact model identifier.
func (m *Model) ModelID() string {
	return m.artifact.ModelID
}

// Version returns the artifact version identifier.
func (m *Model) Version() string {
	return m.artifact.Version
}

// Threshold returns the decision threshold.
func (m *Model) Threshold() float64 {
	return m.artifact.Threshold
}

// Classify computes a deterministic risk score and binary label.
func (m *Model) Classify(text string) Result {
	features, cues := extractFeatures(text)
	score := m.artifact.Intercept
	for name, value := range features {
		weight := m.artifact.Weights[name]
		score += weight * value
	}

	risk := sigmoid(score)
	label := "acceptable"
	if risk >= m.artifact.Threshold {
		label = "verbose-actionable"
	}

	return Result{
		Label:          label,
		RiskScore:      risk,
		Threshold:      m.artifact.Threshold,
		ModelID:        m.artifact.ModelID,
		Backend:        "classifier",
		Version:        m.artifact.Version,
		TriggeredCues:  cues,
		FeatureSummary: features,
	}
}

func validateArtifact(a artifact) error {
	if a.ModelID == "" {
		return fmt.Errorf("classifier: model_id must not be empty")
	}
	if a.Version == "" {
		return fmt.Errorf("classifier: version must not be empty")
	}
	if a.Threshold < 0 || a.Threshold > 1 {
		return fmt.Errorf(
			"classifier: threshold must be in [0,1], got %v",
			a.Threshold,
		)
	}
	if len(a.Weights) == 0 {
		return fmt.Errorf("classifier: weights must not be empty")
	}
	return nil
}

func extractFeatures(text string) (map[string]float64, []string) {
	lower := strings.ToLower(text)
	tokens := wordPattern.FindAllString(lower, -1)
	wordCount := float64(len(tokens))

	features := map[string]float64{
		"filler_rate":         0,
		"hedge_rate":          0,
		"verbose_phrase_rate": 0,
		"modal_rate":          0,
		"vague_rate":          0,
		"action_rate":         0,
		"content_ratio":       0,
		"log_word_count":      0,
	}
	if wordCount == 0 {
		return features, nil
	}

	fillerCount, fillerCues := countTokenMatches(tokens, fillerWords)
	modalCount, modalCues := countTokenMatches(tokens, modalWords)
	vagueCount, vagueCues := countTokenMatches(tokens, vagueWords)
	actionCount, actionCues := countTokenMatches(tokens, actionWords)
	contentCount := countContentTokens(tokens)

	hedgeCount, hedgeCues := countPhraseMatches(lower, hedgePhrases)
	verboseCount, verboseCues := countPhraseMatches(lower, verbosePhrases)

	features["filler_rate"] = float64(fillerCount) / wordCount
	features["hedge_rate"] = float64(hedgeCount) / wordCount
	features["verbose_phrase_rate"] = float64(verboseCount) / wordCount
	features["modal_rate"] = float64(modalCount) / wordCount
	features["vague_rate"] = float64(vagueCount) / wordCount
	features["action_rate"] = float64(actionCount) / wordCount
	features["content_ratio"] = float64(contentCount) / wordCount
	features["log_word_count"] = math.Log1p(wordCount)

	cues := make([]string, 0, 12)
	cues = append(cues, fillerCues...)
	cues = append(cues, modalCues...)
	cues = append(cues, vagueCues...)
	cues = append(cues, actionCues...)
	cues = append(cues, hedgeCues...)
	cues = append(cues, verboseCues...)

	return features, dedupeSorted(cues)
}

func countTokenMatches(
	tokens []string, set map[string]struct{},
) (int, []string) {
	count := 0
	cues := make([]string, 0, 4)
	seen := map[string]struct{}{}
	for _, tok := range tokens {
		if _, ok := set[tok]; !ok {
			continue
		}
		count++
		if _, exists := seen[tok]; exists {
			continue
		}
		seen[tok] = struct{}{}
		cues = append(cues, tok)
	}
	return count, cues
}

func countContentTokens(tokens []string) int {
	count := 0
	for _, tok := range tokens {
		if _, stop := stopWords[tok]; !stop {
			count++
		}
	}
	return count
}

func countPhraseMatches(text string, phrases []string) (int, []string) {
	count := 0
	cues := make([]string, 0, 4)
	for _, phrase := range phrases {
		n := strings.Count(text, phrase)
		if n == 0 {
			continue
		}
		count += n
		cues = append(cues, phrase)
	}
	return count, cues
}

func dedupeSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func wordSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		out[v] = struct{}{}
	}
	return out
}

func sigmoid(x float64) float64 {
	if x >= 0 {
		z := math.Exp(-x)
		return 1 / (1 + z)
	}
	z := math.Exp(x)
	return z / (1 + z)
}
