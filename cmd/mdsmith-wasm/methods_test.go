package main

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// nativeOnlyMethods are exported *mdsmith.Session methods that
// deliberately have no JavaScript mirror. The batch operations
// (checkPaths, fixPaths) read and write many files on disk and return
// the engine's own Result types so the CLI keeps its discovery and
// output formatting; a WASM host has no disk and drives single files
// through check/fix instead (plan 219, documented as native-only in
// docs/background/concepts/engine-api.md). The parity check below
// subtracts these before comparing so adding a JS-mirrored Go method
// without exposing it in JS still fails, while a native-only addition
// is allowed once it is listed here.
var nativeOnlyMethods = map[string]bool{
	"checkPaths":          true,
	"checkSource":         true,
	"checkVersion":        true,
	"fixPaths":            true,
	"fixRule":             true,
	"invalidateWikilinks": true,
	"resolveFile":         true,
}

// TestSessionMethodSetMatchesGo asserts the JS session proxy's method
// set matches the JS-mirrored Go *mdsmith.Session method set
// name-for-name (modulo the Go→JS lower-camel convention and the
// native-only batch ops). The WASM bridge builds its proxy from
// sessionMethodNames(); this test ties that list to the actual Go
// methods via reflection, so adding a JS-mirrored Go method without
// exposing it in JS (or vice versa) fails the build's tests.
func TestSessionMethodSetMatchesGo(t *testing.T) {
	jsNames := sessionMethodNames()
	sort.Strings(jsNames)

	goNames := mirroredGoMethodNames()
	sort.Strings(goNames)

	if !reflect.DeepEqual(jsNames, goNames) {
		t.Fatalf("JS session methods %v != JS-mirrored Go Session methods %v "+
			"(native-only methods %v are excluded)", jsNames, goNames, nativeOnlyMethods)
	}
}

// TestNativeOnlyMethodsExistOnGoSession guards the allowlist: every name
// in nativeOnlyMethods must be a real exported Go Session method, so a
// stale entry (a renamed or removed batch op) cannot silently mask a
// future drift.
func TestNativeOnlyMethodsExistOnGoSession(t *testing.T) {
	all := make(map[string]bool)
	for _, n := range exportedMethodNames(reflect.TypeOf((*mdsmith.Session)(nil))) {
		all[n] = true
	}
	for n := range nativeOnlyMethods {
		if !all[n] {
			t.Fatalf("nativeOnlyMethods lists %q but *mdsmith.Session has no such exported method", n)
		}
	}
}

// mirroredGoMethodNames returns the exported Go Session method names
// minus the native-only batch ops, i.e. the set that must mirror into
// JS one-for-one.
func mirroredGoMethodNames() []string {
	var out []string
	for _, n := range exportedMethodNames(reflect.TypeOf((*mdsmith.Session)(nil))) {
		if nativeOnlyMethods[n] {
			continue
		}
		out = append(out, n)
	}
	return out
}

// TestCoreCapabilitiesAreMethods asserts every name Capabilities()
// advertises is present in the JS proxy method set, so a feature-detect
// in JS (capabilities().includes("check")) lines up with a callable
// method.
func TestCoreCapabilitiesAreMethods(t *testing.T) {
	s, err := mdsmith.NewSession(mdsmith.SessionOptions{
		Workspace: mdsmith.NewMemWorkspace(nil),
		Config:    mdsmith.ConfigYAML(""),
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	methods := make(map[string]bool)
	for _, m := range sessionMethodNames() {
		methods[m] = true
	}
	for _, c := range s.Capabilities() {
		if !methods[c] {
			t.Fatalf("capability %q is not a JS proxy method (%v)", c, sessionMethodNames())
		}
	}
}

// exportedMethodNames returns the exported method names of t in
// lower-camel form (the JS convention: Check -> check), excluding none.
func exportedMethodNames(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumMethod(); i++ {
		name := t.Method(i).Name
		if name == "" || !isExported(name) {
			continue
		}
		out = append(out, lowerFirst(name))
	}
	return out
}

func isExported(name string) bool {
	r := name[0]
	return r >= 'A' && r <= 'Z'
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
