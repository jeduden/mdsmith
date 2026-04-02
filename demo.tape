# mdsmith terminal demo
# Rendered by VHS: https://github.com/charmbracelet/vhs
#
# VHS escaping rules (for future editors / AI agents):
#   - Double-quoted Type strings: Type "hello world"
#   - To embed quotes, use backtick-delimited strings:
#       Type `./cmd 'status: "✅"' args`
#   - Do NOT use escaped quotes inside double-quoted
#     Type strings — VHS crashes on parse.
#   - Every shell command should end with ; true (or
#     || true) so non-zero exits don't abort the tape.
#   - demo/sample.md is ignored by .mdsmith.yml; copy
#     it to /tmp for check/fix to bypass the ignore.
#   - Keep total Sleep time low (1-2s per step) to
#     avoid long VHS render times in CI.

Output assets/demo.gif

Set Shell "bash"
Set FontSize 14
Set Width 960
Set Height 540

# --- Step 1: init in a temporary directory ---
Type "d=$(mktemp -d) && cp ./mdsmith $d/ && (cd $d && ./mdsmith init && head -20 .mdsmith.yml); true"
Enter
Sleep 2s

# --- Step 2: check sample file with lint errors ---
Type "cp demo/sample.md /tmp/s.md && ./mdsmith check /tmp/s.md 2>&1; true"
Enter
Sleep 2s

# --- Step 3: fix the sample file ---
Type "./mdsmith fix /tmp/s.md 2>&1; true"
Enter
Sleep 1s

# --- Step 4: check again — clean pass ---
Type "./mdsmith check /tmp/s.md 2>&1; true"
Enter
Sleep 1s

# --- Step 5: help rule line-length ---
Type "./mdsmith help rule line-length 2>&1 | head -20; true"
Enter
Sleep 2s

# --- Step 6: help rule catalog ---
Type "./mdsmith help rule catalog 2>&1 | head -20; true"
Enter
Sleep 2s

# --- Step 7: help rule directory-structure ---
Type "./mdsmith help rule directory-structure 2>&1 | head -20; true"
Enter
Sleep 2s

# --- Step 8: help rule required-structure ---
Type "./mdsmith help rule required-structure 2>&1 | head -20; true"
Enter
Sleep 2s

# --- Step 9: query front-matter ---
Type `./mdsmith query 'status: "✅"' plan/; true`
Enter
Sleep 2s

# --- Step 10: metrics ---
Type "./mdsmith metrics rank --by bytes --top 5 .; true"
Enter
Sleep 2s
