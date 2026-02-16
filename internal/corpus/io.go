package corpus

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteManifest writes JSONL records to disk.
func WriteManifest(path string, records []ManifestRecord) error {
	if err := ensureParentDir(path); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create manifest: %w", err)
	}
	defer func() { _ = file.Close() }()

	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("encode manifest record: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush manifest: %w", err)
	}
	return nil
}

// ReadManifest reads JSONL records from disk.
func ReadManifest(path string) ([]ManifestRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer func() { _ = file.Close() }()

	records := make([]ManifestRecord, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record ManifestRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse manifest line: %w", err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan manifest: %w", err)
	}
	return records, nil
}

// WriteQASample writes stratified QA sample rows to JSONL.
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
			return fmt.Errorf("encode qa sample record: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush qa sample: %w", err)
	}
	return nil
}

// ReadQASample reads QA sample rows from JSONL.
func ReadQASample(path string) ([]QASampleRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open qa sample: %w", err)
	}
	defer func() { _ = file.Close() }()

	rows := make([]QASampleRecord, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row QASampleRecord
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("parse qa sample line: %w", err)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan qa sample: %w", err)
	}
	return rows, nil
}

// ReadQAAnnotationsCSV reads manual labels with format: record_id,actual_category.
func ReadQAAnnotationsCSV(path string) ([]QAAnnotation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open annotations csv: %w", err)
	}
	defer func() { _ = file.Close() }()

	r := csv.NewReader(file)
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read annotations csv: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("annotations csv is empty")
	}

	start := 0
	if len(records[0]) >= 2 && strings.EqualFold(strings.TrimSpace(records[0][0]), "record_id") {
		start = 1
	}
	annotations := make([]QAAnnotation, 0, len(records)-start)
	for _, row := range records[start:] {
		if len(row) < 2 {
			return nil, fmt.Errorf("annotation rows must have record_id,actual_category")
		}
		annotation := QAAnnotation{
			RecordID:       strings.TrimSpace(row[0]),
			ActualCategory: Category(strings.TrimSpace(row[1])),
		}
		if annotation.RecordID == "" || annotation.ActualCategory == "" {
			return nil, fmt.Errorf("annotation rows cannot contain empty values")
		}
		annotations = append(annotations, annotation)
	}
	return annotations, nil
}

// WriteJSON writes an indented JSON object to disk.
func WriteJSON(path string, value any) error {
	if err := ensureParentDir(path); err != nil {
		return err
	}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}

// ReadBuildReport loads a build report JSON file.
func ReadBuildReport(path string) (BuildReport, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return BuildReport{}, fmt.Errorf("read build report: %w", err)
	}
	var report BuildReport
	if err := json.Unmarshal(content, &report); err != nil {
		return BuildReport{}, fmt.Errorf("parse build report json: %w", err)
	}
	return report, nil
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	return nil
}
