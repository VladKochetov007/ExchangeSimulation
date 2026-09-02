# V2-R2-SV1 survival venue-identity correction

Date: 2026-09-02
Candidate: `V2-R2-SV1`
Scope: pre-campaign survival measurement contract

## Reproduction

The original `v2-r2-sv1-survival-side-availability-v2` transformation checked
that the three book summaries were for `central`, `north`, and `south`, and
checked window counts and timestamps. It did not check the venue IDs of the
window groups against those book summaries. A fixture with 23 complete windows
for each of `east`, `west`, and `up`, paired with the valid three registered
book summaries, made all five predicates true.

That is not a valid reconstruction: side availability for one venue cannot
qualify a different venue's book.

## Intended invariant and correction

The registered venue set is `central`, `north`, `south`. The transformation now
receives that set explicitly as `required_venues` and requires:

* book-summary venue IDs equal the registered set;
* window-group venue IDs equal the same registered set;
* every registered venue has exactly the expected consecutive windows; and
* the observed window venue set is retained in the summary for audit.

The scorer passes the registered set explicitly rather than relying on a hidden
filter default. The adversarial contract test now includes a wholesale venue
relabeling mutation in addition to missing, shifted, and unexpected-window
fixtures.

This is an analyzer/measurement-contract correction only. It does not change
the simulator, calendar, participant roster, evidence bytes, historical
results, or holdout policy. No SV1 24-hour result existed that required a
rescore.

## Verification

The valid, missing-window, shifted-window, unexpected-window, and relabeled-
venue fixtures pass with the corrected contract. The correction remains
subject to the fresh exact-tree independent promotion review.
