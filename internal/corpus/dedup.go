package corpus

// Dedup removes exact duplicates by normalized content hash while keeping first occurrence.
func Dedup(records []Record) []Record {
	seen := make(map[string]struct{}, len(records))
	kept := make([]Record, 0, len(records))
	for _, record := range records {
		hash := record.ContentSHA256
		if hash == "" {
			hash = sha256Hex(normalizeContent(record.RawContent))
		}
		if _, exists := seen[hash]; exists {
			continue
		}
		seen[hash] = struct{}{}
		record.ContentSHA256 = hash
		kept = append(kept, record)
	}
	return kept
}
