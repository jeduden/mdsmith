//go:build !race

package metrics

// raceEnabled is false in the default build; race_on_test.go flips it.
// Alloc gates skip under -race because the detector adds allocation
// bookkeeping that perturbs exact counts.
const raceEnabled = false
