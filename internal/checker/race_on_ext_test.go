//go:build race

package checker_test

// raceEnabled is the build-tag sentinel for `-race` in this package's
// external test binary. See race_off_ext_test.go for why the external
// test package needs its own copy separate from the in-package
// sentinel in race_on_test.go.
const raceEnabled = true
