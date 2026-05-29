package main

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// TestSessionMethodSetMatchesGo asserts the JS session proxy's method
// set matches the Go *mdsmith.Session method set name-for-name (modulo
// the Go→JS lower-camel convention). The WASM bridge builds its proxy
// from sessionMethodNames(); this test ties that list to the actual Go
// methods via reflection, so adding a Go method without exposing it in
// JS (or vice versa) fails the build's tests.
func TestSessionMethodSetMatchesGo(t *testing.T) {
	jsNames := sessionMethodNames()
	sort.Strings(jsNames)

	goNames := exportedMethodNames(reflect.TypeOf((*mdsmith.Session)(nil)))
	sort.Strings(goNames)

	if !reflect.DeepEqual(jsNames, goNames) {
		t.Fatalf("JS session methods %v != Go Session methods %v", jsNames, goNames)
	}
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
