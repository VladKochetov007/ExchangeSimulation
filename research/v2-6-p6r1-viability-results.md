# V2-6 P6-R1 cross-asset-mark viability result

Status: **SUPPORTED (screening) for the registered viability and staged
activation contract.**  This result repairs the inherited CDF/USD collateral
mark omission.  It is not an option-surface causal result and does not claim
that any smile, skew, or risk-transfer effect is emergent.

## Scope and decision rule

P6-R1 was preregistered in
[`v2-6-p6r1-cross-asset-mark-viability-preregistration.md`](v2-6-p6r1-cross-asset-mark-viability-preregistration.md).
The sole simulator delta was the opt-in explicit positive CDF/USD collateral
mark plus a finite CDF borrow cap.  No actor receives that oracle and no
fair-value, quote, clock, feed, or option parameter changed.  Development was
O0--O4 at seeds 211 and 213, eight simulated hours, full persisted evidence.

The registered viability classification is SUPPORTED (screening) when both
development seeds complete the evidence contract, the explicit CDF collateral
path is exercised, and each stage's activation gates pass.  A zero-trigger
stage is not silently treated as a pass.  Holdout seeds were not consumed in
this screen.

## Provenance

The ten worlds were completed with simulator revision
`bf4927b1e02f2671ee4c365ce1f4a5b5a100f987` and multivenue binary SHA-256
`edb2eece46ba0cffd0a3e94f3a83009a8f1e1755f213163c945d986d093ad0be`.
Extraction used analysis revision
`706965d6473a6afc772d2499dac024ec97b0132e` and mvanalyze SHA-256
`a17e40e7b548a824bf44808a80c8a6a46386d206d10f820237d352c9ed4d65d6`.
The immutable configuration hashes, event counts, persisted-evidence digests,
and activation measurements are machine-readable in
[`research/artifacts/v2-6-p6r1/development-summary.json`](artifacts/v2-6-p6r1/development-summary.json).

Every cell has launcher status 0, final `greeks.json` and `latency.json`
sentinels, all registered metric artifacts, complete `analysis-metadata.json`,
passing receipt/frontier, conservation, position/fill, lifecycle, settlement,
expiry, and strict contract checks.  Runtime and offline persisted-evidence
event counts and digests agree in every cell.  Raw evidence remains retained;
no prune operation was authorized by this extractor.

## Cell ledger

`CDF B` is the persisted count of CDF borrow events and `PU rejects` is the
count of exact `CDF/USD` `PRICE_UNAVAILABLE` rejections.  `surface` is
market-price option trades / independently priced observations / surface
points.  Liability, value-taker, and VV columns are decisions / accepted or
canonical fills / filled raw quantity; a zero row is the registered inactive
population for that stage.

