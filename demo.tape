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
#   - Keep Sleep durations short (1-2s per step) to
#     avoid long VHS render times in CI.

Output assets/demo.gif

Set Shell "bash"
Set FontSize 14
Set Width 960
Set Height 540

# --- Use case 1: Initialize a project ---

Type "# Initialize mdsmith in a new directory"
Enter
Sleep 1s
Type "dir=$(mktemp -d); cp ./mdsmith $dir/; cd $dir; true"
Sleep 1s
Enter
Sleep 1s

Type "# Generate the default config"
Enter
Sleep 1s
Type "./mdsmith init; true"
Sleep 1s
Enter
Sleep 1s

Type "head -20 .mdsmith.yml; true"
Sleep 1s
Enter
Sleep 1s

Type "# Config created — go back to the repo"
Enter
Sleep 1s
Type "cd -; true"
Sleep 1s
Enter
Sleep 1s

Type "clear"
Enter
Sleep 1s

# --- Use case 2: Lint and fix a file ---

Type "# Copy a sample file and check it for lint issues"
Enter
Sleep 1s
Type "cp demo/sample.md /tmp/s.md; true"
Sleep 1s
Enter
Sleep 1s

Type "./mdsmith check /tmp/s.md; true"
Sleep 1s
Enter
Sleep 1s

Type "# Lint issues found — auto-fix them"
Enter
Sleep 1s
Type "./mdsmith fix /tmp/s.md; true"
Sleep 1s
Enter
Sleep 1s

Type "# Verify the file is now clean"
Enter
Sleep 1s
Type "./mdsmith check /tmp/s.md; true"
Sleep 1s
Enter
Sleep 1s

Type "# All issues resolved"
Enter
Sleep 1s

Type "clear"
Enter
Sleep 1s

# --- Use case 3: Explore rules ---

Type "# Look up a specific lint rule"
Enter
Sleep 1s
Type "./mdsmith help rule line-length | head -20; true"
Sleep 1s
Enter
Sleep 2s

Type "# Rules include severity, description, and examples"
Enter
Sleep 1s

Type "clear"
Enter
Sleep 1s

# --- Use case 4: Query front matter ---

Type "# Find plans with status ✅"
Enter
Sleep 1s
Type `./mdsmith query 'status: "✅"' plan/; true`
Sleep 1s
Enter
Sleep 1s

Type "# Matched files listed"
Enter
Sleep 1s

Type "clear"
Enter
Sleep 1s

# --- Use case 5: Metrics ---

Type "# Rank the largest files by size"
Enter
Sleep 1s
Type "./mdsmith metrics rank --by bytes --top 5 .; true"
Sleep 1s
Enter
Sleep 1s

Type "# Top 5 largest files shown"
Enter
Sleep 1s
