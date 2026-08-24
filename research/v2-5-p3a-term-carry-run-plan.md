# V2-5 P3a — execution and extraction plan

Status: **preregistered before P3a execution.** The scientific contract is
[`v2-5-p3a-term-carry-preregistration.md`](v2-5-p3a-term-carry-preregistration.md).

## Immutable cells

| arm | config | seed | horizon | output root |
| --- | --- | ---: | --- | --- |
| A, submission disabled | `configs/v2-5-p3a/A-107.json` | 107 | 5 min | `artifacts/v2-5-p3a/A/107/` |
| B, submission enabled | `configs/v2-5-p3a/B-107.json` | 107 | 5 min | `artifacts/v2-5-p3a/B/107/` |

Build `multivenue`, `mvanalyze`, and `prunegate` from the committed source
head. Record the source commit, binary SHA-256, config SHA-256, `GOMAXPROCS`,
wall time, terminal execution hash, and persisted-evidence artifact hash in
the final result. The two configs may differ only in provenance strings and
`term_carry_allocator.enabled`; byte-normalize that declared provenance before
launch and preserve the diff. Each output directory must be absent before
launch.

Run both worlds with full raw evidence only after checking disk headroom.
Completion requires final non-empty `greeks.json` **and** `latency.json`;
process names, partial sidecars, logs, checkpoints, or terminal messages are
not completion sentinels.

## Required extraction before any interpretation or prune decision

For every completed cell, retain raw evidence and write the seven artifacts
listed in the preregistration. Inspect the `termcarry` action counts and
lifecycle fields without inspecting basis or funding performance: `OpenTerms`
is expected for a genuine short-screen entry, while P3a must not call it a
close or a realized term. Run `prunegate` only as a read-only check; P3a raw
evidence is retained regardless of its response.

## Stop rules

- Any failed evidence, receipt, arithmetic, lifecycle, conservation,
  position, order-lifecycle, or latency contract invalidates that cell and
  halts P3a interpretation.
- A control submission or an enabled-arm missing accepted/fill-qualified leg
  is an actor-integrity failure / `NOT EXERCISED`, never a funding conclusion.
- A matched inventory state without an exact `TERM_ACTIVE` lifecycle replay is
  not an activation pass.
- Do not inspect or score funding payment, carry return, basis, price, or
  stability outcomes until P3a is valid and the later P3b/P3c contracts exist.
