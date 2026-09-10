# SV1D capacity-protocol review — 2026-09-10

## Scope

This record preserves the first fresh independent Sol-xhigh review of the
exact candidate tree at `0ed4644d57ea377b45a397fa98cee2766465db50`.
The review was performed by an independent Feynman agent with no world
execution permission. It inspected the complete SV1D tree, the R2 calendar,
correctness hardening, binary evidence contract, activation package, and the
capacity runner.

## Verdict

`REJECTED_FOR_PROMOTION`

The rejection is accepted. It is a protocol defect reachable before any
scientific activation, not evidence against the R2 calendar or finite CDF
supplier mechanism.

## Decisive finding

The supposed capacity-only runner launched the registered treatment
configuration for seed `659` over a simulated 24-hour horizon, retained its
complete binary evidence, and required a completed terminal outcome before
writing the capacity attestation. Consequently the procedure was outcome
bearing: it exposed treatment behavior before the five-minute activation gate,
made the terminal survival result a prerequisite for capacity, and allowed the
capacity label to conceal a scientific trajectory. This contradicted the
preregistration's prohibition on a full campaign before activation.

No capacity runner, activation probe, development cell, freeze, or holdout was
run from the rejected tree.

## Required correction

The successor capacity contract is amended before implementation:

* use a deterministic synthetic `evstream_v3` production-shaped workload;
* use a workload seed distinct from `659` and never invoke `cmd/multivenue`;
* bind the registered treatment config only as a target identity and hash it,
  without normalizing or executing it;
* precommit the profile, timestamps, event-family mix, and event count;
* require stream termination, global ordering, canonical/uncompressed hash,
  readback verification, and closed file manifests;
* record resource measurements, but no market, terminal, valuation, survival,
  PnL, or actor outcome;
* fail closed if the simulator binary, `-config`, treatment seed, terminal
  artifacts, or holdout identifiers appear in the capacity path.

The synthetic measurement is a storage/resource prerequisite only. It cannot
authorize activation by itself; a fresh independent review of the corrected
exact tree remains mandatory first.

