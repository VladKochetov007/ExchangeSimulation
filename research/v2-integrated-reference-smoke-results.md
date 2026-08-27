# Integrated V2 reference compatibility smoke

Status: **PASS for the preregistered compatibility/evidence gate; pending
independent review**

Protocol: `research/v2-integrated-reference-preregistration.md`  
Machine attestation: `research/v2-integrated-reference-smoke-attestation.json`

## Provenance

The clean no-local build used one binary for every parity world:

- source revision: `b312336a3fff3e758ba672137aee0ac917f1bddc`
- `multivenue` SHA-256: `0d3e7c08fdc8d9f7bf892c30381a4583615cb7f0883b519f64f778409022672e`
- `mvanalyze` SHA-256: `7bd1f0620ded51f08ca550fda4f40b567406dc89cbf4285ed46415b2727fc3d2`
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

## Independent evidence checks

- Receipt replay: 8,378 decisions; zero future uses, decision/frontier errors,
  ordinal errors, missing links, or duplicate source identities.
- Frontier replay: 886 vector decisions and 1,772 components; all digest,
  linkage, ordering, and delivery checks valid.
- Post-only: 9,391 accepted post-only orders, 16,452 post-only fills,
  1,992,516,930,524 filled quantity, 75 would-take rejections, and no
  unmatched fill orders. All 3,051 persisted passive quote decisions were
  explicitly `post_only=true` and `cancel_before_replace=true`.
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
  bounded fixed-point residuals (the largest aggregate residual is 262 USD
  units). Settlement and post-expiry-fill checks reported no violations.
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

The result should not be promoted to an integrated long-run candidate until
the independent review in `research/reviews/` accepts this narrow claim.
