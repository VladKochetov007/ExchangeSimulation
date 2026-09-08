# SV1C activation and capacity ordering amendment v2

Date: 2026-09-08  
Candidate: V2-R2-SV1C-24H-CDF-LIQUIDITY-STRICT-RISK  
Supersedes: `v2-r2-sv1c-activation-capacity-order-amendment-2026-09-08.md`

The predecessor amendment is retained unchanged as a rejected historical
protocol artifact. This v2 amendment is the only ordering contract for the
SV1C successor.

## Registered sequence

1. validate the clean exact-tree candidate, its configs, and all bound
   contract dependencies;
2. obtain independent Sol-xhigh review of that exact tree;
3. build the registered Go 1.27 provenance-pinned simulator, analyzer,
   renderer, and checkpoint-validator binaries;
4. run the short seed-643 activation probe using those registered binaries;
5. independently extract and review the activation probe;
6. measure capacity using development seed 659 and only the registered SV1C
   treatment/control configurations;
7. only then consider registered development cells 643, 647, 653 and the
   seed-643 parity controls.

The activation probe precedes capacity because it is the cheaper mechanism and
semantic liveness gate. Capacity calibration is not allowed to authorize an
unactivated mechanism; it is a resource measurement after activation evidence
has passed its independent review. The capacity run remains calibration
evidence, not a market result.

Every step must retain the evstream_v3 evidence contract, strict-risk fields,
resource limits, exact config hashes, and complete provenance. No step permits
economic retuning, predecessor reruns, offline trajectory repair, or deletion
of retained evidence.

Holdout seeds 619, 631, and 641 remain outside this namespace until a separate
explicit freeze authorization.
