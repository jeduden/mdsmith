package corpus

import (
	"fmt"
	"sort"
)

type qaCounts struct {
	tp int
	fp int
	fn int
	n  int
}

// EvaluateQA computes agreement, precision, and recall on manual labels.
func EvaluateQA(sample []QASampleRecord, annotations []QAAnnotation) (QAReport, error) {
	if len(sample) == 0 {
		return QAReport{}, fmt.Errorf("qa sample is empty")
	}
	if len(annotations) == 0 {
		return QAReport{}, fmt.Errorf("qa annotations are empty")
	}

	predByID := indexPredicted(sample)
	counts, matches, total, confusions := scoreAnnotations(predByID, annotations)
	if total == 0 {
		return QAReport{}, fmt.Errorf("no overlapping record IDs between sample and annotations")
	}
	sort.Strings(confusions)

	return QAReport{
		Total:          total,
		Agreement:      ratio(matches, total),
		PerCategory:    buildMetrics(counts),
		ConfusionCases: confusions,
	}, nil
}

func indexPredicted(sample []QASampleRecord) map[string]Category {
	predByID := make(map[string]Category, len(sample))
	for _, row := range sample {
		predByID[row.RecordID] = row.PredictedCategory
	}
	return predByID
}

func scoreAnnotations(
	predByID map[string]Category,
	annotations []QAAnnotation,
) (map[Category]*qaCounts, int, int, []string) {
	counts := make(map[Category]*qaCounts)
	matches := 0
	total := 0
	confusions := make([]string, 0)

	for _, row := range annotations {
		predicted, ok := predByID[row.RecordID]
		if !ok {
			continue
		}
		total++
		actual := row.ActualCategory
		ensureCategoryCount(counts, actual)
		ensureCategoryCount(counts, predicted)
		counts[actual].n++
		if predicted == actual {
			counts[actual].tp++
			matches++
			continue
		}
		counts[predicted].fp++
		counts[actual].fn++
		confusions = append(confusions, fmt.Sprintf("%s predicted=%s actual=%s", row.RecordID, predicted, actual))
	}

	return counts, matches, total, confusions
}

func ensureCategoryCount(counts map[Category]*qaCounts, category Category) {
	if _, ok := counts[category]; ok {
		return
	}
	counts[category] = &qaCounts{}
}

func buildMetrics(counts map[Category]*qaCounts) map[Category]QACategoryMetrics {
	metrics := make(map[Category]QACategoryMetrics, len(counts))
	for category, count := range counts {
		metrics[category] = QACategoryMetrics{
			Precision: ratio(count.tp, count.tp+count.fp),
			Recall:    ratio(count.tp, count.tp+count.fn),
			Support:   count.n,
		}
	}
	return metrics
}

func ratio(num int, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}
