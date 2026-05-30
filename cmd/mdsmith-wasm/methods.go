package main

// sessionMethodNames is the full set of methods the JS session proxy
// exposes, in lower-camel JS form. It is the single source of truth the
// WASM bridge (newSessionProxy) builds its proxy from and that a native
// test compares against pkg/mdsmith.Session's method set, so the two
// surfaces can never drift name-for-name.
//
// The first three (check, fix, kinds) are the core operations
// Capabilities() advertises; the last three (capabilities, invalidate,
// dispose) are introspection and lifecycle. Future plans add methods
// here and on the Go Session together — see
// docs/background/concepts/engine-api.md.
func sessionMethodNames() []string {
	return []string{
		"check",
		"fix",
		"kinds",
		"capabilities",
		"invalidate",
		"dispose",
	}
}
