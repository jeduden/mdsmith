package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/corpus"
)

func runMeasure(args []string) error {
	fs := flag.NewFlagSet("measure", flag.ContinueOnError)
	corpusDir := fs.String("corpus", "", "dataset directory containing manifest.jsonl")
	outPath := fs.String("out", "", "path to write measure report json")
	if err := fs.Parse(args); err != nil {
		return usageError(err.Error())
	}
	if *corpusDir == "" || *outPath == "" {
		return usageError("measure requires -corpus and -out")
	}

	records, err := readManifest(filepath.Join(*corpusDir, "manifest.jsonl"))
	if err != nil {
		return err
	}
	report := measureRecords(*corpusDir, records)
	if err := corpus.WriteJSON(*outPath, report); err != nil {
		return err
	}

	fmt.Println(*outPath)
	return nil
}

func readManifest(path string) ([]corpus.Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer func() { _ = file.Close() }()

	records := make([]corpus.Record, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record corpus.Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse manifest row: %w", err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan manifest: %w", err)
	}
	return records, nil
}

func measureRecords(corpusPath string, records []corpus.Record) corpus.MeasureReport {
	totals := make(map[corpus.Category]struct {
		count int
		words int
		chars int
	})

	for _, record := range records {
		entry := totals[record.Category]
		entry.count++
		entry.words += record.Words
		entry.chars += record.Chars
		totals[record.Category] = entry
	}

	byCategory := make(map[corpus.Category]corpus.MeasureCategoryStats, len(totals))
	for category, entry := range totals {
		stats := corpus.MeasureCategoryStats{Count: entry.count}
		if entry.count > 0 {
			stats.AvgWords = float64(entry.words) / float64(entry.count)
			stats.AvgChars = float64(entry.chars) / float64(entry.count)
		}
		byCategory[category] = stats
	}

	return corpus.MeasureReport{
		CorpusPath: corpusPath,
		Total:      len(records),
		Categories: byCategory,
	}
}
