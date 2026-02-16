package corpus

// CompareReports computes category and size drift between two build reports.
func CompareReports(baseline BuildReport, candidate BuildReport) DriftReport {
	baseTotal := baseline.FilesKept
	candTotal := candidate.FilesKept

	allCategories := make(map[Category]bool)
	for category := range baseline.CategoryCounts {
		allCategories[category] = true
	}
	for category := range candidate.CategoryCounts {
		allCategories[category] = true
	}

	byCategory := make(map[Category]DriftCategoryDelta, len(allCategories))
	for category := range allCategories {
		baseCount := baseline.CategoryCounts[category]
		candCount := candidate.CategoryCounts[category]
		baseShare := share(baseCount, baseTotal)
		candShare := share(candCount, candTotal)
		byCategory[category] = DriftCategoryDelta{
			BaselineCount:  baseCount,
			CandidateCount: candCount,
			DeltaCount:     candCount - baseCount,
			BaselineShare:  baseShare,
			CandidateShare: candShare,
			DeltaShare:     candShare - baseShare,
		}
	}

	return DriftReport{
		BaselineVersion:  baseline.DatasetVersion,
		CandidateVersion: candidate.DatasetVersion,
		BaselineTotal:    baseTotal,
		CandidateTotal:   candTotal,
		DeltaTotal:       candTotal - baseTotal,
		ReadmeShareDelta: candidate.ReadmeShare - baseline.ReadmeShare,
		ByCategory:       byCategory,
	}
}

func share(count int, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) / float64(total)
}
