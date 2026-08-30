# V2-R5 liability-hedger audit hardening

## Trigger

The first current-HEAD `dev-607` extraction reached `liabilityhedger` and was
paused safely after its analyzer grew from about 12.3 GiB to 14.3 GiB RSS in
20 seconds. The raw cell was preserved. Inspection showed that the audit was
retaining order, fill, cancellation, and trade rows for every participant and
instrument even though its contract concerns the three declared
`liability_hedger` participants on `CDF/USD`.

## Hypothesis

An actor-local audit can remain exact and fail-closed while using bounded
memory if it:

1. reads actor decisions and fill evidence only from the venue `general`
   streams;
2. reads exchange lifecycle only from the registered `CDF/USD` book streams;
3. discovers accepted liability order IDs before retaining fills, cancels, and
   trades; and
4. retains only the counterparty order rows named by those retained trades.

The market-data receipt and decision sidecars should also be streamed rather
than materialized as multi-gigabyte byte slices.

## Implementation

`analysis/liability_hedger.go` now enforces those file boundaries, filters
accepted outcomes to declared liability participants, performs a delayed
order-ID join, and preserves acceptance file/ordinal/timestamp metadata. It
rejects payload/envelope identity mismatches, duplicate order identities,
trade-before-acceptance, trades without matching liability fills, wrong-role
fills/cancellations, and actor evidence in book files. Receipt and decision
sidecars are streamed and their second-pass digests are asserted.

## Tests and measurement

The focused liability, option-liability, full, race, vet, and diff-check suites
pass. Adversarial fixtures cover identity spoofing, wrong-role fills,
trade-before-acceptance, duplicate acceptance, missing fills, duplicate
trade/fill identities, fill-before-trade, cancellation-before-acceptance,
swapped maker/taker IDs, malformed counterparty fields, missing configured
participants or books, relocated books, and wrong-file actor evidence.

A diagnostic build of the first scoped implementation held at approximately
212 MiB RSS while its disk-backed receipt audit spill reached 3.2 GiB. That
diagnostic was stopped before completion after independent review found join
integrity gaps; no result from it is used as scientific evidence. The current
implementation addresses those findings, and independent Sol-xhigh review
accepted the complete hardening patch. It still requires a fresh
provenance-bound run before any score is eligible.

## Gate status

The original raw `dev-607` evidence remains retained and is not eligible for
current-HEAD scoring until extraction and verification complete. No holdout
cell was opened.
