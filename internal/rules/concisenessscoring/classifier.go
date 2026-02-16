package concisenessscoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

const (
	defaultMode                = "auto"
	defaultClassifierThreshold = 0.60
	defaultClassifierTimeoutMS = 25
)

var errInferenceTimeout = errors.New("classifier inference timed out")

var supportedFeatures = map[string]struct{}{
	"bias":              {},
	"filler_ratio":      {},
	"hedge_ratio":       {},
	"verbose_ratio":     {},
	"low_content_ratio": {},
	"cue_density":       {},
}

type artifactManifest struct {
	ModelID      string `json:"model_id"`
	Version      string `json:"version"`
	ArtifactPath string `json:"artifact_path"`
	SHA256       string `json:"sha256"`
}

type modelArtifact struct {
	ModelID   string             `json:"model_id"`
	Version   string             `json:"version"`
	Threshold float64            `json:"threshold"`
	Features  []string           `json:"features"`
	Weights   map[string]float64 `json:"weights"`
}

type classifier interface {
	ModelID() string
	Version() string
	DefaultThreshold() float64
	Predict(signals paragraphSignals) (float64, error)
}

type linearClassifier struct {
	modelID   string
	version   string
	threshold float64
	features  []string
	weights   map[string]float64
}

func (c *linearClassifier) ModelID() string {
	return c.modelID
}

func (c *linearClassifier) Version() string {
	return c.version
}

func (c *linearClassifier) DefaultThreshold() float64 {
	return c.threshold
}

func (c *linearClassifier) Predict(signals paragraphSignals) (float64, error) {
	values := map[string]float64{
		"bias":              1.0,
		"filler_ratio":      signals.FillerRatio,
		"hedge_ratio":       signals.HedgeRatio,
		"verbose_ratio":     signals.VerboseRatio,
		"low_content_ratio": 1 - signals.ContentRatio,
		"cue_density":       signals.CueDensity,
	}

	sum := 0.0
	for _, feature := range c.features {
		weight, ok := c.weights[feature]
		if !ok {
			continue
		}
		sum += weight * values[feature]
	}

	risk := 1 / (1 + math.Exp(-sum))
	if risk < 0 {
		risk = 0
	}
	if risk > 1 {
		risk = 1
	}
	return risk, nil
}

func predictWithTimeout(
	ctx context.Context, c classifier, signals paragraphSignals,
) (float64, error) {
	type result struct {
		risk float64
		err  error
	}

	ch := make(chan result, 1)
	go func() {
		risk, err := c.Predict(signals)
		ch <- result{risk: risk, err: err}
	}()

	select {
	case <-ctx.Done():
		return 0, errInferenceTimeout
	case out := <-ch:
		return out.risk, out.err
	}
}

func loadClassifier(modelPath, checksum string) (classifier, error) {
	if modelPath == "" {
		return loadEmbeddedClassifier()
	}
	return loadExternalClassifier(modelPath, checksum)
}

func loadEmbeddedClassifier() (classifier, error) {
	var manifest artifactManifest
	if err := json.Unmarshal(embeddedModelManifest, &manifest); err != nil {
		return nil, fmt.Errorf("parsing embedded manifest: %w", err)
	}

	sum := sha256Hex(embeddedModelArtifact)
	if !strings.EqualFold(sum, manifest.SHA256) {
		return nil, fmt.Errorf(
			"embedded model checksum mismatch: got %s want %s",
			sum, manifest.SHA256,
		)
	}

	return parseClassifierArtifact(embeddedModelArtifact)
}

func loadExternalClassifier(
	modelPath string, checksum string,
) (classifier, error) {
	data, err := os.ReadFile(modelPath)
	if err != nil {
		return nil, fmt.Errorf("reading classifier model %q: %w", modelPath, err)
	}

	sum := sha256Hex(data)
	if !strings.EqualFold(sum, checksum) {
		return nil, fmt.Errorf(
			"classifier checksum mismatch: got %s want %s",
			sum, checksum,
		)
	}

	return parseClassifierArtifact(data)
}

func parseClassifierArtifact(data []byte) (classifier, error) {
	var art modelArtifact
	if err := json.Unmarshal(data, &art); err != nil {
		return nil, fmt.Errorf("parsing classifier model: %w", err)
	}

	if strings.TrimSpace(art.ModelID) == "" {
		return nil, errors.New("classifier model_id must be non-empty")
	}
	if strings.TrimSpace(art.Version) == "" {
		return nil, errors.New("classifier version must be non-empty")
	}
	if len(art.Features) == 0 {
		return nil, errors.New("classifier features must be non-empty")
	}
	if len(art.Weights) == 0 {
		return nil, errors.New("classifier weights must be non-empty")
	}
	if err := validateFeatureWeights(art.Features, art.Weights); err != nil {
		return nil, err
	}
	if art.Threshold <= 0 || art.Threshold > 1 {
		return nil, fmt.Errorf(
			"classifier threshold must be > 0 and <= 1, got %.2f",
			art.Threshold,
		)
	}

	return &linearClassifier{
		modelID:   art.ModelID,
		version:   art.Version,
		threshold: art.Threshold,
		features:  append([]string(nil), art.Features...),
		weights:   cloneWeights(art.Weights),
	}, nil
}

func validateFeatureWeights(
	features []string, weights map[string]float64,
) error {
	declared := make(map[string]struct{}, len(features))

	for _, feature := range features {
		name := strings.TrimSpace(feature)
		if name == "" {
			return errors.New("classifier feature names must be non-empty")
		}
		if _, ok := supportedFeatures[name]; !ok {
			return fmt.Errorf("classifier feature %q is not supported", name)
		}
		if _, exists := declared[name]; exists {
			return fmt.Errorf("classifier feature %q is duplicated", name)
		}
		if _, ok := weights[name]; !ok {
			return fmt.Errorf(
				"classifier feature %q is missing a weight",
				name,
			)
		}
		declared[name] = struct{}{}
	}

	for name := range weights {
		if _, ok := supportedFeatures[name]; !ok {
			return fmt.Errorf("classifier weight %q is not supported", name)
		}
		if _, ok := declared[name]; !ok {
			return fmt.Errorf(
				"classifier weight %q is not declared in features",
				name,
			)
		}
	}

	return nil
}

func cloneWeights(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeMode(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func isValidMode(v string) bool {
	switch normalizeMode(v) {
	case "auto", "classifier", "heuristic":
		return true
	default:
		return false
	}
}

func normalizeChecksum(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func isHexChecksum(v string) bool {
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func classifierTimeout(timeoutMS int) time.Duration {
	if timeoutMS <= 0 {
		timeoutMS = defaultClassifierTimeoutMS
	}
	return time.Duration(timeoutMS) * time.Millisecond
}
