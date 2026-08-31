# V2-R2-SV1 CDF liquidity activation probe

Date: 2026-08-31  
Candidate: `V2-R2-SV1-CDF-LIQUIDITY`  
Predecessor: R2, closed as **NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**  
Scientific branch: `feature/r2-cdf-survival-successor`  
Development seed: 607  
Holdouts: not consumed

## Boundary and preregistration

The predecessor result is retained unchanged. This experiment is a separately
named successor and does not reinterpret the R2 24-hour failure. The accepted
intervention is the finite, separately configured CDF/USD roster specified in
`research/v2-r5-cdf-liquidity-successor-preregistration-2026-08-31.md`:

* four suppliers per venue, on three venues; the eight historical ABC/USD
  suppliers are unchanged;
* 1,000 CDF position limit, 0.5 CDF maximum quote, finite initial balances;
* delayed local market-data link, 2-second decision cadence, 60-second
  observation-age limit, and a 4-hour private reference half-life;
* at most one passive GTC/post-only quote, one-sided and inventory-sensitive;
* cancellation/repricing or withdrawal is allowed, with no replenishment or
  two-sided quote obligation.

The mechanism hypothesis is that finite economically motivated CDF supply can
prevent persistent one-sided collapse often enough for the ecology to remain
valuatable. The probe is an activation test, not evidence for that 24-hour
claim.

## Exact provenance

The simulator binary was built from simulator tree `5131ff656708b5a7ec083d41d890e89d994e16c6`
(`5131ff6`), before the analyzer-only correction recorded in `7315f9f`.
The binary was built with `/usr/local/go/bin/go` Go 1.27.0, `CGO_ENABLED=0`,
`-trimpath`, and `-buildvcs=true`.

Binary SHA-256:

```
729622b9eb57fcf76d7476300ae814a6833245f05e80e637c6173650d609672f
```

Treatment and paired-control artifacts were written outside the repository:

* treatment: `/tmp/v2-r2-sv1-activation-607-validated.W5fGaN/treatment`
* control: `/tmp/v2-r2-sv1-activation-607-validated.W5fGaN/control`
* audit: `/tmp/v2-r2-sv1-activation-607-validated.W5fGaN/cdf-liquidity-audit.json`

Both arms ran for exactly five simulated minutes with `GOMAXPROCS=4`,
`log_mode=full`, `evidence_format=evstream_v3`, and strict population
accounting. Both exited successfully. The treatment and control binary
execution-stream attestations were:

| arm | event frames | execution-stream hash |
| --- | ---: | --- |
| treatment | 150,660 | `06e33bce77c1c676eb6014ab918bc9d933c2458f9ac64eaf16a36ab29b07a9ac` |
| control | 148,701 | `39cded1e82f62c358c756251c8505b4f8dc79a9459f97cf2fed15155880f76b9` |

The streams differ as expected because the treatment has a distinct supplier
population. The audit JSON SHA-256 is
`9540ea5b34fad8986f0a82a77f73f2abf9e8c74cfd3fcf40d7c78938536c8637`.

## Independent evidence audit

`cmd/cdf-liquidity-audit` renders each binary stream through the verified
`RenderBinaryEvidence` path, then uses `analysis.OpenRenderedRun` and a typed
streaming audit. Required fields include touch quantities and `is_full`.
`OrderFill.filled_qty` is cumulative; the audit explicitly checks
`previous cumulative + increment = reported cumulative` and reconciles every
supplier fill to an exchange order fill. A mutation test removes `is_full` and
fails closed.

The treatment audit is valid. It measured:

| diagnostic | result |
| --- | ---: |
| registered CDF suppliers | 12 |
| supplier decisions | 1,800 |
| suppliers with fills | 12/12 |
| supplier fills | 111 |
| supplier filled quantity | 2,400,000,000 raw CDF units |
| total CDF trade quantity | 27,043,539,557 raw CDF units |
| supplier share of CDF volume | 8.8746% |
| accepted quotes | 416 |
| completed / horizon-censored quotes | 409 / 7 |
| mean / maximum accepted quote lifetime | 6.438s / 66s |
| mean / maximum observed touch share | 6.816% / 13.032% |
| submit / rest / repricing-cancel decisions | 421 / 963 / 368 |
| withdrawal decisions | 0 |
| suppliers with nonzero account PnL | 12/12 |
| aggregate supplier marked PnL | +203,599,986 raw USD units |
| maximum observed supplier position | ±200,000,000 raw CDF units |
| configured position limit | 100,000,000,000 raw CDF units |
| maximum observed observation age | 1s |

The three venues show opposite inventory/PnL directions, which is useful
evidence against a pure fixed quote script: north suppliers bought and ended
with negative marked PnL, while central and south suppliers sold and ended with
positive marked PnL. All 12 suppliers changed inventory and account equity.
The 368 cancellation actions were specifically `reprice_for_inventory_or_touch`;
no side disappeared after the initial empty-book warm-up, so the run did not
exercise a non-warm-up withdrawal.

The control had zero CDF successor suppliers and a valid independent account
population. Its CDF volume was the same `27,043,539,557` raw units. Bid and ask
absence were each 6/900 snapshots (0.6667%) in both treatment and control;
all six were the initial empty-book observations. This five-minute probe
therefore does not establish a survival improvement.

Terminal strict risk reports were present in both runs. The treatment ended
with two-sided CDF observations after warm-up and no strict-risk error.

## Activation decision

**Activation probe: ACCEPTED, with scope limited to mechanism activation.**

The probe supports these narrow claims:

1. the separately configured finite roster was actually instantiated on all
   three venues;
2. all suppliers traded, changed inventory, bore marked account PnL, and
   repeatedly repriced/cancelled passive quotes;
3. observed activity was far below the declared volume/touch and position
   limits, with no evidence of an infinite replenishment loop;
4. evidence is independently reconstructible and the paired no-roster control
   is valid.

It does **not** support a 24-hour survival claim, a causal reduction in side
   absence, or a claim that the supplier will withdraw under a future
   one-sided/stale-feed stress. Those require the preregistered development
   stress campaign and its kill criteria.

## Red-team and performance-feed status

The latest independently maintained performance ref inspected was
`origin/autoresearch/v2-performance-research` at `c4434ad`. New material in
that feed was the binary-evidence promotion memo and a port/verification of
the F1 risk fix. F1 is already present in the scientific ancestor `e29f26b`;
F2's `continue` behavior and F3's two-pass coherent mark commit are also
already present. No performance implementation was merged. The remaining
binary work is treated as a separate VNext performance feed, not as a change
to this economic intervention.

Next gate: clean full validation, targeted race/static checks, and one fresh
independent review of the exact successor tree. No 24-hour campaign or holdout
execution is authorized by this probe alone.
