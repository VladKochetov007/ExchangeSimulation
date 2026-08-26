# V2-6 P6-R1 untouched-holdout result

Status: **SUPPORTED (screening)** for the preregistered cross-asset collateral
mark viability and staged option-activation contract.  This is an
out-of-sample replication of the fixed P6-R1 development screen; it is not a
causal option-surface or emergence result.

## Scope and decision rule

P6-R1 repairs the inherited CDF/USD collateral-mark omission with one opt-in
explicit positive CDF/USD accounting mark and a finite CDF borrow cap.  The
mark is used only for collateral authorization/accounting and is not exposed
to actors as a fair-value or information feed.  The holdout policy and
configs were hash-pinned before these worlds were launched: stages O0--O4,
seeds 223, 227 and 229, eight simulated hours, full persisted evidence.

The registered holdout decision is SUPPORT only when every cell has a complete
evidence contract, exercises the CDF borrow path without the historical exact
`CDF/USD` `PRICE_UNAVAILABLE` failure, and passes the stage-specific
activation gates.  A zero-trigger stage would remain NOT EXERCISED.

## Provenance and integrity

All fifteen worlds use simulator revision
`ca12843f69c2c8f0aae802aef3b77992c76c5512`, multivenue binary SHA-256
`edb2eece46ba0cffd0a3e94f3a83009a8f1e1755f213163c945d986d093ad0be`, and
the same analyzer SHA-256
`a17e40e7b548a824bf44808a80c8a6a46386d206d10f820237d352c9ed4d65d6`.
Extraction metadata records analysis revision
`ca12843f69c2c8f0aae802aef3b77992c76c5512` and contract
`v2-6-p6r1-cross-asset-mark-v1`.  Config hashes, exact persisted evidence
event counts/digests, and all stage metrics are in
[`artifacts/v2-6-p6r1/holdout-summary.json`](artifacts/v2-6-p6r1/holdout-summary.json).

The independent read-only checker
[`scripts/check-v2-6-p6r1-holdouts.sh`](../scripts/check-v2-6-p6r1-holdouts.sh)
returned success for all fifteen cells.  Every cell has final `greeks.json`
and `latency.json` sentinels, all 19 required metric artifacts, complete
analysis metadata, valid observation-receipt/frontier reconstruction,
conservation, positions/fills, order lifecycle, settlement/expiry, stream and
evidence hashes.  Runtime and offline persisted-evidence counts and digests
agree exactly in every cell.  Raw evidence remains retained; this protocol
has no prune authority.

## Holdout cell ledger

`CDF B` is the persisted count of CDF borrow records; `PU` is the exact count
of persisted `CDF/USD` `PRICE_UNAVAILABLE` rejections.  `surface` is
market-price option trades / independently priced observations / reconstructed
surface points.  Liability, value-taker and VV columns are decisions / fills /
filled raw quantity.  Liability is intentionally inactive in O0/O1.

