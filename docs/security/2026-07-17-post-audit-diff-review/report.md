---
date: "2026-07-17"
scope: "diff since the 2026-07-10 post-audit diff review — SARIF 2.1.0 output, apm-input-token placeholder, the init --add pack surface, the okf starter, and the perf/schema churn"
method: "audit"
title: "mdsmith post-audit diff review — 2026-07-17"
summary: "Diff review of the 185 files (~6990 insertions) merged since the 2026-07-10 review. No findings at any severity. The two feature surfaces — SARIF 2.1.0 output for check/fix and the apm-input-token placeholder — add no new sink: SARIF serializes to the output stream (no attacker-controlled path, JSON-escaped fields) and the placeholder uses a bounded RE2 regex (no ReDoS). The new init --add pack write path is triple-defended (relative .mdsmith/ containment, symlinked-parent refusal, non-clobbering Lstat) and its paths come from built-in data, not documents. Schema churn is map-to-struct{} set refactors plus a byte-cap added to the index side-output read (hardening). §0 baseline holds: no editor, CI, npm, python, LSP, or merge-driver source changed, and no new exec/spawn sink appears anywhere in the diff."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `d496596`
- **Mode:** audit
- **Scope:** diff since the 2026-07-10 post-audit diff review — SARIF 2.1.0 output, apm-input-token placeholder, the init --add pack surface, the okf starter, and the perf/schema churn
- **Date:** 2026-07-17

## Summary

Critical: 0 | High: 0 | Medium: 0 | Low: 0 | Info: 0

| ID  | Sev | Conf | Title | Surface | Location |
| --- | --- | ---- | ----- | ------- | -------- |

## Coverage

Diff review of the 185 files (~6990 insertions) changed between the 2026-07-10 review base (9c36a270)
and HEAD (d496596). Work maps to two feature surfaces plus perf/test churn.

(1) SARIF 2.1.0 output for `mdsmith check -f sarif` and `mdsmith fix --dry-run -f sarif`
(internal/output/sarif.go, cmd/mdsmith/{check,fix,main}.go): the formatter serializes diagnostics to the
process output stream (stdout/stderr) via json.Encoder — no attacker-controlled file path, no shell/URI
injection, since JSON encoding escapes the file/message fields, which are the only document-derived data.

(2) apm-input-token placeholder (internal/placeholders/placeholders.go): matched by a bounded RE2 regex
`\$\{input:[A-Za-z][\w-]{0,63}\}` — linear-time, no ReDoS.

New file-write surface — `mdsmith init --add <pack>` (internal/pack/*, cmd/mdsmith/main.go writeScaffolds):
pack file paths come from built-in convention data, not documents, and the write boundary is triple-defended.
validatePackPath confines each path to a relative `.mdsmith/` first component after filepath.Clean;
refuseSymlinkedParents Lstat-rejects any symlinked parent component; statTarget Lstat-rejects a symlinked or
existing target and never clobbers. The okf starter reads embedded templates via embed.FS (fs.ValidPath
rejects `..`; the name is a user CLI arg, not document content).

New MDS071 requiredfrontmatter rule and requiredstructure/fieldpatterncache.go: the only new dynamic regex is
built from regexp.QuoteMeta'd literals joined with `.+`, so MustCompile cannot panic and matching stays
linear; MDS071 re-reads only the file under lint via its own fs.FS. The schema/index/validate churn is
map[K]bool->map[K]struct{} set refactors plus routing the index side-output read through
bytelimit.ReadFileLimited — a cap where the previous read was uncapped (hardening).

§0 baseline reconfirmed by exclusion: no editors/, .github/ workflow, npm/, python/, LSP, merge-driver, or
pre-merge-commit source changed this window; the one build-surface change (MDS040 internal/rules/build/rule.go)
is a map->struct{} refactor that leaves the rule advisory-only and the reserved-device-name path check
unchanged. No new exec.Command/child_process/spawn sink appears anywhere in the diff. No Critical/High/Medium/
Low/Info findings.

Not re-reviewed (unchanged this window): the LSP server, VS Code extension, Obsidian plugin, Git integration,
and CI/distribution wrappers — last covered by the 2026-06-19 and 2026-07-03 reviews.
