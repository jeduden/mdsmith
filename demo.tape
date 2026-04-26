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
#   - Style: show, don't tell. Let mdsmith's output
#     speak. Comments are minimal labels, not narration.

Output assets/demo.gif

Set Shell "bash"
Set FontSize 14
Set Width 960
Set Height 540
Set Framerate 120
Set TypingSpeed 0.005
Set PlaybackSpeed 0.1

# --- Hidden setup ---
# Add mdsmith to PATH, create temp dirs for each scene,
# copy sample files and configs.
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
Type "TOC_DIR=$(mktemp -d)"
Enter
Type "cp -R demo/toc/. $TOC_DIR/"
Enter
Type "STRUCT_DIR=$(mktemp -d)"
Enter
Type "cp -R demo/structure/. $STRUCT_DIR/"
Enter
Type "DIR_DIR=$(mktemp -d)"
Enter
Type "cp -R demo/dirstructure/. $DIR_DIR/"
Enter
Type "LINK_DIR=$(mktemp -d)"
Enter
Type "cp -R demo/links/. $LINK_DIR/"
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

Type "# Bootstrap a project with every rule pinned to its default"
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

# --- Use case 2: Lint and auto-fix ---

Hide
Set TypingSpeed 0
Type "cd $DEMO_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Lint a file"
Enter
Sleep 100ms
Type "mdsmith check sample.md"
Sleep 100ms
Enter
Sleep 100ms

Type "# Auto-fix"
Enter
Sleep 100ms
Type "mdsmith fix sample.md"
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith check sample.md"
Sleep 100ms
Enter
Sleep 100ms

# --- Use case 3: Generate a catalog from front matter ---

Hide
Set TypingSpeed 0
Type "cd $CAT_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Catalog: rows generated from each file's front matter"
Enter
Sleep 100ms
Type "head -3 data/*.md"
Sleep 100ms
Enter
Sleep 100ms

Type "cat index.md"
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith fix index.md && cat index.md"
Sleep 100ms
Enter
Sleep 100ms

# --- Use case 4: Auto-generate a table of contents ---

Hide
Set TypingSpeed 0
Type "cd $TOC_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# TOC: nested heading list, generated from the document"
Enter
Sleep 100ms
Type "cat guide.md"
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith fix guide.md && sed -n '1,12p' guide.md"
Sleep 100ms
Enter
Sleep 100ms

# --- Use case 5: Enforce required document structure ---

Hide
Set TypingSpeed 0
Type "cd $STRUCT_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# required-structure: each file must match a schema"
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

# --- Use case 6: Catch broken cross-file links ---

Hide
Set TypingSpeed 0
Type "cd $LINK_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# cross-file-reference-integrity: links must resolve"
Enter
Sleep 100ms
Type "cat index.md"
Sleep 100ms
Enter
Sleep 100ms

Type "ls"
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith check ."
Sleep 100ms
Enter
Sleep 100ms

# --- Use case 7: Restrict file locations ---

Hide
Set TypingSpeed 0
Type "cd $DIR_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# directory-structure: only allow Markdown in listed dirs"
Enter
Sleep 100ms
Type "cat .mdsmith.yml"
Sleep 100ms
Enter
Sleep 100ms

Type "ls"
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith check ."
Sleep 100ms
Enter
Sleep 100ms

# --- Use case 8: Keep AI verbosity in check ---

Hide
Set TypingSpeed 0
Type "cd $ARI_DIR && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# paragraph-readability: dense AI prose flagged"
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

# --- Use case 9: Look up a rule ---

Hide
Set TypingSpeed 0
Type "cd $REPO && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Inspect a rule — settings, examples, diagnostics"
Enter
Sleep 100ms
Type "mdsmith help rule line-length | head -20"
Sleep 100ms
Enter
Sleep 100ms

# --- Use case 10: Query front matter ---

Hide
Set TypingSpeed 0
Type "clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Query: filter files by a CUE expression on front matter"
Enter
Sleep 100ms
Type `mdsmith query 'status: "✅"' plan/`
Sleep 100ms
Enter
Sleep 100ms

# --- Use case 11: Metrics ---

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

# --- Use case 12: Archetypes (reusable schemas) ---

Hide
Set TypingSpeed 0
Type "ARCH_SCENE=$(mktemp -d)"
Enter
Type "cd $ARCH_SCENE && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Archetypes: reusable schemas for required-structure"
Enter
Sleep 100ms
Type "mdsmith archetypes init"
Sleep 100ms
Enter
Sleep 100ms

Type `printf 'archetypes:\n  roots: [archetypes]\n' > .mdsmith.yml`
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith archetypes list"
Sleep 100ms
Enter
Sleep 100ms

Type "mdsmith archetypes show example | head -8"
Sleep 100ms
Enter
Sleep 100ms

# --- Use case 13: Git merge driver ---

Hide
Set TypingSpeed 0
Type "MERGE_DIR=$(mktemp -d)"
Enter
Type "cd $MERGE_DIR && git init -q && clear"
Enter
Sleep 50ms
Set TypingSpeed 0.005
Show

Type "# Merge driver: auto-resolve conflicts in generated sections"
Enter
Sleep 100ms
Type "mdsmith merge-driver install && cat .gitattributes"
Sleep 100ms
Enter
Sleep 100ms
