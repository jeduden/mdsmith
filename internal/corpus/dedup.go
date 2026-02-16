package corpus

import (
	"fmt"
	"math"
	"sort"
)

func dropExactDuplicates(records []collectedRecord) ([]collectedRecord, int) {
	seen := make(map[string]bool, len(records))
	out := make([]collectedRecord, 0, len(records))
	dropped := 0
	for _, record := range records {
		if seen[record.ContentSHA256] {
			dropped++
			continue
		}
		seen[record.ContentSHA256] = true
		out = append(out, record)
	}
	return out, dropped
}

func dropNearDuplicates(records []collectedRecord, threshold float64) ([]collectedRecord, int) {
	// This is O(n²) over kept records. It is acceptable for the current
	// corpus sizes, but larger datasets should switch to an indexed
	// approach (for example MinHash/LSH) to reduce pairwise comparisons.
	out := make([]collectedRecord, 0, len(records))
	dropped := 0

	for _, candidate := range records {
		if nearDuplicateOfAny(candidate, out, threshold) {
			dropped++
			continue
		}
		out = append(out, candidate)
	}
	return out, dropped
}

func nearDuplicateOfAny(
	candidate collectedRecord,
	kept []collectedRecord,
	threshold float64,
) bool {
	for _, current := range kept {
		score := jaccard(candidate.tokenSet, current.tokenSet)
		if score >= threshold {
			return true
		}
	}
	return false
}

func jaccard(a map[string]struct{}, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	for token := range a {
		if _, ok := b[token]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func capReadmes(records []collectedRecord, maxShare float64) ([]collectedRecord, int) {
	if maxShare <= 0 || len(records) == 0 {
		return records, 0
	}
	if maxShare >= 1 {
		return records, 0
	}

	readmes := make([]collectedRecord, 0)
	nonReadmes := make([]collectedRecord, 0)
	for _, record := range records {
		if record.IsReadme {
			readmes = append(readmes, record)
			continue
		}
		nonReadmes = append(nonReadmes, record)
	}

	maxReadmes := int(math.Floor((maxShare * float64(len(nonReadmes))) / (1 - maxShare)))
	if maxReadmes < 1 {
		maxReadmes = 1
	}
	if len(readmes) <= maxReadmes {
		return records, 0
	}

	sort.Slice(readmes, func(i int, j int) bool {
		return readmes[i].RecordID < readmes[j].RecordID
	})
	keptReadmes := readmes[:maxReadmes]
	dropped := len(readmes) - len(keptReadmes)

	combined := make([]collectedRecord, 0, len(nonReadmes)+len(keptReadmes))
	combined = append(combined, nonReadmes...)
	combined = append(combined, keptReadmes...)
	sort.Slice(combined, func(i int, j int) bool {
		return combined[i].RecordID < combined[j].RecordID
	})

	return combined, dropped
}

func applyBalance(
	records []collectedRecord,
	ranges map[Category]BalanceRange,
) ([]collectedRecord, int, []string) {
	if len(ranges) == 0 || len(records) == 0 {
		return records, 0, nil
	}

	total := len(records)
	groups := groupRecordsByCategory(records)
	keptByID := make(map[string]bool, len(records))
	dropped := 0

	for _, record := range records {
		keptByID[record.RecordID] = true
	}

	for category, rng := range ranges {
		group := groups[category]
		if len(group) == 0 {
			continue
		}
		if rng.Max <= 0 {
			continue
		}
		maxCount := int(math.Ceil(rng.Max * float64(total)))
		if maxCount < 1 {
			maxCount = 1
		}
		if len(group) <= maxCount {
			continue
		}

		sort.Slice(group, func(i int, j int) bool {
			return group[i].RecordID < group[j].RecordID
		})
		for _, overflow := range group[maxCount:] {
			if keptByID[overflow.RecordID] {
				keptByID[overflow.RecordID] = false
				dropped++
			}
		}
	}

	filtered := make([]collectedRecord, 0, len(records)-dropped)
	for _, record := range records {
		if keptByID[record.RecordID] {
			filtered = append(filtered, record)
		}
	}

	violations := checkBalanceViolations(filtered, ranges)
	return filtered, dropped, violations
}

func groupRecordsByCategory(records []collectedRecord) map[Category][]collectedRecord {
	groups := make(map[Category][]collectedRecord)
	for _, record := range records {
		groups[record.Category] = append(groups[record.Category], record)
	}
	return groups
}

func checkBalanceViolations(
	records []collectedRecord,
	ranges map[Category]BalanceRange,
) []string {
	if len(records) == 0 {
		return []string{"no records remain after balancing"}
	}

	counts := make(map[Category]int)
	for _, record := range records {
		counts[record.Category]++
	}

	violations := make([]string, 0)
	total := float64(len(records))
	for category, rng := range ranges {
		share := float64(counts[category]) / total
		if share < rng.Min || share > rng.Max {
			violations = append(
				violations,
				fmt.Sprintf("%s share %.4f outside [%.4f, %.4f]", category, share, rng.Min, rng.Max),
			)
		}
	}
	sort.Strings(violations)
	return violations
}
