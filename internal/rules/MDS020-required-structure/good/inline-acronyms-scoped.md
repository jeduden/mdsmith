---
settings:
  inline-schema:
    sections:
      - heading: "Check"
      - heading:
          regex: 'Notes'
          repeat: { min: 0, max: 1 }
    acronyms:
      scope: ["Check"]
      known-safe: [API]
---
# Doc

## Check

The API responds in JSON (JavaScript Object Notation) on first call.

## Notes

Outside scope: OIDC is mentioned here without an
expansion and the rule does not fire.
