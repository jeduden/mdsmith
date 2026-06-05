#!/usr/bin/env bash
# Regenerate website/static/img/features/terminal.svg from a REAL
# `mdsmith check -> fix -> check` run, so the homepage "Auto-fix" feature
# artifact shows genuine CLI output. Re-run after mdsmith's diagnostic
# format changes, then commit the updated SVG.
#
# Requires Go and ansisvg on PATH. The committed SVG was generated with
# ansisvg v0.5.0 — install that exact version so re-runs don't churn it:
#   go install github.com/wader/ansisvg@v0.5.0
set -euo pipefail
cd "$(dirname "$0")/.."
out="website/static/img/features/terminal.svg"

command -v ansisvg >/dev/null 2>&1 || {
  echo "ansisvg not found; install: go install github.com/wader/ansisvg@v0.5.0" >&2
  exit 1
}

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
go build -o "$tmp/mdsmith" ./cmd/mdsmith

# Throwaway project with the canonical auto-fixable pair (trailing
# whitespace + a bare URL). Running here keeps diagnostic paths clean
# (intro.md) and limits output to those rules, so the artifact stays
# compact and the final check ends green. (mdsmith fix does not reflow
# long lines, so MDS001 is deliberately avoided.)
proj="$tmp/proj"; mkdir -p "$proj"
printf '# Quickstart\n\nLint your Markdown, then auto-fix it in one fast command.   \nRead the setup guide at https://github.com/jeduden/mdsmith today.\n' > "$proj/intro.md"

# Capture check -> fix -> check. mdsmith writes diagnostics to stderr and
# colours by default (no --no-color), so each command is run with 2>&1. A
# blank line separates commands, with none trailing. `|| true` tolerates
# check's non-zero exit when it finds issues; the assertion below turns a
# real failure (crash, changed format) into a hard error.
cap="$tmp/cap.ansi"; i=0
for c in check fix check; do
  [ "$i" -gt 0 ] && printf '\n'; i=1
  printf '\033[38;2;229;130;51m\xe2\x9d\xaf\033[0m mdsmith %s intro.md\n' "$c"
  ( cd "$proj" && "$tmp/mdsmith" "$c" intro.md 2>&1 ) || true
done > "$cap"

grep -q 'MDS012' "$cap" && grep -q 'failures=0' "$cap" || {
  echo "gen-feature-terminal: unexpected mdsmith output, refusing to write SVG:" >&2
  sed 's/\x1b\[[0-9;]*m//g' "$cap" >&2
  exit 1
}

# Render to a temp file first, then move into place, so a failed ansisvg
# never clobbers the committed artifact. --charboxsize gives pixel root
# dimensions plus a viewBox, for a stable intrinsic aspect ratio as <img>.
ansisvg --transparent --colorscheme 'Dark+' --fontsize 13 --lineheight 1.3 --charboxsize 8x17 < "$cap" > "$tmp/terminal.svg"
mkdir -p "$(dirname "$out")"
mv "$tmp/terminal.svg" "$out"
echo "wrote $out ($(wc -c < "$out") bytes)"
