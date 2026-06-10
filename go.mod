module github.com/jeduden/mdsmith

// This is the module graph `go install m@version` and library
// consumers resolve. Keep it lean: dev tools (golangci-lint, vhs,
// gobco) live in tools/go.mod — `go tool -modfile=tools/go.mod
// <tool>` — so their dependency trees and go-version floors never
// constrain consumers. TestRootGoModStaysInstallable and
// TestRootGoModCarriesNoDevTools enforce both properties.
go 1.25.0

require (
	cuelang.org/go v0.16.1
	github.com/bmatcuk/doublestar/v4 v4.10.0
	github.com/hexops/gotextdiff v1.0.3
	github.com/mattn/go-runewidth v0.0.24
	github.com/neurosnap/sentences v1.1.2
	github.com/pelletier/go-toml v1.9.5
	github.com/spf13/pflag v1.0.10
	github.com/stretchr/testify v1.11.1
	github.com/tetratelabs/wazero v1.11.0
	github.com/vmihailenco/msgpack/v5 v5.4.1
	golang.org/x/mod v0.34.0
	golang.org/x/net v0.52.0
	golang.org/x/sys v0.42.0
	golang.org/x/tools v0.43.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/cockroachdb/apd/v3 v3.2.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/proto v1.14.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/protocolbuffers/txtpbfmt v0.0.0-20260217160748-a481f6a22f94 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
