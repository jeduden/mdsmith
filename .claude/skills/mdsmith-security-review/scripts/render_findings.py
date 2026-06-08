#!/usr/bin/env python3
"""Render a findings.json into three outputs: SARIF, a Markdown report, and
inline PR annotations. See references/output-formats.md for the schema.

Usage:
    python render_findings.py findings.json --out-dir ./out
"""
import argparse
import json
import os
import sys
from datetime import date

SEVERITY_ORDER = {"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4}
SARIF_LEVEL = {
    "critical": "error", "high": "error",
    "medium": "warning", "low": "note", "info": "note",
}
SECURITY_SEVERITY = {
    "critical": "9.5", "high": "8.0", "medium": "5.5", "low": "3.0", "info": "0.0",
}


def load(path):
    with open(path, encoding="utf-8") as fh:
        data = json.load(fh)
    if "findings" not in data:
        sys.exit("findings.json must have a top-level 'findings' array")
    for f in data["findings"]:
        sev = f.get("severity", "").lower()
        if sev not in SEVERITY_ORDER:
            sys.exit(f"finding {f.get('id', '?')} has bad severity {f.get('severity')!r}")
        f["severity"] = sev
    return data


def sort_key(f):
    return (SEVERITY_ORDER[f["severity"]],
            {"confirmed": 0, "likely": 1, "tentative": 2}.get(f.get("confidence"), 3),
            f.get("id", ""))


def loc_str(loc):
    if not loc:
        return "—"
    s = loc.get("file", "?")
    if loc.get("startLine"):
        s += f":{loc['startLine']}"
        if loc.get("endLine") and loc["endLine"] != loc["startLine"]:
            s += f"-{loc['endLine']}"
    return s


# ---- SARIF -----------------------------------------------------------------
def build_sarif(data):
    findings = data["findings"]
    rules, rule_index = [], {}
    for f in findings:
        rid = f["id"]
        if rid in rule_index:
            continue
        rule_index[rid] = len(rules)
        rules.append({
            "id": rid,
            "name": f.get("surface", "security"),
            "shortDescription": {"text": f.get("title", rid)},
            "fullDescription": {"text": f.get("description", "")},
            "defaultConfiguration": {"level": SARIF_LEVEL[f["severity"]]},
            "properties": {
                "security-severity": SECURITY_SEVERITY[f["severity"]],
                "tags": ["security"] + ([f["cwe"]] if f.get("cwe") else []),
            },
        })

    results = []
    for f in findings:
        physical = []
        for loc in [f.get("location")] + f.get("related_locations", []):
            if not loc or not loc.get("file"):
                continue
            region = {}
            if loc.get("startLine"):
                region["startLine"] = loc["startLine"]
                region["endLine"] = loc.get("endLine", loc["startLine"])
            phys = {"artifactLocation": {"uri": loc["file"]}}
            if region:
                phys["region"] = region
            physical.append({"physicalLocation": phys})
        results.append({
            "ruleId": f["id"],
            "ruleIndex": rule_index[f["id"]],
            "level": SARIF_LEVEL[f["severity"]],
            "message": {"text": f.get("title", f["id"])},
            "locations": physical or [{"physicalLocation": {
                "artifactLocation": {"uri": data.get("target", {}).get("repo", "unknown")}}}],
            "properties": {"confidence": f.get("confidence", "unspecified"),
                           "severity": f["severity"]},
        })

    return {
        "$schema": "https://json.schemastore.org/sarif-2.1.0.json",
        "version": "2.1.0",
        "runs": [{
            "tool": {"driver": {
                "name": "mdsmith-security-review",
                "informationUri": "https://github.com/jeduden/mdsmith",
                "rules": rules,
            }},
            "results": results,
        }],
    }


# ---- Markdown report -------------------------------------------------------
def build_report(data):
    t = data.get("target", {})
    findings = sorted(data["findings"], key=sort_key)
    real = [f for f in findings if f["severity"] != "info"]
    info = [f for f in findings if f["severity"] == "info"]
    counts = {s: sum(1 for f in findings if f["severity"] == s) for s in SEVERITY_ORDER}

    out = []
    out.append("# mdsmith Security Review\n")
    out.append(f"- **Target:** {t.get('repo', '?')} @ `{t.get('ref', '?')}`")
    out.append(f"- **Mode:** {t.get('mode', '?')}  ")
    out.append(f"- **Scope:** {t.get('scope', '?')}")
    out.append(f"- **Date:** {date.today().isoformat()}\n")

    out.append("## Summary\n")
    out.append(" | ".join(f"{s.capitalize()}: {counts[s]}" for s in SEVERITY_ORDER) + "\n")
    out.append("| ID | Sev | Conf | Title | Surface | Location |")
    out.append("|----|-----|------|-------|---------|----------|")
    for f in findings:
        out.append(f"| {f['id']} | {f['severity']} | {f.get('confidence', '?')} "
                   f"| {f.get('title', '')} | {f.get('surface', '')} "
                   f"| `{loc_str(f.get('location'))}` |")
    out.append("")

    def render(f):
        s = [f"### {f['id']} · {f.get('title', '')}\n"]
        s.append(f"**Severity:** {f['severity']} · **Confidence:** {f.get('confidence', '?')}"
                 f" · **Surface:** {f.get('surface', '?')}"
                 + (f" · **{f['cwe']}**" if f.get('cwe') else "") + "\n")
        s.append(f"**Location:** `{loc_str(f.get('location'))}`")
        for rl in f.get("related_locations", []):
            s.append(f"- related: `{loc_str(rl)}`")
        s.append("")
        if f.get("description"):
            s.append(f"**What.** {f['description']}\n")
        if f.get("impact"):
            s.append(f"**Impact.** {f['impact']}\n")
        if f.get("repro"):
            s.append(f"**Repro (sketch).** {f['repro']}\n")
        if f.get("remediation"):
            s.append(f"**Fix.** {f['remediation']}\n")
        return "\n".join(s)

    if real:
        out.append("## Findings\n")
        out.extend(render(f) for f in real)
    if info:
        out.append("## Hardening / Informational\n")
        out.extend(render(f) for f in info)

    out.append("## Coverage\n")
    out.append(data.get("coverage", "_Document what was and was not reviewed here._"))
    return "\n".join(out) + "\n"


# ---- Inline annotations ----------------------------------------------------
def build_annotations(data):
    anns = []
    for f in sorted(data["findings"], key=sort_key):
        loc = f.get("location")
        if not loc or not loc.get("file") or not loc.get("startLine"):
            continue
        body = (f"**[{f['id']} · {f['severity']}] {f.get('title', '')}**\n\n"
                f"{f.get('description', '')}\n\n"
                f"**Fix:** {f.get('remediation', 'n/a')}")
        anns.append({
            "path": loc["file"], "line": loc["startLine"], "side": "RIGHT",
            "severity": f["severity"], "id": f["id"], "body": body,
        })
    return anns


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("findings")
    ap.add_argument("--out-dir", default=".")
    args = ap.parse_args()

    data = load(args.findings)
    os.makedirs(args.out_dir, exist_ok=True)

    def write(name, content):
        p = os.path.join(args.out_dir, name)
        with open(p, "w", encoding="utf-8") as fh:
            fh.write(content)
        return p

    written = [
        write("findings.sarif", json.dumps(build_sarif(data), indent=2)),
        write("security-review.md", build_report(data)),
        write("inline-annotations.json",
              json.dumps(build_annotations(data), indent=2)),
    ]
    n = len(data["findings"])
    print(f"Rendered {n} finding(s) -> " + ", ".join(os.path.basename(p) for p in written))


if __name__ == "__main__":
    main()
