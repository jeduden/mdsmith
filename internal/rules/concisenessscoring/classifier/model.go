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
	EmbeddedArtifactSHA256 = "98c9d8c6c43ad03b8ac4ff63ebcdcec4cdb4a17634dac9bd4f622a302f37d146"

	minFillerWords    = 12
	minModalWords     = 10
	minVagueWords     = 16
	minActionWords    = 20
	minStopWords      = 25
	minHedgePhrases   = 8
	minVerbosePhrases = 8
)

//go:embed data/cue-linear-v1.json
var embeddedArtifact []byte

var (
	wordPattern  = regexp.MustCompile(`[a-z0-9']+`)
	tokenPattern = regexp.MustCompile(`^[a-z0-9']+$`)
)

type artifact struct {
	ModelID   string             `json:"model_id"`
	Version   string             `json:"version"`
	Threshold float64            `json:"threshold"`
	Intercept float64            `json:"intercept"`
	Weights   map[string]float64 `json:"weights"`
	Lexicon   lexiconArtifact    `json:"lexicon"`
}

type lexiconArtifact struct {
	FillerWords    []string `json:"filler_words"`
	ModalWords     []string `json:"modal_words"`
	VagueWords     []string `json:"vague_words"`
	ActionWords    []string `json:"action_words"`
	StopWords      []string `json:"stop_words"`
	HedgePhrases   []string `json:"hedge_phrases"`
	VerbosePhrases []string `json:"verbose_phrases"`
}

type compiledLexicon struct {
	fillerWords    map[string]struct{}
	modalWords     map[string]struct{}
	vagueWords     map[string]struct{}
	actionWords    map[string]struct{}
	stopWords      map[string]struct{}
	hedgePhrases   []string
	verbosePhrases []string
}

// LexiconCounts captures loaded cue-list sizes for quality and drift checks.
type LexiconCounts struct {
	FillerWords    int
	ModalWords     int
	VagueWords     int
	ActionWords    int
	StopWords      int
	HedgePhrases   int
	VerbosePhrases int
}

