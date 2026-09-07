# Red-team findings — economic / correctness audit

Append-only. Each finding records its own base revision; later findings may be
measured against a later one.

**Scientific base revision for RT-001 … RT-003:**
`a666d02faede3d40f046b11e60eb672c59386a94`, head of
`feature/r2-cdf-survival-successor`.

**Method.** Invariants are tested against a real run, not a unit fixture,
because reachability is part of the claim. Primary probe: dev-607 / seed 607 /
**7 simulated hours**, chosen because a 20-minute run reaches no funding
interval, no expiry, no settlement and no exercise — its only balance-change
reasons are `trade_settlement`, `initial_deposit`, `borrow` and
`interest_charge`. Seven hours crosses three futures expiries and three option
expiries per venue. No holdout was read or run. No economics were changed.

---

## RT-001 — Expiry settlement bypasses the conservation tracker

**Severity:** high (evidence integrity / detection capability). Not a loss of
funds.

**Reachability:** ordinary cells. Fires in every venue of a standard dev-607
run, from the first futures expiry onward, permanently.

**Invariant.** Every balance mutation is recorded in the conservation tracker
before the logger is consulted. This is stated on `logBalanceChange`
(`exchange/helpers.go:22`): *"Recorded before the log is consulted: a movement
that happens while no logger is attached is still a movement, and leaving it out
of the running total would make the verification depend on the logging
configuration."* `VerifyConservation` then compares recorded movements against
held balances, so that *"a balance changed without a logged movement leaves the
recorded total behind the held total."*

**Observed violation.** `settleExpiredInstrument` (`exchange/expiry.go:640`)
mutates `client.PerpBalances[quote]` and then emits the `balance_change` event
by calling `log.LogEvent` **directly**, constructing the event by hand rather
than calling `logBalanceChange`. The movement therefore reaches the log file but
never reaches `conservation.record`. This is the only such bypass in production
code; every other mutation site uses the helper.

**Reproduction.** dev-607 / seed 607 / 7h. `conservation_violation` events:

| venue | events | first at | gap after 1st / 2nd / 3rd futures expiry |
| --- | ---: | --- | ---: |
| north | 17,999 | t=+7202 s | -2,129,658,925 / -5,078,053,185 / -10,736,084,864 |
| central | 17,999 | t=+7202 s | -5,962,824,577 / -10,600,880,874 / -25,287,737,787 |
| south | 17,999 | t=+7202 s | -4,396,311,208 / -9,820,755,564 / -23,948,562,684 |

The gap is exactly zero before the first settlement, and each step appears one
second after an `instrument_settled` event. The step equals the settled cash
exactly: north's first futures settlement has Σ cashflow `-2,129,658,925` and
north's first gap is `-2,129,658,925`. Same for central and south.

**Economic interpretation: no funds are lost.** This was checked before
proposing a fix, because recording a movement that was genuinely destroying
money would have silenced a true positive. The project's own auditor closes the
accounting identity on the same run:

```
identity USD  external 16072200000000000  internal -345452610503
              exchange 357011627991  open -11559017469  residual 19 (1.18e-15)
expiry: 9 instants, largest net -14686856908 (not required to be zero)
```

Futures expiry cash is not required to net to zero — surviving positions carry
unpaired bases because partial closes already realised their PnL in cash — and
the residual is 19 units in 1.6e16. The defect is that the tracker is not told
about a legitimate, logged movement.

**Consequence.** Worse than a spurious report. The tracker exists so that an
unrecorded mutation is impossible to hide; once the baseline is wrong from the
first expiry, a later genuinely unrecorded mutation is indistinguishable from
the gap settlement already opened. The detector is disabled for the rest of the
run, and it emits 53,997 false violations per 7-hour run.

**Fix.** Record the movement unconditionally, immediately before the logger is
consulted, keeping the logged bytes unchanged. The site cannot simply call
`logBalanceChange`: the settlement record carries `PositionSide`, which the
helper's event does not, and the logged bytes are evidence.

**Semantic impact: none.** Re-running the same cell patched, all 12 non-`general`
log files are byte-identical, and each `general.jsonl` is identical once the
`conservation_violation` lines are removed. `conservation_violation` count goes
17,999 → **0** in every venue. The execution hash changes only because those
spurious events leave the stream.

**Regression test.** `exchange/expiry_conservation_test.go`,
`TestExpirySettlementIsRecordedForConservation`. Fails on `a666d02` with
`Gap: 4000000`; passes patched. The two sides are given different entry prices
deliberately: the tracker compares per-asset totals, so a settlement whose cash
nets to zero across the book moves neither total and hides the omission
completely. That is also why the bug is invisible in a balanced unit fixture and
only surfaced on a real run.

**Recommendation: adopt.** The intended contract is unambiguous and stated in
the code; the fix restores a detector rather than changing economics.

---

## RT-002 — Option expiry net cash is not exactly zero

**Severity:** low. Documented, not fixed.

**Classification:** EDGE CASE / rounding.

**Invariant.** The project's own auditor states it: *"option expiry … must be
zero: payoff does not depend on entry price."* An option's expiry cash is
`MulDiv(size, intrinsicValue(settlementPrice), basePrecision)`, which has no
entry-price term, so the long and short legs of a netted book must cancel.

**Observed violation.** On the 7h run the auditor reports *"option expiry: 9
instants, worst net 6"*. Per-contract sums are in the range ±3 units of 1e-8
USD. Positions net to zero on every contract (`sum(size) = 0` on all 99
settlements audited).

