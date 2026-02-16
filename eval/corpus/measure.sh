#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FETCH_ROOT="${FETCH_ROOT:-/tmp/mdsmith-corpus-sources}"
DATASET_VERSION="${DATASET_VERSION:-v$(date -u +%Y-%m-%d)}"
COLLECTED_AT="${COLLECTED_AT:-$(date -u +%Y-%m-%d)}"
OUT_DIR="${OUT_DIR:-${ROOT_DIR}/eval/corpus/datasets/${DATASET_VERSION}}"
TMP_CONFIG="$(mktemp "${TMPDIR:-/tmp}/corpus-measure.XXXXXX.yml")"
GO_CACHE_DIR="${GOCACHE:-/tmp/mdsmith-corpus-gocache}"

cleanup() {
  rm -f "$TMP_CONFIG"
}
trap cleanup EXIT

need_cmd() {
  local bin="$1"
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "error: required command not found: ${bin}" >&2
    exit 1
  fi
}

fetch_repo() {
  local name="$1"
  local repo="$2"
  local sha="$3"
  local dest="${FETCH_ROOT}/${name}"

  mkdir -p "$dest"
  if [ ! -d "$dest/.git" ]; then
    git init "$dest" >/dev/null
  fi

  if git -C "$dest" remote get-url origin >/dev/null 2>&1; then
    git -C "$dest" remote set-url origin "https://github.com/${repo}.git"
  else
    git -C "$dest" remote add origin "https://github.com/${repo}.git"
  fi

  git -C "$dest" fetch --depth 1 origin "$sha" >/dev/null
  git -C "$dest" checkout --detach --force FETCH_HEAD >/dev/null

  local got
  got="$(git -C "$dest" rev-parse HEAD)"
  if [ "$got" != "$sha" ]; then
    echo "error: hash mismatch for ${repo}" >&2
    echo "expected: ${sha}" >&2
    echo "actual:   ${got}" >&2
    exit 1
  fi

  echo "fetched ${repo} @ ${sha}"
}

need_cmd git
need_cmd go

mkdir -p "$FETCH_ROOT"
mkdir -p "$OUT_DIR"

echo "[1/3] fetch pinned sources"
fetch_repo "openai-cookbook" "openai/openai-cookbook" "365dfaa2ef36e0a6b7639ba8d211a451e0e90455"
fetch_repo "openai-agents-python" "openai/openai-agents-python" "84fa471e5fc538d744a3ae294749fedb3855131b"
fetch_repo "langchain" "langchain-ai/langchain" "fb0233c9b9cdb95386e8fbb96c5421245fc192d3"
fetch_repo "langgraph" "langchain-ai/langgraph" "7216504ce2ecb56f62ebb08ac787d11b7491de5b"
fetch_repo "semantic-kernel" "microsoft/semantic-kernel" "91f795605e42f0dd03ed9cdfaf4ffd8bdb1ae553"
fetch_repo "claude-cookbooks" "anthropics/claude-cookbooks" "7cb72a9c879e3b95f58d30a3d7483906e9ad548e"
fetch_repo "kubernetes-website" "kubernetes/website" "695611df58280618252e50edf3962a8bd324731a"
fetch_repo "microsoft-autogen" "microsoft/autogen" "13e144e5476a76ca0d76bf4f07a6401d133a03ed"

echo "[2/3] generate measure config"
cat >"$TMP_CONFIG" <<EOF2
dataset_version: ${DATASET_VERSION}
collected_at: ${COLLECTED_AT}
seed: 62
min_words: 20
min_chars: 120
near_duplicate_threshold: 0.92
max_readme_share: 0.20
qa_sample_per_category: 5

license_allowlist:
  - MIT
  - CC-BY-4.0

policy:
  min_stars: 5000
  min_recent_commits_90d: 1
  require_ci: true

balance:
  agent-control:
    min: 0.03
    max: 0.35
  tutorial:
    min: 0.03
    max: 0.40
  how-to:
    min: 0.03
    max: 0.40
  reference:
    min: 0.10
    max: 0.55
  explanation:
    min: 0.03
    max: 0.35
  design-proposal:
    min: 0.03
    max: 0.35
  project-docs:
    min: 0.03
    max: 0.30
  api-cli-config:
    min: 0.03
    max: 0.45
  troubleshooting:
    min: 0.02
    max: 0.30

