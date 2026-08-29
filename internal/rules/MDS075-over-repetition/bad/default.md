---
settings:
  scope: section
  max: 3
  min-length: 4
diagnostics:
  - line: 3
    column: 1
    message: '"process" repeated 5 time(s) in section (max 3)'
---
# Introduction

The process handles requests. Each process step validates input. The
process logs results after each process step, then the process ends.
