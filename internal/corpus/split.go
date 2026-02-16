package corpus

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"sort"
)

func assignSplits(records []collectedRecord, seed int64) {
	groups := make(map[Category][]*collectedRecord)
	for idx := range records {
		record := &records[idx]
		groups[record.Category] = append(groups[record.Category], record)
	}

	for _, group := range groups {
		sort.Slice(group, func(i int, j int) bool {
			left := splitHash(group[i].RecordID, seed)
			right := splitHash(group[j].RecordID, seed)
			if left == right {
				return group[i].RecordID < group[j].RecordID
			}
			return left < right
		})
		assignGroupSplits(group)
	}
}

func splitHash(recordID string, seed int64) uint64 {
	h := fnv.New64a()
	seedBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(seedBytes, uint64(seed))
	_, _ = h.Write(seedBytes)
	_, _ = h.Write([]byte(recordID))
	return h.Sum64()
}

func assignGroupSplits(group []*collectedRecord) {
	total := len(group)
	if total == 0 {
		return
	}

	trainCount := int(math.Floor(float64(total) * 0.8))
	devCount := int(math.Floor(float64(total) * 0.1))
	testCount := total - trainCount - devCount

	if total >= 3 {
		if devCount == 0 {
			devCount = 1
			trainCount--
		}
		if testCount == 0 {
			trainCount--
		}
	}

	for idx, record := range group {
		switch {
		case idx < trainCount:
			record.Split = SplitTrain
		case idx < trainCount+devCount:
			record.Split = SplitDev
		default:
			record.Split = SplitTest
		}
	}
}

func makeQASample(records []ManifestRecord, perCategory int, seed int64) []QASampleRecord {
	grouped := make(map[Category][]ManifestRecord)
	for _, record := range records {
		grouped[record.Category] = append(grouped[record.Category], record)
	}

	sample := make([]QASampleRecord, 0)
	categories := AllCategories()
	for _, category := range categories {
		group := grouped[category]
		if len(group) == 0 {
			continue
		}
		sort.Slice(group, func(i int, j int) bool {
			left := splitHash(group[i].RecordID, seed)
			right := splitHash(group[j].RecordID, seed)
			if left == right {
				return group[i].RecordID < group[j].RecordID
			}
			return left < right
		})
		limit := perCategory
		if len(group) < limit {
			limit = len(group)
		}
		for _, record := range group[:limit] {
			sample = append(sample, QASampleRecord{
				RecordID:          record.RecordID,
				PredictedCategory: record.Category,
				SourceName:        record.SourceName,
				Path:              record.Path,
			})
		}
	}

	sort.Slice(sample, func(i int, j int) bool {
		if sample[i].PredictedCategory == sample[j].PredictedCategory {
			return sample[i].RecordID < sample[j].RecordID
		}
		return sample[i].PredictedCategory < sample[j].PredictedCategory
	})

	return sample
}
