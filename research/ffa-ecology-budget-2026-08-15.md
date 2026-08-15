# FFA Ecology Budget

```text
verification tier       = C for market claims; A for engine/invariant gates
cpu parallelism         = 14 of 16 logical CPUs
gpu                     = 0 hours; no profiled numerical bottleneck justifies it
phase-0 compute cap     = 4 CPU-hours
phase-0 generations     = 3 bounded batches
phase-0 experiments     = 6 maximum, including controls
replicas per accepted arm = 8 development seeds + 8 held-out seeds
log policy              = persistent logs/research/<manifest-id>; aggregate
                          by default, full event logs only for retained arms
```

Phase-0 consumes no large simulation budget until the scenario manifest,
information contract, valuation, and deterministic digest checker exist. A
single run that lacks a complete account conversion, exact matcher policy, or
deterministic digest is recorded as invalid rather than retried for a better
number.

## Generation 0

| ID | Angle | Status | Cost ceiling | Gate |
| --- | --- | --- | ---: | --- |
| FFA-00 | configuration semantics | passed | 0.05 CPU-h | explicit zero option-flow probability remains zero; omission defaults |
| FFA-01 | venue allocation | passed | 0.5 CPU-h | per-venue exact FIFO/pro-rata selection, validation, and deterministic tie policy |
| FFA-02 | graph execution | queued | 1.0 CPU-h | 3-asset valuation/conservation and triangle null control |
| FFA-03 | information boundary | queued | 0.5 CPU-h | no direct exchange state from strategy code; sequence/resync test |
| FFA-04 | population accounting | passed for USD/ABC | 1.0 CPU-h | strict initial/terminal account rows for every participant, or invalid run |
| FFA-05 | ecology control | queued | 1.0 CPU-h | fixed-mixture two-strategy invasion control before selection |

## Abort and rollover

- Stop the batch when a required contract invariant cannot be measured.
- Do not inflate roster size after a negative/null result; first determine
  whether the result is expected from the model or a measurement failure.
- Any retained full log must be named by manifest hash and stored under
  `logs/research`, never a system temporary directory.
