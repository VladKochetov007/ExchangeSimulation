# Pre-V-019 exposure artifacts

This directory preserves the 21 `exposure.json` outputs generated before
analyzer commit `995aede`.  They reconstruct delta and vega but not the
persisted second-order vanna and volga exposures.  They are historical
provenance only and must not be used to score `abl-vanna-volga-off`.

The corresponding raw evidence remains in `logs/`.  The live scoreboard
outputs are regenerated with the corrected analyzer after this archive was
created.  The V-019 record and regression tests are documented in
`research/validation-audit.md`.
