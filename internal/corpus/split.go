package corpus

import (
	"crypto/sha256"
	"encoding/binary"
)

const defaultTestSplitFraction = 0.2

// Split deterministically partitions records into train and test sets.
func Split(records []Record, testFraction float64) (train []Record, test []Record) {
	fraction := testFraction
	if fraction <= 0 || fraction >= 1 {
		fraction = defaultTestSplitFraction
	}

	threshold := uint64(float64(^uint64(0)) * fraction)
	train = make([]Record, 0, len(records))
	test = make([]Record, 0, len(records))

	for _, record := range records {
		key := record.RecordID
		if key == "" {
			key = record.Source + "|" + record.Path + "|" + record.ContentSHA256
		}
		hash := stableUint64(key)
		if hash <= threshold {
			record.Split = SplitTest
			test = append(test, record)
			continue
		}
		record.Split = SplitTrain
		train = append(train, record)
	}
	return train, test
}

func stableUint64(input string) uint64 {
	sum := sha256.Sum256([]byte(input))
	return binary.BigEndian.Uint64(sum[:8])
}
