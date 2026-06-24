package convention

// This file carries the curated forbidden-word, forbidden-phrase, and
// forbidden-opener lists for the built-in no-llm-tells convention.
//
// Source of truth: .claude/skills/docs-author/slop-patterns.md — its
// "Vocabulary tells", "Phrasal tells", and "Sentence openers"
// sections. The lists below are a hand-maintained subset of that
// catalog; the drift-checker integration test
// (internal/integration/nollmtells_drift_test.go) reads slop-patterns.md
// and fails CI if any entry here is no longer present in the catalog.
//
// When you edit either source, edit the other: the skill catalog is
// the human-facing reference, this file is the mechanical layer that
// `mdsmith check` enforces without a model in the loop.

// llmVocabulary returns the single-word LLM vocabulary tells from the
// "Vocabulary tells" section of slop-patterns.md. Each entry is a
// plain word matched as a substring by MDS056 (forbidden-text).
// Figurative tags in the catalog (e.g. "landscape (figurative)") are
// dropped here: MDS056 matches text, not sense.
func llmVocabulary() []string {
	return []string{
		"delve",
		"dive into",
		"dive deep",
		"deep dive",
		"tapestry",
		"realm",
		"testament",
		"vibrant",
		"pivotal",
		"robust",
		"seamless",
		"leverage",
		"unlock",
		"unleash",
		"embark",
		"foster",
		"showcase",
		"emphasize",
		"enhance",
		"highlight",
		"crucial",
		"essential",
		"comprehensive",
		"holistic",
		"multifaceted",
		"nuanced",
		"intricate",
		"paradigm",
		"ecosystem",
		"transformative",
		"profound",
		"paramount",
		"honest",
		"boast",
		"garner",
		"bolster",
		"myriad",
		"plethora",
		"endeavor",
		"spearhead",
		"revolutionize",
		"groundbreaking",
		"cutting-edge",
		"effortless",
		"supercharge",
	}
}

// llmPhrases returns the set-phrase LLM tells from the "Phrasal tells"
// section of slop-patterns.md. Entries with a placeholder X (e.g.
// "ever-evolving X") are dropped: MDS056 matches a literal substring
// and cannot express the placeholder.
func llmPhrases() []string {
	return []string{
		"it's important to note that",
		"it's worth mentioning that",
		"in today's fast-paced world",
		"in the digital age",
		"in the realm of",
		"in the world of",
		"at its core",
		"plays a crucial role",
		"stands as a testament to",
		"a deep dive into",
		"as we navigate",
		"harness the power of",
		"unlock the potential of",
		"embark on a journey",
		"navigating the complexities of",
	}
}

// llmVocabularyAndPhrases concatenates the vocabulary and phrase lists
// for MDS056's contains: setting, vocabulary first.
func llmVocabularyAndPhrases() []string {
	vocab := llmVocabulary()
	phrases := llmPhrases()
	out := make([]string, 0, len(vocab)+len(phrases))
	out = append(out, vocab...)
	out = append(out, phrases...)
	return out
}

// llmParagraphOpeners returns the banned sentence openers from the
// "Sentence openers" section of slop-patterns.md, for MDS055's
// starts: setting. Each entry includes the trailing comma so the
// prefix match anchors on the opener word, not a longer word that
// merely starts with the same letters.
func llmParagraphOpeners() []string {
	return []string{
		"Certainly,",
		"Moreover,",
		"Additionally,",
		"Furthermore,",
		"Indeed,",
		"Notably,",
		"Importantly,",
		"Crucially,",
		"Essentially,",
		"Ultimately,",
		"Fundamentally,",
		"Basically,",
		"In essence,",
		"In conclusion,",
		"To summarize,",
		"To sum up,",
	}
}

// toAnySlice converts a []string to []any so it can populate a
// convention RulePreset Settings map (which the config loader treats
// as []any after YAML decode).
func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
