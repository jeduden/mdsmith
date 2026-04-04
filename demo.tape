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
#   - Recording speed: TypingSpeed, Sleep durations,
#     and Framerate are tuned for ~10x faster recording.
#     PlaybackSpeed 0.1 restores normal apparent speed
#     in the output GIF (~30 s recording → ~5 min GIF).
#   - Hidden blocks use TypingSpeed 0 since that typing
#     is never visible in the output.
#   - Keep Sleep durations short (50-100ms per step)
#     to avoid long VHS render times in CI.

Output assets/demo.gif

Set Shell "bash"
Set FontSize 14
Set Width 960
Set Height 540
Set Framerate 120
Set TypingSpeed 0.005
Set PlaybackSpeed 0.1

# --- Hidden setup ---
# Add mdsmith to PATH, create temp dirs for init and
# check/fix demos, copy sample file and config.
Hide
Set TypingSpeed 0
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
Type "CAT_DIR=$(mktemp -d)"
Enter
Type "cp -R demo/catalog/. $CAT_DIR/"
Enter
Type "STRUCT_DIR=$(mktemp -d)"
Enter
Type "cp -R demo/structure/. $STRUCT_DIR/"
Enter
Type "DIR_DIR=$(mktemp -d)"
Enter
Type "cp -R demo/dirstructure/. $DIR_DIR/"
Enter
Type "ARI_DIR=$(mktemp -d)"
Enter
Type "cp -R demo/readability/. $ARI_DIR/"
Enter
Type "clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

# --- Use case 1: Initialize a project ---

Hide
Set TypingSpeed 0
Type "cd $INIT_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Initialize mdsmith in a new project"
Enter
Sleep 100ms
Type "mdsmith init"
Sleep 100ms
Enter
Sleep 100ms

Type "head -18 .mdsmith.yml"
Sleep 100ms
Enter
Sleep 100ms

Type "# Config created with default rules"
Enter
Sleep 100ms

# --- Use case 2: Lint and fix a file ---

Hide
Set TypingSpeed 0
Type "cd $DEMO_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Check a file for lint issues"
Enter
Sleep 100ms
Type "mdsmith check sample.md"
Sleep 100ms
Enter
Sleep 100ms

Type "# Lint issues found — auto-fix them"
Enter
Sleep 100ms
Type "mdsmith fix sample.md"
Sleep 100ms
Enter
Sleep 100ms

Type "# Verify the fix"
Enter
Sleep 100ms
Type "mdsmith check sample.md"
Sleep 100ms
Enter
Sleep 100ms

Type "# All issues resolved"
Enter
Sleep 100ms

# --- Use case 3: Auto-generate a catalog ---

Hide
Set TypingSpeed 0
Type "cd $CAT_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Auto-generate a file catalog"
Enter
Sleep 100ms
Type "cat index.md"
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith check index.md"
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith fix index.md"
Sleep 100ms
Enter
Sleep 100ms

Type "cat index.md"
Sleep 100ms
Enter
Sleep 100ms

Type "# Catalog filled in from data/ files"
Enter
Sleep 100ms

# --- Use case 4: Enforce document structure ---

Hide
Set TypingSpeed 0
Type "cd $STRUCT_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Enforce required sections via a schema"
Enter
Sleep 100ms
Type "cat schema.md"
Sleep 100ms
Enter
Sleep 100ms

Type "cat plan.md"
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith check plan.md"
Sleep 100ms
Enter
Sleep 100ms

Type `# Missing "Tasks" section caught`
Sleep 100ms
Enter
Sleep 100ms

# --- Use case 5: Restrict file locations ---

Hide
Set TypingSpeed 0
Type "cd $DIR_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Restrict Markdown to allowed directories"
Enter
Sleep 100ms
Type "cat .mdsmith.yml"
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith check ."
Sleep 100ms
Enter
Sleep 100ms

Type "# notes.md flagged — not under docs/"
Enter
Sleep 100ms

# --- Use case 6: Keep AI verbosity in check ---

Hide
Set TypingSpeed 0
Type "cd $ARI_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Keep AI verbosity in check"
Enter
Sleep 100ms
Type "cat notes.md"
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith check notes.md"
Sleep 100ms
Enter
Sleep 100ms

Type "# ARI flags hard-to-read paragraphs"
Enter
Sleep 100ms

# --- Use case 7: Explore rules ---

Hide
Set TypingSpeed 0
Type "cd $REPO && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Look up a specific lint rule"
Enter
Sleep 100ms
Type "mdsmith help rule line-length | head -20"
Sleep 100ms
Enter
Sleep 100ms

Type "# Shows rule description, severity, and config"
Enter
Sleep 100ms

# --- Use case 8: Query front matter ---

Hide
Set TypingSpeed 0
Type "clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Find plans with status ✅"
Enter
Sleep 100ms
Type `mdsmith query 'status: "✅"' plan/`
Sleep 100ms
Enter
Sleep 100ms

Type "# Matched files listed"
Enter
Sleep 100ms

# --- Use case 9: Metrics ---

Hide
Set TypingSpeed 0
Type "clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Rank files by size"
Enter
Sleep 100ms
Type "mdsmith metrics rank --by bytes --top 5 ."
Sleep 100ms
Enter
Sleep 100ms

Type "# Top 5 largest files shown"
Enter
Sleep 100ms

# --- Use case 10: Merge driver for generated sections ---

Hide
Set TypingSpeed 0
Type "MERGE_DIR=$(mktemp -d)"
Enter
Type "cd $MERGE_DIR && git init -q && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Install merge driver for generated sections"
Enter
Sleep 100ms
Type "mdsmith merge-driver install"
Sleep 100ms
Enter
Sleep 100ms

Type "cat .gitattributes"
Sleep 100ms
Enter
Sleep 100ms

Type "# Auto-resolves conflicts for files listed in .gitattributes"
Enter
Sleep 100ms
