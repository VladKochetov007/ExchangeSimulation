# Pre-V-020 derivative-semantics artifacts

This directory preserves derivative-analysis outputs generated before analyzer
commit `c8a8221`.  At a shared funding/expiry timestamp, the old analyzer used
the last position at or before the timestamp, so a later same-timestamp trade
could be treated as if it had occurred before funding.  The preserved raw
derivative log order disproved those apparent funding sign errors.

All current ae13f9a baseline and treatment `derivatives.json` artifacts have
been replayed with the corrected file-ordinal boundary.  The historic files
remain provenance only; see V-020 in `research/validation-audit.md`.
