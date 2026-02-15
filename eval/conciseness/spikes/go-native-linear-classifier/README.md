# Go-Native Linear Classifier Spike

## Goal

Provide a fully embedded Go alternative to service-based and
native-library inference paths.

## Constraint

The runtime must not depend on external dynamic libraries or
`YZMA_LIB`-style filesystem setup.

## Candidate Approach

Use a pure-Go linear classifier with deterministic scoring:

- features: cue counts, phrase hits, and length/density metrics
- model: sparse logistic regression (or linear SVM-style score)
- output: calibrated risk score in `[0, 1]` and binary label

## Packaging Shape

- export trained weights to a compact deterministic artifact
- embed artifact with `go:embed`
- verify checksum at startup
- no runtime network access

## Open Questions

1. Should training export be JSON, gob, or generated Go code?
2. What feature set gives best precision at small model size?
3. What binary-size ceiling is acceptable for default builds?

## Next Step

Execute `plan/64_spike-go-native-linear-classifier.md` and compare
results directly with `spikes/yzma-embedded-weasel-detection`.
