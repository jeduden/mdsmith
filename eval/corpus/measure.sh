#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONFIG_PATH="${CONFIG_PATH:-${ROOT_DIR}/eval/corpus/config.yml}"
FETCH_ROOT="${FETCH_ROOT:-/tmp/mdsmith-corpus-sources}"
DATASET_VERSION="${DATASET_VERSION:-v$(date -u +%Y-%m-%d)}"
COLLECTED_AT="${COLLECTED_AT:-$(date -u +%Y-%m-%d)}"
OUT_DIR="${OUT_DIR:-${ROOT_DIR}/eval/corpus/datasets/${DATASET_VERSION}}"
GO_CACHE_DIR="${GOCACHE:-/tmp/mdsmith-corpus-gocache}"

if ! command -v go >/dev/null 2>&1; then
  echo "error: required command not found: go" >&2
  exit 1
fi

if ! command -v git >/dev/null 2>&1; then
  echo "error: required command not found: git" >&2
  exit 1
fi

mkdir -p "${OUT_DIR}"

cd "${ROOT_DIR}"
GOCACHE="${GO_CACHE_DIR}" go run ./cmd/corpusctl measure \
  -config "${CONFIG_PATH}" \
  -out "${OUT_DIR}" \
  -fetch-root "${FETCH_ROOT}" \
  -dataset-version "${DATASET_VERSION}" \
  -collected-at "${COLLECTED_AT}"
