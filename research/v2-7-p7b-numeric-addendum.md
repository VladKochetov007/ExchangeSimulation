# V2-7 P7b numeric addendum

Status: **immutable before P7b outcomes**. This file fixes the P7b numbers;
the preregistration and rendered configs are the authority. No P7b outcome is
used to choose a value.

| choice | value | class | ex-ante rationale |
|---|---|---|---|
| venue roster | north, central, south | A | inherited V2 environment |
| cross-asset graph | false | A | inherited P7a perp-only risk scope |
| fixed physical exposure | -1,000,000,000 raw ABC | A | preserve P7a's declared 10-ABC liability and isolate the unit correction |
| request cap | 250,000,000 raw ABC | A | preserve ordinary bounded IOC execution |
| quote wallet | 50,000,000 raw USD | A | preserve P7a local operating wallet |
| tick | 10,000 raw quote | A | preserve ABC-PERP grid |
| public-feed delay | 40 ms | A | preserve audited local-feed contract |
| decision interval | 2 s | A | preserve actor clock |
| disabled/control margin | 10,000,000,000 raw USD | C | $100k finite capital and 5x gross leverage at $500k notional |
| lower active margin | 10,000,000,000 raw USD | C | wider finite-capital comparison, two times the approximate initial requirement |
| higher active margin | 5,500,000,000 raw USD | C | $55k, a small stated buffer above the approximately $50k initial requirement plus fees |
| horizon | 24 h | C | one-day risk window, fixed before outcomes |
| preflight horizon | 15 min | C | mechanics-only cheap gate; cannot score risk |
| development seeds | 337, 341 | C | fresh odd screening seeds reserved before P7b runs |
| holdout seeds | 347, 349, 353 | C | fresh seeds reserved before any outcome |
| evidence | full + receipts/frontiers + strict risk | A | inherited scientific evidence contract |

`USD_PRECISION=100,000`, `ABC` base precision is `100,000,000`, and the
inherited perp initial/maintenance rates are 10%/5%. At 10 ABC and $50,000,
the opening notional is about $500,000 (50,000,000,000 raw quote), the initial
requirement is about 5,000,000,000 raw, and a five-basis-point fee is about
25,000,000 raw. No field is selected from an observed P7a mark path.

The exact config hashes are recorded here after rendering and before any
P7b world is run. No holdout config may be generated until the development
promotion rule is met.

| cell | SHA-256 |
|---|---|
| C-337 | `d9bd9f0928d80d3cb289037110f5c22a1af2ed6de1e417ece9b94e16f78539bf` |
| C-341 | `c18a9df7d70b36964dae40d8bccb9fcb64b14e38c8f072384af2a7fea8be36b7` |
| L-337 | `eadae97f8363e25d73584d9699194f2e1d7673f01f0888071b04211a8f94d468` |
| L-341 | `c5c037dc5bc270dfeb40cd38ddbbc847aabf69c25069ff6a4d016b705f6f7c39` |
| H-337 | `eee3960cea6f62d38125bbf2c151b18242ea9d15825109bb5e75c5df6ce4749b` |
| H-341 | `7f4a44a981b5ec13938856cc87dc9458a04ea3038984af4f1afeca74f35f6277` |
