package corpus

// CompareReports compares two build reports and returns a drift report.
func CompareReports(baseline BuildReport, candidate BuildReport) *DriftReport {
	taxonomy := make(map[Category]int)
	for category, count := range baseline.Taxonomy {
		taxonomy[category] = -count
	}
	for category, count := range candidate.Taxonomy {
		taxonomy[category] += count
	}

	return &DriftReport{
		BaselineVersion:  baseline.DatasetVersion,
		CandidateVersion: candidate.DatasetVersion,
		FilesKeptDelta:   candidate.FilesKept - baseline.FilesKept,
		TaxonomyDeltas:   taxonomy,
		MetricDeltas: MetricSummary{
			AvgWords: candidate.Metrics.AvgWords - baseline.Metrics.AvgWords,
			AvgChars: candidate.Metrics.AvgChars - baseline.Metrics.AvgChars,
		},
	}
}
