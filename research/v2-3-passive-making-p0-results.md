# V2-3 P0 replacement results — passive refresh contract

Status: **complete short mechanism screen; not a price-stability or robustness
claim.**

This report scores only `V2-3-P0-R1`. The earlier six-world attempt is retained
but invalidated before scoring because the field named `spot_passive_maker_*`
also changed derivative makers; see
[`v2-3-p0-invalidated-attempt.md`](v2-3-p0-invalidated-attempt.md). No
attempt-0 number appears below.

## Contract and provenance

The immutable inputs are
`research/configs/v2-3-p0-r1/{A,B,C}-{101,103}.json`; the exact raw evidence,
sidecars, evidence artifact digests, and score rows are in
`research/artifacts/v2-3-p0-r1/`. Every cell used the same binary hash
`5d7bec090afc0cd7c842a102110c8a9925bff6ed90cb7171d0dade79862b3a04`,
`GOMAXPROCS=2`, a five-minute horizon, and raw-evidence retention. The source
revision recorded at run time was `f4d4c0c8d36e35b8ee0bd3c5c323af53fb41c29a`.

| Arm | Contract |
| --- | --- |
| A | ordinary refresh request; legacy submit-before-cancel |
| B | post-only at arrival; legacy submit-before-cancel |
| C | post-only at arrival; actor sends cancel-before-replace |

Thus B−A identifies the venue admission contract and C−B identifies requested
actor ordering. Request latency still controls actual arrival order; neither
is atomic replace. The policy is restricted to `SpotInstrument`s. It does not
apply to perp or derivative-configured naïve makers.

All six V2-0 receipt/frontier audits were valid. The required receipt roles
had nonzero schedules and receipts; all three CDF/USD maker classes had
accepted activity; all three CDF/USD venues had 900 snapshots; every pooled
two-sided share, trade count, and volume was positive. The derivative-scope
audit found zero accepted post-only ABC-PERP orders in every cell. Raw logs are
retained; nothing in this report is prunable yet.

## Raw evidence

`policy rejects` counts `POST_ONLY_WOULD_TAKE` across the declared passive
spot policy. CDF-specific rejects were zero in every cell. A post-only order
filling later as resting liquidity is expected and is not an admission
violation.

| Cell | persisted events / digest prefix | CDF accepted (post/regular) | policy rejects | CDF two-sided share | CDF trades | CDF volume | derivative post-only accepts |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A/101 | 315,518 / `0a3defbfb52a272a` | 1,118 (0 / 1,118) | 0 | 0.9800 | 2,504 | 851,466,222,541 | 0 |
| B/101 | 315,864 / `9aec092f6b735449` | 1,118 (1,118 / 0) | 13 | 0.9800 | 2,504 | 851,466,222,541 | 0 |
| C/101 | 351,946 / `1926215c26a79a9b` | 1,138 (1,138 / 0) | 69 | 0.9678 | 5,450 | 792,121,832,188 | 0 |
| A/103 | 313,924 / `c4925c896ec6e8c0` | 1,264 (0 / 1,264) | 0 | 0.9600 | 2,794 | 879,125,360,556 | 0 |
| B/103 | 317,529 / `0704d6d8de124eec` | 1,266 (1,266 / 0) | 39 | 0.9600 | 3,027 | 879,405,360,556 | 0 |
| C/103 | 355,776 / `5486c6e0f0ea53df` | 1,192 (1,192 / 0) | 96 | 0.9644 | 4,642 | 838,327,632,005 | 0 |

CDF role-level accepted counts (Stoikov / fixed-distance / imbalance) were:
A/101 366/572/180, B/101 366/572/180, C/101 294/636/208;
A/103 356/684/224, B/103 356/686/224, C/103 286/692/214. Therefore no class
was silently inactive.

## Paired deltas and interpretation

| Seed | comparison | Δ policy rejects | Δ CDF accepted | Δ two-sided share | Δ trades | Δ volume |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| 101 | B−A | +13 | 0 | 0.0000 | 0 | 0 |
| 103 | B−A | +39 | +2 | 0.0000 | +233 | +280,000,000 |
| 101 | C−B | +56 | +20 | −0.0122 | +2,946 | −59,344,390,353 |
| 103 | C−B | +57 | −74 | +0.0044 | +1,615 | −41,077,728,551 |

### B−A — exchange post-only admission: **SUPPORTED (mechanical screening)**

All selected CDF orders changed from regular to explicit post-only while
legacy actor request order remained. The passive **spot** policy was actually
challenged: 13 and 39 arrival-time would-take requests rejected in seeds 101
and 103. The independent exchange fixture proves the stronger admission
invariant: a marketable post-only request rejects before ID allocation,
reserve/borrow, fill, or book mutation; stripping the bit produces the fill.
The derivative scope was clean.

The direct CDF/USD would-take count was zero, so this screen does **not**
identify a CDF-specific economic consequence of rejected passive refreshes.
The global spot policy was exercised on its other spot book(s), and the CDF
viability outcome is therefore a short spillover/viability observation, not
proof that the CDF runaway mechanism has been removed.

### C−B — actor cancellation/replacement order: **MIXED**

Actor tests establish the requested cancel-before-replace order, independently
of network arrival. In both seeds it raised policy rejections by 56–57 and
raised CDF trade count (+2,946, +1,615), while CDF volume fell
(−59.34bn, −41.08bn). The two-sided-share changes disagree in sign. All
viability gates survived. These two short paired worlds support a real
ordering effect but give no consistent direction for displayed-book quality
or traded volume, and no robustness claim is warranted.

## Non-claims and next gate

P0 did not tune or score terminal price, spread parameters, maker skew,
clocks, demand, latency, or population. It does not establish CDF price
stability, identify an inventory-feedback cure, or validate a longer horizon.
It does establish a usable separation between venue passive admission and
actor refresh ordering, with scoped evidence and adversarial mechanical tests.

The next V2-3 slice may now use this contract as a fixed boundary: separate
explicit costly inventory rebalance, quantity-asymmetric inventory policy,
and flow/adverse-selection response require their own local hypotheses,
activation measurements, controls, and kill criteria.
