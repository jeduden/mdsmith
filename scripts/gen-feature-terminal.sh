#!/usr/bin/env bash
# Regenerate website/static/img/features/terminal.svg from REAL mdsmith
# output (check -> fix -> check) so the homepage "Auto-fix" feature artifact
# shows genuine CLI output instead of a hand-drawn mockup. Re-run whenever
# mdsmith's diagnostic format changes; commit the updated SVG.
#
# Requires:
#   - Go (builds ./cmd/mdsmith)
#   - ansisvg on PATH:  go install github.com/wader/ansisvg@latest
set -euo pipefail
cd "$(dirname "$0")/.."
out="website/static/img/features/terminal.svg"

command -v ansisvg >/dev/null 2>&1 || {
  echo "ansisvg not found; install with: go install github.com/wader/ansisvg@latest" >&2
  exit 1
}

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
go build -o "$tmp/mdsmith" ./cmd/mdsmith

# A throwaway project dir with the canonical auto-fixable pair (trailing
# whitespace + a bare URL). Running here keeps the diagnostic paths clean
# (intro.md) and limits output to these two rules so the artifact stays
# compact and ends green.
proj="$tmp/proj"; mkdir -p "$proj"
printf '# Quickstart\n\nLint your Markdown, then auto-fix it in one fast command.   \nRead the setup guide at https://github.com/jeduden/mdsmith today.\n' > "$proj/intro.md"

cap="$tmp/cap.ansi"
for c in check fix check; do
  # Forge-coloured prompt, then the real (force-coloured) command output.
  printf '\033[38;2;229;130;51m\xe2\x9d\xaf\033[0m mdsmith %s intro.md\n' "$c"
  ( cd "$proj" && CLICOLOR_FORCE=1 FORCE_COLOR=1 "$tmp/mdsmith" "$c" intro.md 2>&1 ) || true
  printf '\n'
done > "$cap"
sed -i '${/^$/d}' "$cap"

mkdir -p "$(dirname "$out")"
ansisvg --transparent --colorscheme 'Dark+' --fontsize 13 --lineheight 1.3 < "$cap" > "$out"
echo "wrote $out ($(wc -c < "$out") bytes)"