sources:
  - name: openai-cookbook
    repository: github.com/openai/openai-cookbook
    repository_url: https://github.com/openai/openai-cookbook
    root: ${FETCH_ROOT}/openai-cookbook
    commit_sha: 365dfaa2ef36e0a6b7639ba8d211a451e0e90455
    license: MIT
    include:
      - AGENTS.md
      - README.md
      - examples/**/*.md
    exclude:
      - examples/data/**
      - "**/*.ipynb"
    quality:
      stars: 71454
      recent_commits_90d: 30
      archived: false
      has_ci: true

  - name: openai-agents-python
    repository: github.com/openai/openai-agents-python
    repository_url: https://github.com/openai/openai-agents-python
    root: ${FETCH_ROOT}/openai-agents-python
    commit_sha: 84fa471e5fc538d744a3ae294749fedb3855131b
    license: MIT
    include:
      - AGENTS.md
      - CLAUDE.md
      - README.md
      - docs/**/*.md
    exclude:
      - "**/*.ipynb"
    quality:
      stars: 18961
      recent_commits_90d: 30
      archived: false
      has_ci: true

  - name: langchain
    repository: github.com/langchain-ai/langchain
    repository_url: https://github.com/langchain-ai/langchain
    root: ${FETCH_ROOT}/langchain
    commit_sha: fb0233c9b9cdb95386e8fbb96c5421245fc192d3
    license: MIT
    include:
      - AGENTS.md
      - CLAUDE.md
      - README.md
      - docs/**/*.md
    exclude:
      - "**/*.ipynb"
    quality:
      stars: 126724
      recent_commits_90d: 80
      archived: false
      has_ci: true

  - name: langgraph
    repository: github.com/langchain-ai/langgraph
    repository_url: https://github.com/langchain-ai/langgraph
    root: ${FETCH_ROOT}/langgraph
    commit_sha: 7216504ce2ecb56f62ebb08ac787d11b7491de5b
    license: MIT
    include:
      - AGENTS.md
      - CLAUDE.md
      - README.md
      - docs/**/*.md
    exclude:
      - "**/*.ipynb"
    quality:
      stars: 24747
      recent_commits_90d: 40
      archived: false
      has_ci: true

  - name: semantic-kernel
    repository: github.com/microsoft/semantic-kernel
    repository_url: https://github.com/microsoft/semantic-kernel
    root: ${FETCH_ROOT}/semantic-kernel
    commit_sha: 91f795605e42f0dd03ed9cdfaf4ffd8bdb1ae553
    license: MIT
    include:
      - README.md
      - docs/**/*.md
    exclude:
      - "**/*.ipynb"
    quality:
      stars: 27225
      recent_commits_90d: 35
      archived: false
      has_ci: true

  - name: claude-cookbooks
    repository: github.com/anthropics/claude-cookbooks
    repository_url: https://github.com/anthropics/claude-cookbooks
    root: ${FETCH_ROOT}/claude-cookbooks
    commit_sha: 7cb72a9c879e3b95f58d30a3d7483906e9ad548e
    license: MIT
    include:
      - CLAUDE.md
      - README.md
      - "**/*.md"
    exclude:
      - "**/*.ipynb"
      - .github/**
    quality:
      stars: 32921
      recent_commits_90d: 12
      archived: false
      has_ci: true

  - name: kubernetes-website
    repository: github.com/kubernetes/website
    repository_url: https://github.com/kubernetes/website
    root: ${FETCH_ROOT}/kubernetes-website
    commit_sha: 695611df58280618252e50edf3962a8bd324731a
    license: CC-BY-4.0
    include:
      - README.md
      - content/en/docs/tutorials/**/*.md
      - content/en/docs/tasks/**/*.md
      - content/en/docs/concepts/**/*.md
      - content/en/docs/reference/**/*.md
    exclude:
      - "**/*.html"
    quality:
      stars: 5168
      recent_commits_90d: 60
      archived: false
      has_ci: true

  - name: microsoft-autogen
    repository: github.com/microsoft/autogen
    repository_url: https://github.com/microsoft/autogen
    root: ${FETCH_ROOT}/microsoft-autogen
    commit_sha: 13e144e5476a76ca0d76bf4f07a6401d133a03ed
    license: CC-BY-4.0
    include:
      - README.md
      - docs/**/*.md
    exclude:
      - "**/*.ipynb"
    quality:
      stars: 54571
      recent_commits_90d: 20
      archived: false
      has_ci: true
EOF2

cp "$TMP_CONFIG" "${OUT_DIR}/config.generated.yml"

echo "[3/3] build corpus manifest"
(
  cd "$ROOT_DIR"
  GOCACHE="${GO_CACHE_DIR}" go run ./cmd/corpusctl build \
    -config "$TMP_CONFIG" \
    -out "$OUT_DIR"
)

echo "done"
echo "dataset: ${OUT_DIR}"
echo "config:  ${OUT_DIR}/config.generated.yml"
