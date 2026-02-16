package corpus

import (
	"sort"
)

// Build runs collection, filtering, balancing, and split assignment.
func Build(cfg BuildConfig) (BuildOutput, error) {
	collected, stats, err := collect(cfg)
	if err != nil {
		return BuildOutput{}, err
	}

	exactDeduped, exactDropped := dropExactDuplicates(collected)
	nearDeduped, nearDropped := dropNearDuplicates(exactDeduped, cfg.NearDuplicateThreshold)
	balanced, balanceDropped, _ := applyBalance(nearDeduped, cfg.Balance)
	readmeCapped, readmeDropped := capReadmes(balanced, cfg.MaxReadmeShare)
	violations := checkBalanceViolations(readmeCapped, cfg.Balance)
	assignSplits(readmeCapped, cfg.Seed)

	sort.Slice(readmeCapped, func(i int, j int) bool {
		return readmeCapped[i].RecordID < readmeCapped[j].RecordID
	})

	manifest := make([]ManifestRecord, 0, len(readmeCapped))
	for _, record := range readmeCapped {
		manifest = append(manifest, record.ManifestRecord)
	}

	report := makeBuildReport(
		cfg,
		stats,
		manifest,
		exactDropped,
		nearDropped,
		readmeDropped,
		balanceDropped,
		violations,
	)
	sample := makeQASample(manifest, cfg.QASamplePerCategory, cfg.Seed)

	return BuildOutput{
		Manifest: manifest,
		QASample: sample,
		Report:   report,
	}, nil
}

func makeBuildReport(
	cfg BuildConfig,
	stats collectionStats,
	manifest []ManifestRecord,
	exactDropped int,
	nearDropped int,
	readmeDropped int,
	balanceDropped int,
	violations []string,
) BuildReport {
	categoryCounts := make(map[Category]int)
	splitCounts := make(map[string]int)
	readmes := 0
	for _, record := range manifest {
		categoryCounts[record.Category]++
		splitCounts[record.Split]++
		if record.IsReadme {
			readmes++
		}
	}

	readmeShare := 0.0
	if len(manifest) > 0 {
		readmeShare = float64(readmes) / float64(len(manifest))
	}

	return BuildReport{
		DatasetVersion:     cfg.DatasetVersion,
		CollectedAt:        cfg.CollectedAt,
		SourcesConsidered:  len(cfg.Sources),
		SourcesIncluded:    stats.sourcesIncluded,
		FilesScanned:       stats.filesScanned,
		FilesKept:          len(manifest),
		FilteredByPolicy:   stats.filteredByPolicy,
		FilteredGenerated:  stats.filteredGenerated,
		FilteredLowSignal:  stats.filteredLowSignal,
		DroppedExactDupes:  exactDropped,
		DroppedNearDupes:   nearDropped,
		DroppedReadmes:     readmeDropped,
		DroppedByBalancing: balanceDropped,
		CategoryCounts:     categoryCounts,
		SplitCounts:        splitCounts,
		BalanceRanges:      cfg.Balance,
		BalanceViolations:  violations,
		ReadmeShare:        readmeShare,
	}
}
