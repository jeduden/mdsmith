package corpus

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type qaCounts struct {
	tp int
	fp int
	fn int
}

type qaEvaluation struct {
	categoryCounts  map[Category]*qaCounts
	actualCounts    map[Category]int
	predictedCounts map[Category]int
	annotated       int
	matches         int
}

// EvaluateQA compares predicted labels with manual annotations.
func EvaluateQA(sample []QASampleRecord, annotations []QAAnnotation) (*QAReport, error) {
	if len(sample) == 0 {
		return nil, fmt.Errorf("qa sample is empty")
	}
	if len(annotations) == 0 {
		return nil, fmt.Errorf("qa annotations are empty")
	}

	predictedByID := indexPredictions(sample)
	eval, err := scoreAnnotations(predictedByID, annotations)
	if err != nil {
		return nil, err
	}
	if eval.annotated == 0 {
		return nil, fmt.Errorf("no overlapping record ids between sample and annotations")
	}

	report := &QAReport{
		Total:      len(sample),
		Annotated:  eval.annotated,
		Accuracy:   float64(eval.matches) / float64(eval.annotated),
		Categories: make(map[Category]QACategoryMetrics, len(eval.categoryCounts)),
	}

	categories := make([]Category, 0, len(eval.categoryCounts))
	for category := range eval.categoryCounts {
		categories = append(categories, category)
	}
	sort.Slice(categories, func(i int, j int) bool { return categories[i] < categories[j] })

	for _, category := range categories {
		entry := eval.categoryCounts[category]
		precision := ratio(entry.tp, entry.tp+entry.fp)
		recall := ratio(entry.tp, entry.tp+entry.fn)
		f1 := 0.0
		if precision+recall > 0 {
			f1 = 2 * precision * recall / (precision + recall)
		}
		report.Categories[category] = QACategoryMetrics{
			Precision: precision,
			Recall:    recall,
			F1:        f1,
			Support:   eval.actualCounts[category],
		}
	}

	if kappa, ok := cohensKappa(
		eval.annotated,
		report.Accuracy,
		eval.predictedCounts,
		eval.actualCounts,
	); ok {
		report.Kappa = &kappa
	}

	return report, nil
}

func indexPredictions(sample []QASampleRecord) map[string]Category {
	predictedByID := make(map[string]Category, len(sample))
	for _, record := range sample {
		if record.RecordID != "" {
			predictedByID[record.RecordID] = record.PredictedCategory
		}
	}
	return predictedByID
}

func scoreAnnotations(
	predictedByID map[string]Category,
	annotations []QAAnnotation,
) (*qaEvaluation, error) {
	eval := &qaEvaluation{
		categoryCounts:  map[Category]*qaCounts{},
		actualCounts:    map[Category]int{},
		predictedCounts: map[Category]int{},
	}

	for _, annotation := range annotations {
		predicted, ok := predictedByID[annotation.RecordID]
		if !ok {
			continue
		}
		actual := annotation.ActualCategory
		if !isAllowedCategory(actual) {
			return nil, fmt.Errorf(
				"annotation for record %s has unknown category %q",
				annotation.RecordID,
				actual,
			)
		}
		if !isAllowedCategory(predicted) {
			return nil, fmt.Errorf(
				"sample for record %s has unknown category %q",
				annotation.RecordID,
				predicted,
			)
		}

		eval.annotated++
		eval.actualCounts[actual]++
		eval.predictedCounts[predicted]++
		ensureMetricEntry(eval.categoryCounts, actual)
		ensureMetricEntry(eval.categoryCounts, predicted)

		if predicted == actual {
			eval.matches++
			eval.categoryCounts[actual].tp++
		} else {
			eval.categoryCounts[predicted].fp++
			eval.categoryCounts[actual].fn++
		}
	}
	return eval, nil
}

// ReadQASample reads qa-sample JSONL records.
func ReadQASample(path string) ([]QASampleRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open qa sample: %w", err)
	}
	defer func() { _ = file.Close() }()

	sample := make([]QASampleRecord, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row QASampleRecord
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("parse qa sample row: %w", err)
		}
		sample = append(sample, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan qa sample: %w", err)
	}
	return sample, nil
}

// WriteQASample writes qa-sample JSONL records.
func WriteQASample(path string, records []QASampleRecord) error {
	if err := ensureParentDir(path); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create qa sample: %w", err)
	}
	defer func() { _ = file.Close() }()

	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("encode qa sample row: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush qa sample: %w", err)
	}
	return nil
}

// ReadQAAnnotationsCSV reads CSV annotations in the form record_id,actual_category.
func ReadQAAnnotationsCSV(path string) ([]QAAnnotation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open qa annotations: %w", err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read qa annotations: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("qa annotations csv is empty")
	}

	start := 0
	if len(rows[0]) >= 2 &&
		strings.EqualFold(strings.TrimSpace(rows[0][0]), "record_id") &&
		strings.EqualFold(strings.TrimSpace(rows[0][1]), "actual_category") {
		start = 1
	}

	annotations := make([]QAAnnotation, 0, len(rows)-start)
	for i, row := range rows[start:] {
		line := i + start + 1
		if len(row) < 2 {
			return nil, fmt.Errorf(
				"annotation row %d must contain record_id,actual_category",
				line,
			)
		}
		recordID := strings.TrimSpace(row[0])
		actual := Category(strings.TrimSpace(row[1]))
		if recordID == "" {
			return nil, fmt.Errorf("annotation row %d has empty record_id", line)
		}
		if !isAllowedCategory(actual) {
			return nil, fmt.Errorf("annotation row %d has unknown category %q", line, actual)
		}
		annotations = append(annotations, QAAnnotation{
			RecordID:       recordID,
			ActualCategory: actual,
		})
	}
	return annotations, nil
}

func ratio(numerator int, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func ensureMetricEntry(metrics map[Category]*qaCounts, category Category) {
	if _, ok := metrics[category]; ok {
		return
	}
	metrics[category] = &qaCounts{}
}

func isAllowedCategory(category Category) bool {
	return category == CategoryReference || category == CategoryOther
}

func cohensKappa(
	annotated int,
	observed float64,
	predictedCounts map[Category]int,
	actualCounts map[Category]int,
) (float64, bool) {
	if annotated <= 0 {
		return 0, false
	}

	expected := 0.0
	for _, category := range []Category{CategoryReference, CategoryOther} {
		predictedShare := float64(predictedCounts[category]) / float64(annotated)
		actualShare := float64(actualCounts[category]) / float64(annotated)
		expected += predictedShare * actualShare
	}
	if expected >= 1 {
		return 0, false
	}
	return (observed - expected) / (1 - expected), true
}
