# V2-5 P0 replacement activation result

Status: **PASS — evidence/activation gate only.** This is not a causal result
for funding, carry, basis convergence, profitability, or realism.

The final machine-readable result is
[`activation-r1-101/p0-verdict.json`](artifacts/v2-5-p0/activation-r1-101/p0-verdict.json).
Raw logs and V2 receipts remain retained at the same path.

## Provenance

| field | value |
| --- | --- |
| config / seed / horizon | `configs/v2-5-p0/activation-r1-101.json` / 101 / 5 minutes |
| simulator source at run | `78c0a2a552772153daf947867000b0093d2da584` |
| terminal events | 56,415 |
| ordered execution hash | `feef18193965147a02437b935af3a2fdc9ceabe889f86dcef0b81ac00a6fa5ec` |
| persisted-evidence digest | 56,924 records / `fed0769291e48161e2f75f5625ac49a41fce30da646525dc7496751c3b8489a8` |

The execution hash equals invalid attempt 0 exactly. That is expected and
desired: the P0 correction is append-only provenance instrumentation and has
no simulated-world effect.

## Independent checks

- V2 receipt audit: **valid** — 2,679 schedules, 2,670 actual deliveries, 27
  scalar requests; all three sidecar digests match, with zero bad frontiers,
  future decisions, or missing due receipts.
- Funding-carry replay: **valid** — 450 policy evaluations; 444 funding and
  894 book cache identities independently located in their declared decision
  frontier prefixes; zero source misses, mismatches, future use, arithmetic
  mismatches, gateway mismatches, or actor-outcome mismatches.
- Activation: 27 submitted non-atomic legs, all accepted, with 32 fills. The
  action trace includes 11 target spot legs and 16 explicit perp orphan-repair
  legs, so both policy legs are observed rather than assumed atomic.
- Deferred states are preserved rather than silently ignored: 288
  `ZERO_PREMIUM`, 3 `FUNDING_UNAVAILABLE`, 3 `NET_CARRY_BELOW_MINIMUM`, and 6
  terminal-horizon censored evaluations.
- Terminal delta reconstruction checked 3,145 records with zero mismatches and
  zero broken chains. Aggregate ABC residual is zero; USD residual is −5 quote
  units, the bounded integer truncation residual recorded by the conservation
  audit.

## Conclusion and next gate

The P0 claim supported by this one seed is narrow: the opt-in desk consumes
delivered public funding and local books, calculates its declared signed carry
components, and submits independently attested non-atomic orders when its
fixed activation policy permits. It does **not** establish that funding anchors
the perpetual, that carry is profitable, or that any basis effect is emergent.

The next allowable work is a separately preregistered P1 funding/carry causal
screen with paired controls, named nonzero cost/risk assumptions, activation
criteria for inventory and actual legs, and no post-hoc change to the P0 cell.
