#!/usr/bin/env bash
# Build the mdsmith WebAssembly artifact for in-process JS hosts (plan
# 217). Two targets:
#
#   ./build.sh        # standard Go toolchain (default)
#   ./build.sh tinygo # tinygo toolchain (smaller, slower to build)
#
# Both write mdsmith.wasm plus the matching wasm_exec.js into ./dist/.
# The standard-Go wasm_exec.js comes from $(go env GOROOT)/lib/wasm/;
# the tinygo one from $(tinygo env TINYGOROOT)/targets/.
#
# The artifacts are build outputs, not committed (see .gitignore).
set -euo pipefail

cd "$(dirname "$0")"
mkdir -p dist

target="${1:-go}"

case "$target" in
go)
	# -s -w strips the symbol table and DWARF; -trimpath drops local
	# path prefixes. Reproducible and as small as the standard runtime
	# allows.
	GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" \
		-o dist/mdsmith.wasm .
	cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" dist/wasm_exec.js
	;;
tinygo)
	if ! command -v tinygo >/dev/null 2>&1; then
		echo "build.sh: tinygo not found on PATH" >&2
		exit 1
	fi
	# -stack-size=1MB: the engine's package init (rule registry, the
	# regexp tables in internal/lint, config defaults) overflows
	# tinygo's default 64 KB goroutine stack, which surfaces as a
	# "memory access out of bounds" trap at startup. 1 MB clears it
	# with room to spare. See docs/background/concepts/engine-api.md.
	tinygo build -target wasm -no-debug -stack-size=1MB -o dist/mdsmith.wasm .
	cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" dist/wasm_exec.js
	;;
*)
	echo "build.sh: unknown target $target (want: go | tinygo)" >&2
	exit 2
	;;
esac

size=$(wc -c <dist/mdsmith.wasm)
printf 'built dist/mdsmith.wasm (%s, %d bytes)\n' "$target" "$size"