| cell | evidence events | persisted-evidence digest | CDF B | PU rejects | surface | liability | value taker | VV |
|---|---:|---|---:|---:|---:|---:|---:|---:|
| O0-211 | 30,439,762 | `580462da90ab09611f68b2678c6dd67e4a49da686d2541a52bdca67a60d151f7` | 21,722 | 0 | 161,168 / 73,830 / 150 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| O0-213 | 30,429,748 | `a95bb6906d0803c74b67f6fcf11dc4029581f5ae1afd8cf8523c828b1a540f3d` | 21,010 | 0 | 161,125 / 72,590 / 150 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| O1-211 | 33,587,003 | `a6ae0b595abd482236279cd6c04f22acc53901673a11f5f491e3173ea586abe3` | 21,508 | 0 | 247,406 / 154,488 / 150 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| O1-213 | 35,169,307 | `86ce0a89c7f8ac8ef8f7c55259dc70ee5114be98f0ccd3568eb375075f3a589e` | 18,248 | 0 | 250,752 / 156,394 / 150 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| O2-211 | 33,921,824 | `530d45392506de7d55e28fd9828943117f05558cd2eff199c245f5fb0517bfe7` | 21,115 | 0 | 245,958 / 154,230 / 150 | 17,280 / 64 / 300,000,000 | 0 / 0 / 0 | 0 / 0 / 0 |
| O2-213 | 34,696,592 | `d7347f221498b940f3bfcc982cb993f02f07f429828f0822aabdc68228bf8772` | 19,568 | 0 | 243,781 / 153,624 / 150 | 17,280 / 59 / 300,000,000 | 0 / 0 / 0 | 0 / 0 / 0 |
| O3-211 | 34,702,818 | `e5d03246b5a62c4ae47aeb0d39c8f7c880c9b95fdf80e5f05018d5365877383d` | 19,730 | 0 | 260,227 / 176,523 / 150 | 17,280 / 64 / 300,000,000 | 10,937 / 16,915 / 10,708,000,000 | 0 / 0 / 0 |
| O3-213 | 35,296,682 | `173e38db811cfb9d11e8f981d114923fbc91ecabd724cbedd6754aeb25a3bae4` | 15,220 | 0 | 257,658 / 176,730 / 150 | 17,280 / 59 / 300,000,000 | 10,939 / 16,624 / 10,721,000,000 | 0 / 0 / 0 |
| O4-211 | 33,747,463 | `657247a56fd14acec01e3344e865dac903a4fa8ad97ae94cacbc267b2c6d42f9` | 9,999 | 0 | 266,908 / 179,849 / 150 | 17,280 / 64 / 300,000,000 | 11,139 / 16,460 / 10,982,000,000 | 6,177 / 7,569 / 30,455,000,000 |
| O4-213 | 32,851,287 | `776c0e90c3af1117601bfc6e62b933f95e06f28e71804990ab551d97a1726bbd` | 8,460 | 0 | 271,911 / 181,470 / 150 | 17,280 / 60 / 300,000,000 | 10,913 / 16,885 / 10,719,000,000 | 5,967 / 6,871 / 27,695,000,000 |

All ten configs passed `scripts/check-v2-6-p6r1-configs.sh`.  Their SHA-256
values are retained in the JSON artifact and in each cell's run metadata.

## Activation and integrity interpretation

* **Cross-asset viability:** every cell exercised CDF borrowing (8,460 to
  21,722 persisted borrow records) and had zero exact historical
  `PRICE_UNAVAILABLE` CDF/USD rejections.  The repaired path is therefore
  exercised in both seeds and every stage.  The count is an activity measure,
  not a profitability or basis claim.
* **O0/O1:** all cells independently reconstruct 150 option listings and 90
  option settlements, have zero post-expiry fills, 150 surface points, and
  positive market-priced observations.  Option-dealer maker fills are
  161,125--161,168 in O0 and 247,406--250,752 in O1.  These are activation and
  quote/liquidity gates only.
* **O2:** both liability users make 17,280 local decisions, reach the
  registered 300,000,000 raw-unit target, and have 43/44 canonical
  fill-linked orders.  Dealer exposure reconstruction reports non-zero option
  inventory and hedge-tagged underlying flow in both seeds (39,360,052,961 and
  39,417,691,180 raw units respectively).  The P6 transmission sign was not
  preregistered, so no directional causal claim is made here.
* **O3:** both seeds have an active three-participant SABR value-taker path,
  10,913--10,939 decisions, and 16,624--16,915 canonical fills while the
  chain remains active.  Any surface contrast involving O3 includes an
  explicit SABR prior and is inherited structure.
* **O4:** both seeds have an active VV path, 5,967--6,177 decisions and
  6,871--7,569 canonical fills, with active liability, value-taker, dealer,
  and market-price option evidence.  Any VV surface contrast is descriptive
  prior-driven structure, not emergence.

The evidence contract is valid in every cell.  The repaired mark is used only
for collateral authorization/accounting; it is not a participant observation
or a hidden global price.  The result therefore supports the narrow R1
viability/activation proposition at screening level.  It does not retroactively
upgrade the incomplete original P6 O3/O4 worlds and does not by itself license
O3-O2 or O4-O3 causal surface comparisons.

## Holdout decision

The complete paired development screen now satisfies the preregistered
activation and integrity gates for every stage.  This authorizes the already
fixed untouched holdout policy (seeds 223, 227, and 229) under the same R1
configs and horizon.  Holdout configs must be generated and hash-pinned before
launch; no stage or seed may be selected after seeing these results.  Until
those holdouts complete, R1 remains a development-screen result and no
out-of-sample options claim is made.

