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

	predictedByID := indexPredictions(sample)
	if err := validateAnnotationsAgainstSample(predictedByID, annotations); err != nil {
		return nil, err
	}
	eval, err := scoreAnnotations(predictedByID, annotations)
	if err != nil {
		return nil, err
	}
	report := &QAReport{
		Total:      len(sample),
		Annotated:  eval.annotated,
		Coverage:   ratio(eval.annotated, len(sample)),
		Accuracy:   0,
		Categories: make(map[Category]QACategoryMetrics, len(eval.categoryCounts)),
	}
	if eval.annotated == 0 {
		return report, nil
	}

	report.Accuracy = float64(eval.matches) / float64(eval.annotated)

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

func validateAnnotationsAgainstSample(
	predictedByID map[string]Category,
	annotations []QAAnnotation,
) error {
	seenIDs := make(map[string]struct{}, len(annotations))
	extraIDs := make([]string, 0)

	for _, annotation := range annotations {
		recordID := annotation.RecordID
		if _, seen := seenIDs[recordID]; seen {
			return fmt.Errorf("duplicate annotation for record_id %q", recordID)
		}
		seenIDs[recordID] = struct{}{}

		if _, ok := predictedByID[recordID]; !ok {
			extraIDs = append(extraIDs, recordID)
		}
	}
	if len(extraIDs) == 0 {
		return nil
	}

	sort.Strings(extraIDs)
	preview := extraIDs
	if len(preview) > 5 {
		preview = preview[:5]
	}
	return fmt.Errorf(
		"annotations contain %d record ids not present in sample (first ids: %s)",
		len(extraIDs),
		strings.Join(preview, ", "),
	)
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
		return []QAAnnotation{}, nil
	}

	start := 0
	if len(rows[0]) >= 2 &&
		strings.EqualFold(strings.TrimSpace(rows[0][0]), "record_id") {
		// First column looks like a header; validate all column names.
		if !strings.EqualFold(strings.TrimSpace(rows[0][1]), "actual_category") {
			return nil, fmt.Errorf(
				"annotation csv header has unexpected second column %q, expected \"actual_category\"",
				strings.TrimSpace(rows[0][1]),
			)
		}
		start = 1
	}

	annotations := make([]QAAnnotation, 0, len(rows)-start)
	for i, row := range rows[start:] {
		line := i + start + 1
		if isBlankCSVRow(row) {
			continue
		}
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
		if actual == "" {
			continue
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

func isBlankCSVRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// WriteQAAnnotationTemplateCSV writes an annotation CSV aligned to a QA sample.
func WriteQAAnnotationTemplateCSV(
	path string,
	sample []QASampleRecord,
	existing []QAAnnotation,
) (QAAnnotationTemplateStats, error) {
	if len(sample) == 0 {
		return QAAnnotationTemplateStats{}, fmt.Errorf("qa sample is empty")
	}
	if err := ensureParentDir(path); err != nil {
		return QAAnnotationTemplateStats{}, err
	}

	file, err := os.Create(path)
	if err != nil {
		return QAAnnotationTemplateStats{}, fmt.Errorf("create qa annotation template: %w", err)
	}
	defer func() { _ = file.Close() }()

	existingByID, err := indexExistingAnnotations(existing)
	if err != nil {
		return QAAnnotationTemplateStats{}, err
	}

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"record_id", "actual_category"}); err != nil {
		return QAAnnotationTemplateStats{}, fmt.Errorf("write qa annotation template header: %w", err)
	}

	stats, err := writeAnnotationTemplateRows(writer, sample, existingByID)
	if err != nil {
		return QAAnnotationTemplateStats{}, err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return QAAnnotationTemplateStats{}, fmt.Errorf("flush qa annotation template: %w", err)
	}
	return stats, nil
}

func indexExistingAnnotations(existing []QAAnnotation) (map[string]Category, error) {
	existingByID := make(map[string]Category, len(existing))
	for _, row := range existing {
		if row.RecordID == "" {
			continue
		}
		if _, exists := existingByID[row.RecordID]; exists {
			return nil, fmt.Errorf(
				"duplicate existing annotation for record_id %q",
				row.RecordID,
			)
		}
		existingByID[row.RecordID] = row.ActualCategory
	}
	return existingByID, nil
}

func writeAnnotationTemplateRows(
	writer *csv.Writer,
	sample []QASampleRecord,
	existingByID map[string]Category,
) (QAAnnotationTemplateStats, error) {
	seenSampleIDs := make(map[string]struct{}, len(sample))
	preserved := 0
	for _, row := range sample {
		if row.RecordID == "" {
			return QAAnnotationTemplateStats{}, fmt.Errorf("qa sample contains empty record_id")
		}
		if _, exists := seenSampleIDs[row.RecordID]; exists {
			return QAAnnotationTemplateStats{}, fmt.Errorf(
				"qa sample contains duplicate record_id %q",
				row.RecordID,
			)
		}
		seenSampleIDs[row.RecordID] = struct{}{}

		actual := ""
		if value, ok := existingByID[row.RecordID]; ok {
			actual = string(value)
			preserved++
		}
		if err := writer.Write([]string{row.RecordID, actual}); err != nil {
			return QAAnnotationTemplateStats{}, fmt.Errorf(
				"write qa annotation template row: %w",
				err,
			)
		}
	}

	return QAAnnotationTemplateStats{
		Total:     len(sample),
		Preserved: preserved,
	}, nil
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
