# Technical evaluations

This directory contains dated evaluations of new ideas, data, observations, external tools,
and techniques against the system's architecture and workflows. Each evaluation is stored in its own distinct file.

Every evaluation identifies its sources, the repository revision reviewed, local evidence, and decisions or follow-up work.
Revisit a decision when its stated trigger or the surrounding code changes.

## Evaluations

| Evaluation | Decision |
| --- | --- |
| [go-adsb codec integration](2026-09-16-go-adsb.md) | Reuse raw fields, CRC, callsign, and altitude; implement encoding, velocity semantics, and guarded CPR locally |
