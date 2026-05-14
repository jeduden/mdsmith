---
title: "Compare"
summary: "How mdsmith compares to markdownlint, Vale, Prettier, and remark-lint across formatting, structural, cross-file, and AI-content rules."
---
# How mdsmith compares to other Markdown linters

mdsmith overlaps with general-purpose Markdown linters and prose linters,
but covers a different combination of concerns: formatting, structure, *and*
cross-file integrity, plus guardrails specific to AI-generated content. The
table below summarizes the overlap.

<table class="tbl">
  <thead>
    <tr>
      <th>Capability</th>
      <th>mdsmith</th>
      <th>markdownlint</th>
      <th>Vale</th>
      <th>Prettier</th>
      <th>remark-lint</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td>Whitespace / headings / list / fence formatting</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell yes">yes</td>
    </tr>
    <tr>
      <td>Auto-fix in place</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell partial">partial</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell yes">yes</td>
    </tr>
    <tr>
      <td>Cross-file link &amp; anchor integrity</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell partial">partial</td>
    </tr>
    <tr>
      <td>Per-file section schemas</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
    </tr>
    <tr>
      <td>Reading-grade &amp; sentence-structure metrics</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
    </tr>
    <tr>
      <td>Token-budget &amp; size caps for AI content</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
    </tr>
    <tr>
      <td>Self-maintaining sections (TOC, catalog, include)</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell partial">partial</td>
    </tr>
    <tr>
      <td>LSP server for editor integration</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell yes">yes</td>
      <td class="cmp-cell no">no</td>
      <td class="cmp-cell no">no</td>
    </tr>
    <tr>
      <td>Implementation language</td>
      <td>Go</td>
      <td>Node</td>
      <td>Go</td>
      <td>Node</td>
      <td>Node</td>
    </tr>
  </tbody>
</table>

The full breakdown is in the background note:
[Markdown linters compared]({{< relref "/docs/background/markdown-linters" >}}).
It covers each tool's design priorities and where they shine outside
mdsmith's scope.