| cell | evidence events | persisted-evidence digest | CDF B | PU | surface | liability | value taker | VV |
|---|---:|---|---:|---:|---:|---:|---:|---:|
| O0-223 | 30,330,262 | `ab20c13422a19098ed05ac60d07f8d6c97f87cf4a7272cdb10914635894525d2` | 17,946 | 0 | 160,279 / 73,533 / 150 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| O0-227 | 30,804,178 | `19cde458e93cbf32b3692870019da74724c1761fd0ba7c70813f3d4209849b90` | 18,423 | 0 | 161,136 / 73,474 / 150 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| O0-229 | 29,224,683 | `e4f1809f9e2c7e0ad87837c0e76b1189290b7b43d882aec0089c151afc25b1bf` | 10,923 | 0 | 161,108 / 73,200 / 150 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| O1-223 | 33,979,011 | `73037868cfcf1d48f2e93ad971db49841f52f263f89c947407e6052e57c62d21` | 17,895 | 0 | 242,564 / 154,656 / 150 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| O1-227 | 34,173,797 | `5077df707622c96d6e88d2dfe1706be8ec768a78fe11edb921ccd15e3e63b776` | 18,450 | 0 | 241,226 / 153,917 / 150 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| O1-229 | 34,461,132 | `018df0796e380788f58de5e38f6416faa12d1ac71179d0f1a615171b347f369f` | 18,211 | 0 | 242,397 / 153,711 / 150 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| O2-223 | 34,294,095 | `14feb1c9a0605c779ad2db9aea932d041fab3f12654eabf40ec43fd86c26a3fc` | 16,883 | 0 | 243,196 / 155,200 / 150 | 17,280 / 49 / 300,000,000 | 0 / 0 / 0 | 0 / 0 / 0 |
| O2-227 | 34,267,192 | `cbd3549bb609855558d26b753282b1c802fac2c7af3617da5740a7a303fe591b` | 18,438 | 0 | 244,188 / 154,694 / 150 | 17,280 / 44 / 300,000,000 | 0 / 0 / 0 | 0 / 0 / 0 |
| O2-229 | 34,075,257 | `52597ba7a3c63d1f82b2c7735402e3a812e21a1f11951216e6f8c8d3a21f7cd8` | 16,126 | 0 | 247,628 / 154,389 / 150 | 17,280 / 45 / 300,000,000 | 0 / 0 / 0 | 0 / 0 / 0 |
| O3-223 | 33,560,572 | `085ca6d2cb2981d1a925503769cfcf5d6bc6a126dac60a31c0cb6813d198e791` | 18,381 | 0 | 262,480 / 177,880 / 150 | 17,280 / 49 / 300,000,000 | 11,000 / 16,943 / 10,776,000,000 | 0 / 0 / 0 |
| O3-227 | 33,979,888 | `990498c4c015f3235999ae39edbc474404420f96e15ed78841664dfd827b0912` | 17,557 | 0 | 260,644 / 178,082 / 150 | 17,280 / 43 / 300,000,000 | 11,113 / 17,270 / 10,865,000,000 | 0 / 0 / 0 |
| O3-229 | 34,787,825 | `b3c9a74dcb2064da7059ac772f4559b585e924867382e541dde6b506778921ef` | 18,942 | 0 | 254,256 / 173,556 / 150 | 17,280 / 45 / 300,000,000 | 10,909 / 16,565 / 10,671,000,000 | 0 / 0 / 0 |
| O4-223 | 35,034,582 | `c51439fb11e469fc31b7a8805809484620fb06d7b070d48849475f68ec9b8ecb` | 20,315 | 0 | 261,779 / 178,157 / 150 | 17,280 / 48 / 300,000,000 | 10,862 / 16,405 / 10,688,000,000 | 5,915 / 7,093 / 27,945,000,000 |
| O4-227 | 32,072,029 | `49eeb9a622f29d7e92040940fa985128a9ed6b224ee6ff4adc4ac2b7381b269c` | 6,859 | 0 | 265,756 / 178,823 / 150 | 17,280 / 44 / 300,000,000 | 10,941 / 16,999 / 10,729,000,000 | 5,849 / 6,930 / 27,335,000,000 |
| O4-229 | 34,650,407 | `1c430d9f74518e96a89a36b06d5b057ef575a1160186c9bf630f086fe052528d` | 18,647 | 0 | 266,950 / 179,964 / 150 | 17,280 / 45 / 300,000,000 | 10,826 / 16,406 / 10,630,000,000 | 5,898 / 7,223 / 28,805,000,000 |

## Interpretation

* **Cross-asset mark viability:** all 15 holdouts exercised CDF borrowing
  (6,859--20,315 records) and produced zero exact historical unavailable-price
  rejections.  This validates the repaired collateral path out of sample at
  screening level; activity is not profitability or basis evidence.
* **O0/O1:** every cell independently reconstructs 150 option listings, 90
  option settlements, zero post-expiry fills and 150 surface points with
  positive market-priced observations.  The flat/reference stages therefore
  remain viable under untouched seeds.
* **O2:** all three cells execute the liability/delta-hedge activation (17,280
  decisions, 44--49 canonical fills, declared 300,000,000 raw quantity) with
  non-zero independently reconstructed dealer exposure and hedge flow.  No
  directional transmission or surface claim is registered here.
* **O3:** all three cells have active minority SABR-view users (10,826--11,113
  decisions and 16,565--17,270 fills) while the option chain remains active.
  Any O3-vs-O2 surface difference is inherited SABR-prior structure.
* **O4:** all three cells have active Vanna--Volga risk-transfer activity
  (5,849--5,915 decisions and 6,930--7,223 fills) alongside liability and
  value-taker activity.  VV-induced structure is likewise explicitly
  prior-driven.

The holdout result licenses no claim that option smile/skew, hedge feedback or
relative-value effects are emergent.  It only establishes that the repaired
P6-R1 stages and evidence contract remain executable on untouched seeds.  A
future causal surface experiment must compare stages under a newly frozen
paired design and continue to infer IV from market prices only.

