# mdsmith terminal demo
# Rendered by VHS: https://github.com/charmbracelet/vhs
#
# VHS escaping rules (for future editors / AI agents):
#   - Double-quoted Type strings: Type "hello world"
#   - To embed quotes, use backtick-delimited strings:
#       Type `./cmd 'status: "✅"' args`
#   - Do NOT use escaped quotes inside double-quoted
#     Type strings — VHS crashes on parse.
#   - Hidden setup runs set +e, adds mdsmith to PATH,
#     and prepares temp dirs so visible commands match
#     what a developer would actually type.
#   - demo/sample.md is ignored by .mdsmith.yml; the
#     hidden setup copies it to a temp dir and runs
#     check/fix there as sample.md.
#   - Keep Sleep durations short (1-2s per step) to
#     avoid long VHS render times in CI.

Output assets/demo.gif

Set Shell "bash"
Set FontSize 14
Set Width 960
Set Height 540

# --- Hidden setup ---
# Add mdsmith to PATH, create temp dirs for init and
# check/fix demos, copy sample file and config.
Hide
Type "set +e"
Enter
Type "export PATH=$PWD:$PATH"
Enter
Type "REPO=$PWD"
Enter
Type "INIT_DIR=$(mktemp -d)"
Enter
Type "DEMO_DIR=$(mktemp -d)"
Enter
Type "cp demo/sample.md $DEMO_DIR/sample.md"
Enter
Type "cp .mdsmith.yml $DEMO_DIR/"
Enter
Type "clear"
Enter
Sleep 1s
Show

# --- Use case 1: Initialize a project ---

Hide
Type "cd $INIT_DIR && clear"
Enter
Sleep 1s
Show

Type "# Initialize mdsmith in a new project"
Enter
Sleep 1s
Type "mdsmith init"
Sleep 1s
Enter
Sleep 1s

Type "cat .mdsmith.yml"
Sleep 1s
Enter
Sleep 1s

Type "# Config created with default rules"
Enter
Sleep 1s

# --- Use case 2: Lint and fix a file ---

Hide
Type "cd $DEMO_DIR && clear"
Enter
Sleep 1s
Show

Type "# Check a file for lint issues"
Enter
Sleep 1s
Type "mdsmith check sample.md"
Sleep 1s
Enter
Sleep 1s

Type "# Lint issues found — auto-fix them"
Enter
Sleep 1s
Type "mdsmith fix sample.md"
Sleep 1s
Enter
Sleep 1s

Type "# Verify the fix"
Enter
Sleep 1s
Type "mdsmith check sample.md"
Sleep 1s
Enter
Sleep 1s

Type "# All issues resolved"
Enter
Sleep 1s

# --- Use case 3: Explore rules ---

Hide
Type "cd $REPO && clear"
Enter
Sleep 1s
Show

Type "# Look up a specific lint rule"
Enter
Sleep 1s
Type "mdsmith help rule line-length"
Sleep 1s
Enter
Sleep 2s

Type "# Shows rule description, severity, and config"
Enter
Sleep 1s

# --- Use case 4: Query front matter ---

Hide
Type "clear"
Enter
Sleep 1s
Show

Type "# Find plans with status ✅"
Enter
Sleep 1s
Type `mdsmith query 'status: "✅"' plan/`
Sleep 1s
Enter
Sleep 1s

Type "# Matched files listed"
Enter
Sleep 1s

# --- Use case 5: Metrics ---

Hide
Type "clear"
Enter
Sleep 1s
Show

Type "# Rank files by size"
Enter
Sleep 1s
Type "mdsmith metrics rank --by bytes --top 5 ."
Sleep 1s
Enter
Sleep 1s

Type "# Top 5 largest files shown"
Enter
Sleep 1s
