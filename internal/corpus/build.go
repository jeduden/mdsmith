package corpus

import "sort"

// Build runs collect -> dedup -> classify -> split and produces reports.
func Build(cfg *Config, cacheDir string) (*BuildResult, error) {
	records, err := Collect(cfg, cacheDir)
	if err != nil {
		return nil, err
	}
	filesCollected := len(records)

	deduped := Dedup(records)
	filesDeduped := filesCollected - len(deduped)

	classified := Classify(deduped)
	train, test := Split(classified, cfg.TestFraction)

	manifest := make([]Record, 0, len(classified))
	manifest = append(manifest, train...)
	manifest = append(manifest, test...)
	sort.Slice(manifest, func(i int, j int) bool {
		return manifest[i].RecordID < manifest[j].RecordID
	})

	report := BuildReport{
		DatasetVersion: cfg.DatasetVersion,
		CollectedAt:    cfg.CollectedAt,
		FilesCollected: filesCollected,
		FilesKept:      len(manifest),
		FilesDeduped:   filesDeduped,
		Taxonomy:       map[Category]int{},
		Split: SplitSummary{
			Train: len(train),
			Test:  len(test),
		},
	}
	for _, record := range manifest {
		report.Taxonomy[record.Category]++
		report.Metrics.AvgWords += float64(record.Words)
		report.Metrics.AvgChars += float64(record.Chars)
	}
	if len(manifest) > 0 {
		report.Metrics.AvgWords /= float64(len(manifest))
		report.Metrics.AvgChars /= float64(len(manifest))
	}

	sample := makeQASample(manifest, cfg.QASampleLimit)

	return &BuildResult{
		Manifest: manifest,
		Report:   report,
		QASample: sample,
	}, nil
}

func makeQASample(records []Record, limit int) []QASampleRecord {
	if limit <= 0 {
		limit = defaultQASampleLimit
	}
	if len(records) == 0 {
		return nil
	}

	groups := map[Category][]Record{
		CategoryReference: {},
		CategoryOther:     {},
	}
	for _, record := range records {
		groups[record.Category] = append(groups[record.Category], record)
	}
	for category := range groups {
		group := groups[category]
		sort.Slice(group, func(i int, j int) bool {
			left := group[i].RecordID
			right := group[j].RecordID
			if left == right {
				return group[i].Path < group[j].Path
			}
			return left < right
		})
		groups[category] = group
	}

	picked := make([]QASampleRecord, 0, min(limit, len(records)))
	for len(picked) < limit {
		added := false
		for _, category := range []Category{CategoryReference, CategoryOther} {
			group := groups[category]
			if len(group) == 0 {
				continue
			}
			record := group[0]
			groups[category] = group[1:]
			picked = append(picked, QASampleRecord{
				RecordID:          record.RecordID,
				PredictedCategory: record.Category,
				Source:            record.Source,
				Path:              record.Path,
			})
			added = true
			if len(picked) >= limit {
				break
			}
		}
		if !added {
			break
		}
	}

	sort.Slice(picked, func(i int, j int) bool {
		if picked[i].PredictedCategory == picked[j].PredictedCategory {
			return picked[i].RecordID < picked[j].RecordID
		}
		return picked[i].PredictedCategory < picked[j].PredictedCategory
	})
	return picked
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
