#!/usr/bin/env bash
# merge-driver-catalog.sh — custom merge driver for files with
# auto-generated catalog sections (PLAN.md, README.md).
#
# Catalog sections are delimited by <!-- catalog --> and
# <!-- /catalog --> markers. Their content is fully regenerated
# by `mdsmith fix`, so conflicts inside catalogs are resolved
# by keeping both sides' additions and deduplicating rows,
# then running `mdsmith fix` to regenerate the canonical table.
#
# For conflicts outside catalog sections, normal merge markers
# are preserved so the user (or AI agent) can resolve them.
#
# Usage (configured via .gitattributes + git config):
#   [merge "catalog"]
#     name = Catalog-aware Markdown merge
#     driver = scripts/merge-driver-catalog.sh %O %A %B %P
#
# Arguments:
#   $1 = %O (ancestor / base)
#   $2 = %A (ours — also the output file)
#   $3 = %B (theirs)
#   $4 = %P (pathname, optional)

set -euo pipefail

BASE="$1"
OURS="$2"
THEIRS="$3"
PATHNAME="${4:-}"

# Step 1: attempt standard 3-way merge.
# git merge-file exits 0 on clean merge, >0 on conflicts.
if git merge-file "$OURS" "$BASE" "$THEIRS" 2>/dev/null; then
  # Clean merge — run mdsmith fix for consistency.
  if command -v mdsmith >/dev/null 2>&1 && [ -n "$PATHNAME" ]; then
    mdsmith fix "$PATHNAME" 2>/dev/null || true
  fi
  exit 0
fi

# Step 2: conflicts detected. Resolve catalog sections by
# stripping conflict markers inside catalog blocks, removing
# duplicate table rows, then regenerating via mdsmith fix.
awk '
BEGIN { in_catalog = 0; in_conflict = 0; side = "" }

/^<!-- catalog/ { in_catalog = 1 }
/^<!-- \/catalog -->/ { in_catalog = 0 }

# Inside a catalog section, resolve conflict markers.
in_catalog && /^<<<<<<</ { in_conflict = 1; side = "ours"; next }
in_catalog && /^=======/ { side = "theirs"; next }
in_catalog && /^>>>>>>>/ { in_conflict = 0; side = ""; next }

# Outside catalog sections, pass everything through unchanged
# (including any conflict markers for manual resolution).
{ print }
' "$OURS" > "${OURS}.tmp"

# Deduplicate table rows inside catalog sections.
awk '
BEGIN { in_catalog = 0 }
/^<!-- catalog/ { in_catalog = 1 }
/^<!-- \/catalog -->/ { in_catalog = 0 }
in_catalog && /^\|/ {
  if (!seen[$0]++) print
  next
}
{ print }
' "${OURS}.tmp" > "${OURS}.dedup"
mv "${OURS}.dedup" "$OURS"
rm -f "${OURS}.tmp"

# Step 3: regenerate catalog content with mdsmith fix.
if command -v mdsmith >/dev/null 2>&1 && [ -n "$PATHNAME" ]; then
  mdsmith fix "$PATHNAME" 2>/dev/null || true
fi

# Step 4: check for remaining conflict markers.
if grep -q '^<<<<<<<' "$OURS" 2>/dev/null; then
  exit 1
fi

exit 0
