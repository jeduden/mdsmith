module github.com/jeduden/mdsmith

// This is the module graph `go install m@version` and library
// consumers resolve. Keep it lean: dev tools (golangci-lint, vhs,
// gobco) live in tools/go.mod — `go tool -modfile=tools/go.mod
// <tool>` — so their dependency trees and go-version floors never
// constrain consumers. TestRootGoModStaysInstallable and
// TestRootGoModCarriesNoDevTools enforce both properties.
go 1.25.11

require (
	github.com/bmatcuk/doublestar/v4 v4.10.0
	github.com/hexops/gotextdiff v1.0.3
	github.com/mattn/go-runewidth v0.0.24
	github.com/neurosnap/sentences v1.1.2
	github.com/pelletier/go-toml v1.9.5
	github.com/spf13/pflag v1.0.10
	github.com/stretchr/testify v1.11.1
	github.com/tetratelabs/wazero v1.12.0
	github.com/vmihailenco/msgpack/v5 v5.4.1
	golang.org/x/mod v0.37.0
	golang.org/x/net v0.56.0
	golang.org/x/sys v0.46.0
	golang.org/x/tools v0.45.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
