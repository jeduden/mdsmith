package corpus

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteManifest writes manifest records as JSONL.
func WriteManifest(path string, records []Record) error {
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
			return fmt.Errorf("encode manifest row: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush manifest: %w", err)
	}
	return nil
}

// WriteJSON writes an indented JSON document.
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

// ReadBuildReport reads a build report JSON document.
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
