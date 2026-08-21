# Accounting and conservation audit

Measurements below are from runs of the pre-re-freeze binary unless marked
otherwise; they are being repeated on the re-frozen baseline (`01c9ceb11aa1`).
Where a number changes, this document is corrected rather than appended to.

## The identities

For a market that creates nothing, everything a participant holds came from
outside it or from another participant. Per asset:

    InternalNet + ExchangeTake + OpenLinearValue = 0

External deposits and borrowing are the source of every holding and are not
part of the zero-sum; they appear only as the scale a residual is read against.
The earlier statement of this identity wrongly included them, which made it
off by the entire external float.

- **ExternalIn** — deposits and borrowing, the only legitimate creation, and
  the scale the residual is judged against. Borrowing enters once: a borrow
  logs the cash it credits and the debt it creates, and counting both doubles
  it. Against gross internal turnover rather than the external float, the same
  residual reads 7e-7 rather than 4.6e-10, and both figures are reported for
  that reason.
- **InternalNet** — every other logged balance movement, summed over
  participants: trades, option premium, funding, settlement, interest.
- **ExchangeTake** — what the venue itself holds: fee revenue and the insurance
  fund. A fund driven negative by a bankruptcy enters here with its sign.
- **OpenLinearValue** — the unrealised profit of positions nobody has closed,
  which is cash that has not been paid yet.

Option positions are deliberately excluded from the open value. Their reported
unrealised figure is a Black-76 mark: a model's opinion, not a claim on
anybody's cash. Folding it in makes the identity untestable, since it can then
be satisfied by adjusting a volatility.

Per contract, and independently of any of the above:

    Σ position size = 0

Per funding instant, per venue:

    Σ funding payments = 0     (up to one unit of truncation per account)

Per option at expiry:

    payout(holder) = intrinsic × position,  and  Σ payout = 0

Per dated future at expiry:

    payout(holder) = (settlement − entry) × position

which does **not** sum to zero: it sums to the settlement price times open
interest, which is zero, minus the entry-weighted size, which is not.

## Results

| identity | result |
|---|---|
| Closed system, ABC | residual exactly 0 |
| Closed system, CDF | residual exactly 0 |
| Closed system, USD | residual 7,594,763 units on an external float of 1.65e16 (4.6e-10) |
| Zero net supply, every contract | holds exactly, from position updates alone |
| Spot books, base asset | nets exactly 0 per book |
| Spot books, quote asset | nets exactly minus the venue's logged fee revenue |
| Fee ledger against fee-revenue stream | **they do not agree.** The venues hold 562,254 ABC and 17,232,038 USD more than the fee events account for; CDF agrees exactly. The gap is exactly the interest charged (17,232,067 USD) minus the funding remainder paid out (29), both of which increment revenue without emitting a fee event |
| Independent position reconstruction against the report | unrealised value gap exactly 0 |
| Dated settlement, 15 contracts | payout residual 0, every holder paid, no fill after expiry |
| Funding, 17 instants | all within the per-account truncation bound; direction consistent with the published rate |
| Option exercise, 150 expiries, ~2,100 holder-level payouts | none mispaid, none paid while worthless |
| Balance changes self-consistent | 14,991,128 deltas checked on the twelve-hour run, 0 whose reported delta disagrees with their own before and after, 0 decode failures |
| Movements reconstruct the reported final holdings | 735 of 735 accounts exactly. This is the only check covering a balance changed without a logged record, which the closed-system identity cannot see |

## The USD residual

Pre-registered as a test rather than explained after the fact. The prediction
was that if the residual is integer truncation, it stays small against the
number of truncating operations and does not grow with run length; the kill
criterion was ten units per derivative balance change.

| run length | residual | derivative records | units per record |
|---|---|---|---|
| 6 hours | −1,001,561 | 719,310 | −1.39 |
| 12 hours | −1,772,400 | 1,581,481 | −1.12 |

Both inside the bound and not growing. The residual is rounding.

Its sign is mixed across venues (+1,749,567 at central, −1,212,958 at north,
+7,058,154 at south on the 24-hour run), which is what truncation toward zero
produces when profits and losses are both truncated, and not what a systematic
leak produces.

## What this audit does not test

An independent critique of the accounting made the point sharply and it is
recorded here rather than answered away: these identities test whether the cash
movements are **consistent and completely logged**, not whether they are
**correct**. A mechanism that overcharges a participant and routes the excess
to the venue satisfies every identity above to the unit, because the debit and
the credit are the same number. The fee gap in the table is exactly that shape
— revenue taken with no fee event — and it was invisible until the venue's take
was checked against an independent stream.

Specifically untested:

- fee rates against what the fee schedule says they should be;
- the funding remainder routed to the venue, which has no event of its own;
- borrow and repay, since nothing in any audited run ever repays — 1,229
  borrows and zero repayments in 24 hours;
- liquidation and bankruptcy, which never fire (V-005);
- isolated-margin collateral, which logs one leg of two and is inert only
  because the population runs cross margin.

## Whether the audit can see anything

An audit that never fires proves nothing. A build that credits one thousand
extra units on roughly a thousandth of settlements takes the ABC and CDF
residuals from exactly zero to 41,726,000 and 24,702,000 respectively, and the
USD residual from 4,248 to 31,624,727. The mutation was reverted before any
recorded measurement; it exists to show the identity binds.

That test proves the audit catches value created inside a settlement. It does
not prove it would catch a leak that moves value between two participants
symmetrically, or one that hides inside the open-position term. Those remain
unprobed.
