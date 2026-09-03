# SV1B activation/capacity ordering amendment

Date: 2026-09-03

Status: preregistered operational amendment; development only.

## Scope

This document amends the execution order in the SV1B successor protocol. It
does not change the CDF supplier roster, the R2 calendar, the eight historical
ABC/USD suppliers, any economic parameter, or the reserved holdout policy.
The predecessor R2 result remains archived as
`NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE`.

The earlier activation-diagnostics amendment is the authoritative definition
of the activation and capacity artifacts. This document makes its ordering
binding where the earlier successor preregistration described capacity before
activation.

## Binding order

For one exact scientific HEAD and one clean, pinned binary set:

1. obtain an independent review of the complete candidate tree;
2. build the simulator and analyzers from the reviewed HEAD;
3. run only the paired five-minute seed-643 activation probe;
4. require an accepted `ACTIVATION_CONTRACT_SATISFIED` provenance artifact for
   the exact HEAD and simulator binary hash;
5. measure capacity using a separate full-log 24-hour production-shaped
   workload. The workload uses the registered `treatment-643.json` or
   `control-643.json` configuration structure, but overrides the simulator seed
   to the preregistered non-scientific capacity-only seed `659`, at
   `GOMAXPROCS=4`; repeat the treatment at `GOMAXPROCS=8`;
6. bind each calibration attestation to its source configuration hash, the
   complete registered SV1B production launch-configuration hash set, and the
   accepted activation provenance hash;
7. only then run the registered development cells and their registered
   controls/parity cells.

The capacity workload is deliberately not seed 643 and is not a scored
treatment, a control, an activation result, or one of the registered
development cells. Its output must never be substituted for seed-643
development evidence. It exists only to measure the binary-evidence storage
floor after the mechanism has first demonstrated that its activation evidence
is mechanically valid. This removes the circularity of using an outcome-bearing
scientific development trajectory to justify the resource floor while retaining
the production configuration shape that determines evidence volume.

## Capacity contract

The capacity measurement source configurations are exactly:

* `research/configs/v2-r2-sv1b-24h/treatment-643.json`, overridden to seed `659`;
* `research/configs/v2-r2-sv1b-24h/control-643.json`, overridden to seed `659`.

The primary launch binding for each production-shaped workload is the same
registered source configuration. The three retained cases are:

* capacity-only seed 659, treatment source, `GOMAXPROCS=4`;
* capacity-only seed 659, control source, `GOMAXPROCS=4`;
* capacity-only seed 659, treatment source, `GOMAXPROCS=8`.

The activation roster configuration is used only by the preceding five-minute
activation probe:

`research/configs/v2-r2-sv1b/activation-643.json`

The attestation must list all seven registered SV1B launch configuration
hashes, including the no-log parity control. Each capacity run uses
`log_mode=full` and `evidence_format=evstream_v3`; the no-log parity control is
not a capacity measurement case. The measured effective seed-659 run-config
hash is retained separately from the registered source-config hash.

The capacity runner must:

- refuse a dirty source tree, a non-pinned binary, or an unaccepted activation
  provenance artifact;
- retain the complete probe root and never overwrite an existing attestation;
- enforce at least 4 GiB free space plus a 4 GiB safety margin;
- use a hard 20 GiB simulator address-space/RSS ceiling and a lower 18 GiB
  `GOMEMLIMIT` guard;
- record peak RSS and output size in the attestation;
- fail closed if the probe cell is not rooted under the attested probe root;
- publish no attestation after a failed, incomplete, or terminal-failure run.

The registered runner may consume the attestation only when the current
configuration hash is present in its authorized launch hash list, the
calibration flag is true, the activation provenance binding is valid, and the
attested evidence manifest verifies in place.

## Holdout boundary

No holdout seed (`619`, `631`, or `641`) is read, generated, probed, or
consumed by this amendment. Freeze authorization remains a separate explicit
boundary after development review; holdout validation remains unavailable
before that authorization.
