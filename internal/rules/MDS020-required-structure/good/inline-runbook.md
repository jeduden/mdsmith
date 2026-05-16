---
settings:
  inline-schema:
    sections:
      - heading:
          regex: 'Symptoms|Indicators'
      - heading: "Diagnosis"
        sections:
          - heading: "Step"
            sections:
              - heading: "Check"
              - heading: "Expected"
              - heading:
                  regex: 'If different'
                  repeat: { min: 0, max: 1 }
      - heading:
          regex: 'References'
          repeat: { min: 0, max: 1 }
---
# Runbook

## Indicators

Listed as alias of Symptoms.

## Diagnosis

### Step

#### Check

Probe state.

#### Expected

Healthy.

## References

Doc link here.
