# V2-3 P0 replacement evidence-contract correction

Status: **resolved before scoring; simulator, configurations, and raw worlds
unchanged.**

The replacement run plan already requires reporting every actual
arrival-time post-only rejection. Its first extraction implementation wrote
only `postonly-cdf.json`, filtered to CDF/USD. That is correct for the primary
viability screen but insufficient for policy activation: P0 applies to the
complete passive **spot** population, including ABC/CDF.

The corrected extraction additionally writes `postonly-spot-policy.json`,
selecting:

```text
spot_maker,cdf_spot_maker,abc_cdf_spot_maker,
fixed_distance_maker,imbalance_maker
```

with no symbol filter. The separately required `postonly-derivative-scope`
artifact proves that the regular derivative orders contributed by the last two
roles were not made post-only. Thus the aggregate rejection count is an
explicit policy-activation measurement, not an accidental derivative count.

This is a missing evidence artifact correction, not a revised prediction or a
new market metric. Re-extraction operates only on the six retained raw worlds.
