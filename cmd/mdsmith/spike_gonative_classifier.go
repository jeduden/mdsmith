//go:build spike_gonative_classifier

package main

import "github.com/jeduden/mdsmith/internal/rules/concisenessscoring/classifier"

func init() {
	// Force-link the embedded artifact so size deltas can be measured.
	_ = classifier.EmbeddedArtifactBytes()
}
