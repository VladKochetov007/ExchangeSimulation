# V2 R5 `dev-607` extraction failure

Date: 2026-08-29 UTC  
Cell: `/home/vlad/v2-integrated-longrun-candidate-20260828-v5/dev-607`  
Source revision during the run: `adc4e03fd4e2141a30db753421dbb1535216f4de`

The registered simulator run completed the 24-hour horizon with exit status
zero and a valid raw evidence manifest. The first fail-closed extraction then
stopped at the exact-replay `positions` metric before writing any derived
artifact. The analyzer error was:

```text
-require-exact-replay: analysis: read report: open -require-exact-replay/greeks.json: no such file or directory
```

Cause: Go's standard flag parser stops at the first positional argument. The
extractor placed `-require-exact-replay` (and, for other strict metrics, the
other flags) after the positional cell directory, so the analyzer interpreted
the flag as a second run directory.

Action: reorder all strict analyzer flags before `-json <cell-directory>` in
the extractor. The raw run directory was not pruned, scored, or modified by
the failed extraction; only its non-evidence analyzer error sidecar was
preserved for diagnosis and moved aside before retry.

The retry then failed before analysis because the run was produced at the
previous protocol revision (`adc4e03...`) and the extractor correctly requires
the run's source revision to equal the current analysis HEAD (`f6f7c1a...`).
That completed run remains preserved as a stale-revision diagnostic and is
not eligible for scoring. A fresh registered `dev-607` run is required at the
final reviewed HEAD.
