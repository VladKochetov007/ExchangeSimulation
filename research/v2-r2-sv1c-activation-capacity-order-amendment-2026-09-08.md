# SV1C activation and capacity ordering amendment

Date: 2026-09-08
Candidate: V2-R2-SV1C-24H-CDF-LIQUIDITY-STRICT-RISK

The SV1C order is immutable:

1. validate the clean exact-tree candidate and its configs;
2. obtain independent Sol-xhigh review;
3. build Go 1.27 provenance-pinned simulator, analyzer, renderer, and
   checkpoint validator binaries;
4. measure capacity using development seed 659 and the registered SV1C
   treatment/control configurations only;
5. run the short seed-643 activation probe;
6. independently extract and review that probe;
7. only then consider registered development cells 643, 647, 653 and the
   seed-643 parity controls.

The capacity run is calibration evidence, not a market result. It must retain
the binary evidence contract, strict-risk fields, resource limits, and exact
config hashes. Holdout seeds 619, 631, and 641 remain outside this namespace
until a separate explicit freeze authorization.
