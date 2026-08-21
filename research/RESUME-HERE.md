# Resume the validation campaign

The session that wrote this lost its shell — every command, including `true`,
returned exit 1 — so the measurement half of the suite is specified but not
executed. Nothing here needs re-deriving; it needs running.

Start by confirming the shell works and the tree is clean:

    cd /home/vlad/development/exchange_simulation
    git log --oneline -3
    git status --porcelain

If `research/` and `scratch/` show uncommitted files, commit them first:

    git add -A research scratch
    git commit -m "docs(research): ecology audit, frozen stylized-fact list, ablation pre-registrations and machine-readable summary"

## Rules that must not be broken

- **Never edit `research/configs/frozen-baseline-2026-08-21.json`.** Every arm
  is a copy with one named delta. Editing the baseline in response to a
  measurement turns validation into calibration.
- **Every run is pinned to one core.** `scratch/parallel_runs.sh` sets
  `GOMAXPROCS=1`; multi-core runs of this configuration diverge (V-008).
- **The script refuses to launch a stale binary.** If it does, rebuild:
  `go build -o bin/multivenue ./cmd/multivenue && go build -o bin/mvanalyze ./cmd/mvanalyze`.
- **Pre-registration is already written.** Do not revise a prediction or a kill
  criterion after seeing a result; record the outcome against what was written.

## 1. Deterministic baseline (blocks everything else)

    nohup bash scratch/parallel_runs.sh scratch/jobs_det.txt 3 80 > scratch/jobs_det.out 2>&1 &

Three seeds, 24 simulated hours each, roughly 40–60 minutes apiece pinned to
one core. Poll with `tail -1 scratch/det_10*.log` until all three say `done`.

Then confirm reproducibility actually holds now:

    for i in 1 2; do GOMAXPROCS=1 ./bin/multivenue -config research/configs/frozen-baseline-2026-08-21.json \
      -seed 101 -duration 30m -logdir /tmp/rep_$i >/dev/null 2>&1; done
    md5sum /tmp/rep_1/greeks.json /tmp/rep_2/greeks.json

Identical hashes are the precondition for every paired comparison below.

## 2. Repeat the audits on the deterministic runs

    for m in conservation positions settlements derivatives arbitrage roleaudit lifecycle hedging; do
      ./bin/mvanalyze -metric $m -json logs/det_101 > research/artifacts/scoreboard/det_101/$m.json
    done

Compare against the numbers in `research/artifacts/validation-summary.json`.
Anything that moves by more than the noise recorded there (0.03% on the primary
metric) is a finding about the re-freeze, not about the market.

Then rewrite the tables in `research/economic-ecology-audit.md` from
`roleaudit` on these runs; the ones there now are from pre-re-freeze runs and
are marked provisional.

## 3. The nine pre-registered ablations

    python3 scratch/build_ablation_arms.py
    nohup bash scratch/parallel_runs.sh scratch/jobs_ablations.txt 4 80 > scratch/jobs_ablations.out 2>&1 &

Eighteen runs (nine arms, two seeds). Predictions and kill criteria are in
`research/causal-ablations.md`; record each outcome against what is written
there, including the arms that change nothing — the triangular-arbitrage arm is
a deliberate null control and a large effect there would falsify V-001's
account of that desk.

The two that matter most:

- `abl-own-mid-anchor` decides V-004: whether cross-venue agreement comes from
  the shared index or from anything else.
- `abl-option-value-takers-off` decides section 10: whether the option smile is
  inherited from SABR priors or produced by dealer inventory and hedging.

## 4. Stress, and only then the liquidation ablation

    GOMAXPROCS=1 ./bin/multivenue -config research/configs/v005-stress-perp.json -seed 101 -duration 24h -logdir logs/stress_101

Count `liquidation` and `liquidation_deficit` events. If any fire, audit the
ledger across them (`-metric conservation`) and check the insurance fund
carries the deficit. If none fire even here, V-005 strengthens: the population
cannot be stressed by its own flow.

## 5. Stylized facts

Only after 1–4. The list is frozen in `research/stylized-facts-baseline.md`,
along with five facts already known to be uninterpretable. Measure, record, and
do not adjust the population in response.

## 6. Still open

- **V-008 root cause.** Pinning to one core is a workaround. What is known: not
  a data race (detector clean), not a scheduler tie-break (orders by time then
  insertion sequence), not concurrent goroutines (a dump during a run shows one
  simulation goroutine), and it disappears when latency profiles are removed.
  The next step is to log every `Schedule` call's time, id and caller for the
  first few thousand events across two runs and diff them.
- **Clock-artifact sweep** (section 13), untouched.
- **Broader mutation suite** (section 15): reversed funding sign, duplicated
  fill, violated price-time priority, wrong Black-76 delta sign, swapped
  call/put payoff. Each needs a named invariant that should fail; if it passes,
  the audit is too weak and must be strengthened.
