---
settings:
  max-run: 2
  forbid-escaped-in-run: true
  forbid-adjacent-same-delim: true
---
# In fenced block

The patterns below sit inside a fenced code block and must not flag.

```text
*****\*a*
***bold-italic***
__a__b__
***Peter* Piper**
```
