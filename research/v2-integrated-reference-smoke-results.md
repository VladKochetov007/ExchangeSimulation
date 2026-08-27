# Integrated V2 reference compatibility smoke

Status: **PASS for the preregistered compatibility/evidence gate; independently
reviewed and accepted**

Protocol: `research/v2-integrated-reference-preregistration.md`  
Machine attestation: `research/v2-integrated-reference-smoke-attestation.json`

## Provenance

The clean no-local build used one binary for every parity world:

- source revision: `b312336a3fff3e758ba672137aee0ac917f1bddc`
- `multivenue` SHA-256: `0d3e7c08fdc8d9f7bf892c30381a4583615cb7f0883b519f64f778409022672e`
- `mvanalyze` SHA-256: `7bd1f0620ded51f08ca550fda4f40b567406dc89cbf4285ed46415b2727fc3d2`
- refresh-replay analyzer (`bacb7d86cc45c616b0825235919850df6fbb04cc`, clean clone, `vcs.modified=false`) SHA-256: `e00314ce97de6763edd3b45770090a3bb04b243a1841eca9113135a128ce5870`
- `prunegate` SHA-256: `57e38453263d3b8c68f8b70319a745d46490b125f88a822a38bbde429da24d56`
- Go toolchain: `go1.26.6-X:nodwarf5`; clean build metadata

The full input is
`research/configs/v2-integrated/reference-dev-601.json` (SHA-256
`0724f2d022ee9b13ca814cdda67d70b057aa8d5c8b4c33f92cd57a1f52015475`).  The
no-log parity input is the separately documented derived companion
`research/configs/v2-integrated/reference-dev-601-none.json` (SHA-256
`2b43cc846a06dd5ade991592dbc9823e3f4adcf1dcb25add1f44d86a0852ffcb`).

## Execution and evidence reproducibility

All three clean worlds ran for 5 simulated minutes with the same seed 601:

| world | mode | GOMAXPROCS | observations | final execution hash |
|---|---|---:|---:|---|
| `reference-dev-601-full-clean` | full | 4 | 352,099 | `9d40fd652c0c291079332dbe70cfd33bc744f991cdb5da4ca6bdc730803a4e01` |
| `reference-dev-601-none-clean` | none parity | 4 | 352,099 | same |
| `reference-dev-601-none-clean-g1` | none parity | 1 | 352,099 | same |

Checkpoint files, `greeks.json`, and `latency.json` are byte-identical across
the three worlds.  The full run's runtime persisted-evidence artifact and the
independent offline replay agree exactly:

```text
events = 359325
digest = 3dc2ac37e4e0d594eb17fada96cdb29ebc2bc2ad82a2caf714019a55fb5d66d2
domain = persisted_json_records / unordered_multiset
```

The evidence manifest additionally records a SHA-256 and physical record count
for each of the 15 persisted JSONL streams; their counts sum to 359,325. This
per-file inventory makes the offline replay's input set auditable rather than
relying on the aggregate multiset digest alone.

## Independent evidence checks

- Receipt replay: 8,378 decisions; zero future uses, decision/frontier errors,
  ordinal errors, missing links, or duplicate source identities.
- Frontier replay: 886 vector decisions and 1,772 components; all digest,
  linkage, ordering, and delivery checks valid.
- Post-only: 9,391 accepted post-only orders, 16,452 post-only fills,
  1,992,516,930,524 filled quantity, 75 would-take rejections, and no
  unmatched fill orders. All 3,051 persisted passive quote decisions were
  explicitly `post_only=true` and `cancel_before_replace=true`.
- Independent passive-refresh replay (mvanalyze `makerrefresh`) reads the
  physical spot-book streams rather than trusting that declaration. It joins
  6,102 decision sides to 6,017 accepted and 69 rejected venue outcomes,
  checks 5,493 prior-resting sides whose cancellation request precedes the
  replacement outcome, classifies 116 initial/no-prior sides, 477 prior full
  fills, and 16 explicit horizon-censored sides, and reports zero missing,
  duplicate, late, cancellation-order, fill-quantity, or cancel-quantity
  failures. Ordinary IOC/taker orders from selected maker clients are tracked
  as known non-passive orders and excluded from this lifecycle contract. The
  deterministic lineage digest is
  `c0c681b6bd2d6f8de8d4348fe0ca48e6df6e5db1abab4fec114f24a6bbc7b5de`.
- The replay's censor contract is fail-closed: a horizon-censored decision is
  accepted only when every persisted terminal account has the same nonzero
  timestamp and the decision timestamp equals it. Focused mutations for absent,
  zero, and mixed terminal timestamps are rejected.
- CDF inventory rebalance: 180 valid decisions, 39 accepted IOC actions,
  115 fills, and no evidence or receipt mismatches.
- CDF liability hedger: 450 valid decisions, 276 accepted submissions, 505
  fills, and no evidence, receipt, or state-transition mismatches.
- Option liability user: 180 valid decisions, 45 canonical fills, and target
  quantity reached.
- SABR value taker: 174 decisions and 174 fills across three venues.
- Vanna--Volga desk: 84 decisions and 85 fills across three venues.
- Cross-venue router: constructed with three audited links, but zero
  executable signals/submitted groups in this smoke; classified
  **NOT EXERCISED**, not falsified.
- Fill/position reconstruction: 5,504 linear fills matched with zero missing
  or unexpected position updates.
- Conservation: 79,768 delta checks, zero chain breaks/mismatches, and only
  bounded fixed-point residuals (the largest aggregate residual is 262
  fixed-point report units, approximately $0.00262 at precision 100,000).
  The settlement checker was not exercised (66 contracts were listed and none
  settled in five minutes); its `checks:null` result is therefore
  **NOT EXERCISED**, while the post-expiry-fill scan observed no violations.
- Cross-asset CDF collateral borrowing was configured but not exercised in
  this horizon; the evidence contains ABC borrowing only. This is
  **CONFIGURED / NOT EXERCISED**, not a collateral-path validation.
- Strict population ecology: 252 initial and 252 terminal accounts; all role
  rosters remained present. SABR/Vanna--Volga structure is inherited from the
  O4 base and is not evidence of emergence.

## Interpretation boundary

This is a compatibility and evidence-boundary gate for the first integrated
reference composition. It licenses no market-level claim about realism,
funding anchoring, basis convergence, price discovery, or emergence. The
router's zero-action outcome is an activation limitation of this five-minute
smoke. The complete full-evidence run is the scientific evidence source; the
no-log companion exists only to test execution neutrality because the current
validator couples several optional raw JSON rows to full-mode persistence.

The independent review in
`research/reviews/v2-integrated-reference-smoke-independent-review.md` accepts
this narrow compatibility claim. It remains a short smoke and is not a
long-run realism candidate.

## Terminal-time interpretation

Receipt/terminal-account evidence places the simulation terminal fixed point
at `1735689900000000000` (300 simulated seconds after the start). The final
checkpoint row is stamped `1735689960000000000` because the close path emits
the next checkpoint bound while flushing; it carries the terminal execution
hash and is not an additional simulated minute.
