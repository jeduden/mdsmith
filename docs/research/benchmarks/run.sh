#!/usr/bin/env bash
# Reproducible cross-tool Markdown-linter benchmark.
#
# Builds a clean corpus, installs the comparison tools at pinned
# versions, and runs hyperfine over each tool's default check/lint
# command. Results land in $OUT (JSON + Markdown).
#
# Usage:  docs/research/benchmarks/run.sh [workdir]
#
# Requires network on first run (downloads prebuilt binaries and
# the rumdl/markdownlint packages). Go and a C-free toolchain are
# enough to build mdsmith; everything else ships as a prebuilt
# binary so no Rust/Node compile is needed.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK="${1:-/tmp/mdsmith-bench}"
BIN="$WORK/bin"
OUT="$WORK/out"
mkdir -p "$BIN" "$OUT"
export PATH="$BIN:$HOME/.local/bin:$WORK/npm/node_modules/.bin:$PATH"

HYPERFINE_VER="v1.20.0"
MADO_VER="v0.3.0"
PANACHE_VER="v2.46.0"

# --- tools -----------------------------------------------------------------
[ -x "$BIN/mdsmith" ] || ( cd "$REPO_ROOT" && go build -o "$BIN/mdsmith" ./cmd/mdsmith )

if [ ! -x "$BIN/hyperfine" ]; then
  curl -sSL \
    "https://github.com/sharkdp/hyperfine/releases/download/$HYPERFINE_VER/hyperfine-$HYPERFINE_VER-x86_64-unknown-linux-musl.tar.gz" \
    | tar xz -C "$WORK"
  cp "$WORK"/hyperfine-*/hyperfine "$BIN/"
fi

[ -x "$BIN/mado" ] || curl -sSL \
  "https://github.com/akiomik/mado/releases/download/$MADO_VER/mado-Linux-gnu-x86_64.tar.gz" \
  | tar xz -C "$BIN/"

[ -x "$BIN/panache" ] || { curl -sSL \
  "https://github.com/jolars/panache/releases/download/$PANACHE_VER/panache-x86_64-unknown-linux-gnu.tar.gz" \
  | tar xz -C "$WORK"; find "$WORK" -maxdepth 2 -name panache -type f -exec cp {} "$BIN/" \; ; }

command -v rumdl >/dev/null || uv tool install rumdl
[ -x "$WORK/npm/node_modules/.bin/markdownlint-cli2" ] || npm i --prefix "$WORK/npm" markdownlint-cli2

# --- corpora ---------------------------------------------------------------
# A: this repository's own Markdown (drop generated/bad fixtures).
if [ ! -d "$WORK/corpus_repo" ]; then
  mkdir -p "$WORK/corpus_repo"
  ( cd "$REPO_ROOT" && git ls-files '*.md' '*.markdown' \
      | grep -vE '^(demo/|internal/rules/[^/]*/bad/)' \
      | tar cf - -T - ) | tar xf - -C "$WORK/corpus_repo"
fi
# B: neutral third-party prose (Rust Book + Rust Reference).
if [ ! -d "$WORK/corpus_neutral" ]; then
  git clone --depth 1 -q https://github.com/rust-lang/book.git "$WORK/rust-book"
  git clone --depth 1 -q https://github.com/rust-lang/reference.git "$WORK/rust-ref"
  mkdir -p "$WORK/corpus_neutral"
  find "$WORK/rust-book/src" "$WORK/rust-ref/src" -name '*.md' -print0 \
    | tar --null -cf - -T - | tar xf - -C "$WORK/corpus_neutral"
fi

# --- benchmark -------------------------------------------------------------
# mdsmith        — default rule set (what users actually run; the
#                  cross-file / readability / generated layer is on).
# mdsmith-parity — only the structural rules that have a markdownlint
#                  analog, so the work class matches mado / rumdl /
#                  markdownlint. See bench-parity.mdsmith.yml.
PARITY="$REPO_ROOT/docs/research/benchmarks/bench-parity.mdsmith.yml"
for corpus in corpus_repo corpus_neutral; do
  hyperfine -i --warmup 3 --runs 10 -N \
    --command-name "mado"    "mado check $WORK/$corpus" \
    --command-name "rumdl"   "rumdl check --no-cache $WORK/$corpus" \
    --command-name "panache" "panache lint --no-cache $WORK/$corpus" \
    --command-name "mdsmith-parity" "mdsmith check -c $PARITY $WORK/$corpus" \
    --command-name "mdsmith" "mdsmith check $WORK/$corpus" \
    --export-json "$OUT/$corpus.json" \
    --export-markdown "$OUT/$corpus.md"
  hyperfine -i --warmup 2 --runs 6 -N \
    --command-name "markdownlint-cli2" "markdownlint-cli2 '$WORK/$corpus/**/*.md'" \
    --export-json "$OUT/${corpus}_mdl.json"
done

echo "Results written to $OUT"

# --- promote results to the committed source of truth ---------------------
# The fragments are derived from these JSON files, and the
# `bench-fragments` CI gate regenerates from the *committed* copies
# and fails on any diff. So a fresh run must update them here.
DATA_DIR="$REPO_ROOT/docs/research/benchmarks/data"
mkdir -p "$DATA_DIR"
for j in corpus_repo corpus_repo_mdl corpus_neutral corpus_neutral_mdl; do
  cp "$OUT/$j.json" "$DATA_DIR/$j.json"
done

# --- regenerate the doc fragments ------------------------------------------
# Single generator, shared with the CI drift gate. The docs do not
# hand-type numbers; they <?include?> these fragments. Run
# `mdsmith fix` afterwards so the include bodies in README /
# comparison / research docs refresh.
python3 "$REPO_ROOT/docs/research/benchmarks/gen_fragments.py" \
  "$DATA_DIR" "$REPO_ROOT/docs/research/benchmarks"
echo "Now run: mdsmith fix . (refresh <?include?> bodies)"