**Mechanism.** The payoff is truncated per position, not per book.
`MulDiv(3, v, p)` is not `3 × MulDiv(1, v, p)`, so one long facing three unit
shorts leaves a residual of a few units. Equal-and-opposite pairs cancel exactly;
unequal aggregations do not.

**Economic interpretation.** A few units of 1e-8 USD per expiry instant. Real,
bounded, and of the same nature as the rounding carry the futures path already
drains deliberately through `CommitPositionAccountingCarry`.

**Fix: none proposed.** Changing it changes payouts, and the choice between
per-position truncation and a book-level carry is a modelling decision. The
futures path resolves the same problem by making the venue the residual
counterparty; whether options should do likewise is the scientific owner's call.

**Recommendation:** owner decides. Flagged because the auditor's own text says
this quantity must be zero and it is not.

---

## RT-003 — Invariants tested and found intact

Recorded so the audit's negative space is explicit.

- **Position netting.** For every derivative contract on every venue, the signed
  positions of all clients sum to exactly zero — 9 perp/futures contracts and 90
  option contracts, 0 with a non-zero net. No phantom counterparty.
- **System accounting identity.** Closes to 19 units in 1.6e16 on USD, and
  exactly on ABC and CDF.
- **Venue take reconstruction.** Fee revenue reconstructs exactly from its
  movement stream for all three assets.
- **Borrow/repay symmetry.** `borrow` nets exactly zero on both ABC and CDF.
- **Spot trade settlement.** Nets exactly zero on ABC.
- **Funding.** Residual ≤ 4 units per instant, with the remainder explicitly
  routed to `funding_remainder` on the venue ledger.

---

## Unaudited

Named so the freeze knows what this report does not cover.

- Liquidation paths: no liquidation occurred in the probe run, so debt
  disappearance, collateral duplication and residual-value routing are untested
  here.
- Cross-venue transfer and latency composition.
- Margin call and insurance-fund draw sequencing.
- Option exercise/assignment against a live underlying position (delta hedge
  accounting).
- Relisting after settlement under the same symbol.
- Negative and zero price domains at settlement.
- The holdout cells, by instruction.

---

## RT-004 — Conservation tracker detects unrecorded mutations only

**Severity:** medium (audit coverage, not an economic defect).
**Classification:** CORRECT BUT SURPRISING. Layer: evidence / detector.
**Base revision:** `a666d02`.

**What was tested.** Seven controlled faults injected into a clean fixture, with
the outcome predicted before running. See E-009 in the research note.

| injected fault | detected |
| --- | --- |
| valid control | no report (correct) |
| unrecorded credit (+7) | yes |
| unrecorded debit (−3) | yes |
| debt silently cancelled | yes |
| one smallest currency unit | yes |
| value paid to the wrong participant | **no** |
| value destroyed but faithfully recorded | **no** |

**Interpretation.** `VerifyConservation` compares per-asset totals of recorded
movements against per-asset totals of holdings. It therefore detects unrecorded
mutations and only those. Nothing in it requires a debit to have a matching
credit, and nothing in it identifies a recipient — so a payment to the wrong
customer preserves every total it checks, and destruction that is faithfully
logged moves both totals together.

**Why this matters for reading other results.** RT-001 was invisible in a
balanced two-party fixture for exactly this reason and only surfaced on a run
where settlement cash did not net to zero. A green tracker is not evidence that
payments reached the right parties.

**Disposition: no code change.** The two blind spots are covered by the identity
check in `research/accounting-audit.md`
(`InternalNet + ExchangeTake + OpenLinearValue = 0`, via
`mvanalyze -metric conservation`). The checks are complementary and neither
subsumes the other. The gap is recorded rather than closed, and the surviving
faults are asserted as surviving in the tests so that a future change to
sensitivity shows up as a failure.

**Regression tests.** `tests/economic_audit_detector_sensitivity_test.go`,
`exchange/economic_audit_recorded_destruction_test.go`.

---

## RT-005 — A bankrupt account's spot wallet is not seized

**Severity:** open pending owner decision. **Classification: NOT ENOUGH
EVIDENCE** to call it either a defect or an intended assumption.
Layer: specification. **Base revision:** `a666d02`.

**Observation.** `liquidate` (`exchange/exchange.go:2242`) resolves a bankrupt
account by zeroing negative *perp* cash and debiting `VenueInsuranceFund` the
same amount. The repay path above it touches only `PerpBalances` and `Borrowed`.
`client.Balances` — the spot wallet — is never consulted. In E-008 the
defaulter keeps 500 USD of spot cash while the fund absorbs the full 100 USD
deficit.

**Economic consequence if unintended.** The insurance fund, and ultimately the
venue, bears a loss that an aggregate-solvent account could have covered. That
is a loss forced onto a party that should not bear it.

**Evidence it may be intended.** `Client.BorrowedSpot` is documented as
splitting a liability by wallet precisely so that "perp equity, liquidation
estimates, and snapshots must not charge a spot-credited loan to the perp
wallet." That reads as deliberate wallet segregation.

**Owner decision required.** If wallets are segregated by design this is an
INTENDED MODEL ASSUMPTION and should be stated as one. If the model claims
cross-margin netting across wallets, this is a real defect. The audit does not
resolve it and has not changed it.

**Reachability.** The transition is reachable in a fixture. It was **not**
exercised in the 7h integration run — no liquidation occurred there at all — so
production reachability on dev cells is unestablished.
