# ME-003 prospective review ledger — 2026-09-24

This ledger separates review execution from scientific verdict. It does not
authorize a development world or convert a fixture into an economic result.

| Candidate | Reviewer | Execution | Verdict | Scope |
|---|---|---|---|---|
| `f4075053dd4113303107a3d58a1db50cc32d70ee` | independent fresh Sol-6 medium (`01a0d0b4-db33-78b3-8231-8802d63f66d3`) | COMPLETED | REJECT | ME-003 protocol, instruction adapter, pinned plan, replay and pre-world fixtures; read-only source/design review, no worlds or holdouts |

The reviewer found four concrete blockers before the first development cell:

1. A coherent rehashed above-cap fill/trade could pass replay because the
   locked limit was checked at order send/admission but not execution.
2. IOC/FOK trade, fill and residual cancellation were not required at venue
   arrival, so a delayed execution could pass the order/ledger joins.
3. The fixture set did not demonstrate all protocol boundaries: no reachable
   ask, cap equality and one tick below, stale local versus venue quote,
   fee-plan FOK rejection, reordered response receipts, undefined terminal
   mark, and request/order ID or canonical fill-time corruption.
4. The protocol was still draft; clean tests alone did not lock or release a
   source/protocol candidate. The reviewer did not run worlds or holdouts.

`3012682ae3f33e9eed9e54e8956ffd420ebf747e` is the separately committed
response. It checks cap and venue-arrival time for focal Trade/OrderFill and
IOC cancellation, adds direct invariant and rehashed mutation tests, selected
cap/no-depth and terminal-mark fixtures, an earlier-execution/later-receipt
fixture, and a separate stale-ask venue fixture. The existing venue fee-plan
pruning regression is named in the locked protocol. These repairs have **not**
been accepted by an independent reviewer merely because they were committed.

At this ledger's creation, `3012682` passed a clean full `make test`,
`go vet ./...`, focused instruction tests and `git diff --check`. Targeted
race testing and a new exact-tree prospective review are still in progress.
No ME-003 economic cell has run.