// Model is a deterministic linear classifier loaded from an embedded artifact.
type Model struct {
	artifact artifact
	lexicon  compiledLexicon
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
	lex, err := compileLexicon(a.Lexicon)
	if err != nil {
		return nil, err
	}

	return &Model{artifact: a, lexicon: lex}, nil
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

// LexiconCounts returns the active cue-list sizes loaded from the artifact.
func (m *Model) LexiconCounts() LexiconCounts {
	return LexiconCounts{
		FillerWords:    len(m.lexicon.fillerWords),
		ModalWords:     len(m.lexicon.modalWords),
		VagueWords:     len(m.lexicon.vagueWords),
		ActionWords:    len(m.lexicon.actionWords),
		StopWords:      len(m.lexicon.stopWords),
		HedgePhrases:   len(m.lexicon.hedgePhrases),
		VerbosePhrases: len(m.lexicon.verbosePhrases),
	}
}

// Classify computes a deterministic risk score and binary label.
func (m *Model) Classify(text string) Result {
	features, cues := extractFeatures(text, m.lexicon)
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
	if err := validateWeights(a.Weights); err != nil {
		return err
	}
	if _, err := compileLexicon(a.Lexicon); err != nil {
		return err
	}
	return nil
}

func validateWeights(weights map[string]float64) error {
	required := []string{
		"filler_rate",
		"hedge_rate",
		"verbose_phrase_rate",
		"modal_rate",
		"vague_rate",
		"action_rate",
		"content_ratio",
		"log_word_count",
	}
	for _, key := range required {
		if _, ok := weights[key]; !ok {
			return fmt.Errorf("classifier: weights missing required key %q", key)
		}
	}
	for key := range weights {
		if !containsString(required, key) {
			return fmt.Errorf("classifier: weights include unknown key %q", key)
		}
	}
	return nil
}

func compileLexicon(raw lexiconArtifact) (compiledLexicon, error) {
	fillerWords, err := normalizeCueList(
		"filler_words", raw.FillerWords, minFillerWords, true,
	)
	if err != nil {
		return compiledLexicon{}, err
	}
	modalWords, err := normalizeCueList(
		"modal_words", raw.ModalWords, minModalWords, true,
	)
	if err != nil {
		return compiledLexicon{}, err
	}
	vagueWords, err := normalizeCueList(
		"vague_words", raw.VagueWords, minVagueWords, true,
	)
	if err != nil {
		return compiledLexicon{}, err
	}
	actionWords, err := normalizeCueList(
		"action_words", raw.ActionWords, minActionWords, true,
	)
	if err != nil {
		return compiledLexicon{}, err
	}
	stopWords, err := normalizeCueList(
		"stop_words", raw.StopWords, minStopWords, true,
	)
	if err != nil {
		return compiledLexicon{}, err
	}
	hedgePhrases, err := normalizeCueList(
		"hedge_phrases", raw.HedgePhrases, minHedgePhrases, false,
	)
	if err != nil {
		return compiledLexicon{}, err
	}
	verbosePhrases, err := normalizeCueList(
		"verbose_phrases", raw.VerbosePhrases, minVerbosePhrases, false,
	)
	if err != nil {
		return compiledLexicon{}, err
	}

	return compiledLexicon{
		fillerWords:    wordSetFromSlice(fillerWords),
		modalWords:     wordSetFromSlice(modalWords),
		vagueWords:     wordSetFromSlice(vagueWords),
		actionWords:    wordSetFromSlice(actionWords),
		stopWords:      wordSetFromSlice(stopWords),
		hedgePhrases:   hedgePhrases,
		verbosePhrases: verbosePhrases,
	}, nil
}

func normalizeCueList(
	name string,
	values []string,
	minimum int,
	tokenOnly bool,
) ([]string, error) {
	if len(values) < minimum {
		return nil, fmt.Errorf(
			"classifier: %s must contain at least %d entries, got %d",
			name, minimum, len(values),
		)
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		normalized := strings.ToLower(strings.TrimSpace(v))
		if tokenOnly {
			if !tokenPattern.MatchString(normalized) {
				return nil, fmt.Errorf(
					"classifier: %s contains invalid token %q",
					name, v,
				)
			}
		} else {
			normalized = strings.Join(strings.Fields(normalized), " ")
			if normalized == "" {
				return nil, fmt.Errorf(
					"classifier: %s contains empty phrase entry",
					name,
				)
			}
		}

		if _, ok := seen[normalized]; ok {
			return nil, fmt.Errorf(
				"classifier: %s contains duplicate entry %q",
				name, normalized,
			)
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	return out, nil
}

func extractFeatures(
	text string, lexicon compiledLexicon,
) (map[string]float64, []string) {
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

	fillerCount, fillerCues := countTokenMatches(tokens, lexicon.fillerWords)
	modalCount, modalCues := countTokenMatches(tokens, lexicon.modalWords)
	vagueCount, vagueCues := countTokenMatches(tokens, lexicon.vagueWords)
	actionCount, actionCues := countTokenMatches(tokens, lexicon.actionWords)
	contentCount := countContentTokens(tokens, lexicon.stopWords)

	hedgeCount, hedgeCues := countPhraseMatches(lower, lexicon.hedgePhrases)
	verboseCount, verboseCues := countPhraseMatches(
		lower,
		lexicon.verbosePhrases,
	)

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

func countContentTokens(tokens []string, stopWords map[string]struct{}) int {
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

func wordSetFromSlice(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		out[v] = struct{}{}
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func sigmoid(x float64) float64 {
	if x >= 0 {
		z := math.Exp(-x)
		return 1 / (1 + z)
	}
	z := math.Exp(x)
	return z / (1 + z)
}
