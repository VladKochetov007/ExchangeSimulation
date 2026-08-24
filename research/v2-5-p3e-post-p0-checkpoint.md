# V2-5 P3e — post-P0 architectural checkpoint

Status: **P3e P0 scored; signed-price gate independently verified as already
merged.** This checkpoint authorizes preregistration work for a fresh P3e
lifecycle A/B only. It does not alter P3e P0, simulate a new world, or make a
market/economic claim.

## P0 disposition

The only final P0 evidence is the completed B/107 cell at
`research/artifacts/v2-5-p3e/p0-B-107/`, scored in
[`v2-5-p3e-passive-exit-p0-results.md`](v2-5-p3e-passive-exit-p0-results.md).
It is **SUPPORTED (screening)** for the narrow activation/integrity predicate:
two locally evidenced sub-minimum aggressive exits produced two ordinary legal
100,000-unit post-only children with matching gateway, venue, actor, receipt,
accounting, and exact-artifact evidence. P0 is not a closure, cancellation,
funding, basis, profitability, or realism result.

The historical interrupted P0 attempt remains non-evidence. It is neither
compared with nor pooled into the completed cell.

## Signed-price hard gate — verified, not assumed

The repository contains a completed and integrated signed-price migration:

| requirement | concrete evidence |
| --- | --- |
| dedicated migration branch and merge | branch `v2/signed-price`, audit closure `cc91896`, merged into the research line at `320262e` (`merge: signed price contract`) |
| post-merge re-audit of later boundaries | branch `autoresearch/v2-signed-price-hardening`, integrated at `5afdd45`; both `320262e` and `5afdd45` are ancestors of the present head |
| final provenance closure | `7644b2`; also an ancestor of the present head |
| representation / availability / domain contract | [`v2-signed-price-audit.md`](v2-signed-price-audit.md) and [`v2-signed-price-hardening-ledger.md`](v2-signed-price-hardening-ledger.md): signed `int64`, `ErrNoPrice` for absence, explicit `PriceDomain` for admissibility |
| midpoint and matcher boundaries | full-range `types.Midpoint` tests against `math/big`; signed dated-future book, price-time, post-only, IOC/FOK, market, and self-trade fixtures |
| zero settlement wire distinction | nullable `InstrumentAnnouncement.SettlementPrice` plus unavailable/negative/zero/positive round-trip fixtures |
| accounting / lifecycle / ratio domains | signed dated settlement/PnL and crossing-zero fixtures; explicit risk magnitude; funding, BPS, log-return, IV, and ratio domain reports instead of sign-as-absence |
| positive-world equivalence / performance | machine artifact [`artifacts/v2-signed-price-hardening-gate.json`](artifacts/v2-signed-price-hardening-gate.json): identical 2,126,782-event execution/evidence/receipt trajectory at `GOMAXPROCS={1,4}`, unchanged logging semantics, no material allocation/RSS regression |

The hardening gate records `gofmt`, full `go test ./...`, full `go vet ./...`,
source builds, and the scoped race suite as passing. `golangci-lint` is absent
on this host and is explicitly not claimed as passing. The migration's
remaining intentional restrictions are named positive-domain policies
(crypto spot/perp, current funding-ratio models, Black-76/SABR), not implicit
zero sentinels.

## Consequence for the next P3e experiment

No signed-price branch is required before deeper dated carry work. The next
permitted P3e slice is still a **fresh, separately preregistered same-build
lifecycle A/B**. It must fix seed set, population, financial terms, feeds,
clocks, liquidity, term horizon, and evidence contract; vary only the declared
passive-exit policy; and put the passive deadline inside the observable run.

Its endpoint must keep activation, ordinary IOC ineligibility, passive order
attempt/admission, partial fill, deadline cancellation, open residual,
post-close funding attribution, and actual flat closure separate. Historical
P3c is diagnostic only and cannot be the A control. No P3f/P4 extension,
funding/carry treatment, or other simulator-semantic V2-5 change is authorized
until that protocol exists.
