# Binary evidence format: promotion memo

Branch `autoresearch/v2-performance-research`. Nothing here is merged into the
scientific branch, and no performance commit changed scientific semantics.

**Recommendation: KEEP EXPERIMENTAL.** The mechanism is independently confirmed
and the economics are strong. What is not yet done is the adoption work, and one
adoption question is the user's to answer rather than mine.

## 1. Contract and tier

**Question.** Should JSON be retired from the simulator's evidence path in
favour of a canonical binary event stream?

**Tier A.** A deterministic simulator with an exact oracle: identical seed and
config must reproduce an identical ordered execution hash. A change that alters
behaviour fails outright rather than scoring worse.

**Model-to-reality gap.** The oracle proves trajectory identity, not benefit.
Every performance claim rests on wall-clock measurement, and this host's A/A
control ranged from 0.06 % to 5.52 % across the campaign. Every accepted number
carries its own A/A measured in the same session.

**Falsifiers, stated in advance and all survived.** A codec that changes the
execution hash; a rendered corpus that differs from the JSON one; a determinism
failure across thread counts; an A/A control exceeding the measured effect.

## 2. Claims, and who verified them

| claim | result | verified by |
| --- | --- | --- |
| wall time, `log_mode: full`, replace mode | **-20.02 %** paired, 24/24 rounds, A/A -0.89 % | **independent grader, own harness** |
| same, on real storage rather than tmpfs | **-21.64 %**, 8/8 rounds, A/A -0.10 % | independent grader |
| disk | 594,722,408 → 272,576,389 (**2.18x**), → 124,174,797 zstd (**4.79x**) | independent grader, to the byte |
| evidence corpus rebuilds from binary alone | **1,597,303 records bit-identical** | independent grader, stronger test than claimed |
| per-file line order preserved | 15 files, 0 violations | independent grader |
| execution hash independent of codec | four codecs, identical digest | self, test-pinned |
| determinism across GOMAXPROCS 2/4/8 | byte-identical streams | self, and an earlier grader at 1 h |
| RSS | unchanged (+0.55 %) | independent grader |

The corpus result proves more than losslessness: both arms produce the same
complete record multiset, so **replace mode does not perturb the trajectory**.

## 3. The correction that reframed the campaign

Every number published for most of this campaign came from `log_mode: none`.
**Six of the seven registered configs use `log_mode: full`.** In that regime the
candidate was **+8.18 %** — slower than the thing it replaces — because the sink
returned no reusable bytes and the venue logger marshalled every payload twice.

A reviewer predicted this in one sentence. It was filed as a known "design gap"
and not tested for days. Testing took twenty minutes and moved the result from a
regression to a larger win than the original claim.

## 4. Defects found by review, all fixed

| defect | severity | found by |
| --- | --- | --- |
| binary runs wrote `event_count: 0` and an all-zero hash, so any two runs compared identical in the tool built to tell runs apart | **critical** | grader 1 |
| an unencodable payload truncated the stream, recording 1 of 102 events | high | grader 1 |
| encoding depended on how the caller boxed a value | high | grader 1 |
| stream had no terminator, so truncation at a block boundary read as a valid shorter run | high | self, via new format tests |
| `os.Getenv` per event on the hottest path | moderate | grader 2 |

Every one is regression-tested, and each test was verified to fail against the
unfixed code.

## 5. What blocks promotion

1. **File-layout routing.** `cmd/evsrender` emits one globally ordered stream and
   does not reproduce the `venues/<v>/spot/<sym>.jsonl` directory shape, because
   routing lives in the logger tree rather than in the events. Any analyzer that
   opens a file by path needs that rebuilt or needs changing to read the stream.
   **This is buildable and is the main remaining work.**
2. **Re-baselining.** A binary digest covers different canonical bytes than a
   JSON one, so adopting the format re-establishes every registered execution
   hash. Tagged `representation: evstream_v1` so the two can never be confused
   silently. **This is a scientific decision, not a performance one.**
3. **Grading currency.** The grader graded `6901edb`. Three changes have landed
   since, each re-verified (bit-identical digest, identical 1.6 M-record union,
   suite green), but not independently.

## 6. Declined on measurement, so the record is honest about what was left

| candidate | measured value | why declined |
| --- | ---: | --- |
| complete schema coverage (`maker_state` first) | ~1.4 % | at or below the A/A floor |
| cache one interning lookup | ~0.6 % | inside the A/A floor |
| faster digest (BLAKE3 etc.) | ≤2.69 % | format-identity change for a sub-noise gain |
| `GOAMD64=v3/v4` | not measured | **disqualified**: changes the execution hash |
| native-language rewrite | — | matching is 0.45 % of CPU; the hash already runs on SHA-NI |

## 7. A finding for the scientific branch, not this one

Building the same revision with `GOAMD64=v3` produces a **different execution
hash**: the compiler fuses multiply-add into FMA, `log_variance` shifts by two
ulp, and 168 of 208,942 frames differ. At three minutes no price or position
field diverged, but it feeds the Stoikov spread, so that does not extend to 24 h.

The run manifest now records Go version, GOARCH, GOOS and GOAMD64 — **on this
branch only**. The scientific branch still does not, so a failed reproduction
there cannot be told apart from a semantic change. Porting it is a
provenance-only change with no semantics.

## 8. Reproduction

```bash
CFG=research/configs/v2-integrated-longrun/dev-607.json

# wall clock, both arms same binary
EXSIM_BINARY_EVIDENCE=replace ./multivenue -config $CFG -duration 20m -seed 607 -logdir bin/
env -u EXSIM_BINARY_EVIDENCE  ./multivenue -config $CFG -duration 20m -seed 607 -logdir json/

# losslessness, union form
cat <(go run ./cmd/evsrender -dir bin/) bin/venues/*/*.jsonl bin/venues/*/*/*.jsonl |
  LC_ALL=C sort > union.sorted
cat json/venues/*/*.jsonl json/venues/*/*/*.jsonl | LC_ALL=C sort > original.sorted
cmp union.sorted original.sorted

# codec independence: all four must print the same hash
for c in none lz4 s2 zstd; do EXSIM_BINARY_EVIDENCE=1 EXSIM_BINARY_CODEC=$c ... ; done
```

## 9. The next experiment with the highest information gain

Build the file-layout renderer and differentially verify **per file** rather
than as one stream. That closes blocker 1 and converts the losslessness result
from "the content is all there" to "the artifacts are interchangeable", which is
what an adoption decision actually needs.
