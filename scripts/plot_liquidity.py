#!/usr/bin/env python3
"""
Liquidity report — reads logs/microstructure_v1/liquidity_report.json.
Generates plots and a structured text analysis.

Usage:
    python scripts/plot_liquidity.py [-v|-vv] [<json_path>]

    default : health summary + flagged anomalies
    -v      : + per-symbol table + per-client table
    -vv     : + raw stats for every field
"""

import argparse
import json
import sys
from pathlib import Path

import matplotlib.patches as mpatches
import matplotlib.pyplot as plt
import numpy as np

parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
parser.add_argument("json_path", nargs="?", default="logs/microstructure_v1/liquidity_report.json")
parser.add_argument("-v", "--verbose", action="count", default=0,
                    help="verbosity: -v adds per-symbol/client tables, -vv adds raw fields")
args = parser.parse_args()

json_path = Path(args.json_path)
out_dir = json_path.parent / "plots"
out_dir.mkdir(exist_ok=True)

with open(json_path) as f:
    rep = json.load(f)

syms_data       = rep["symbols"]
clients_data    = rep["clients"]
bootstrap_end_ns = rep["bootstrap_end_ns"]
sim_epoch_ns    = rep["sim_epoch_ns"]
bootstrap_sec   = (bootstrap_end_ns - sim_epoch_ns) / 1e9
sim_dur_h       = 25.0  # hardcoded, not in JSON

TYPE_COLORS = {"perp": "#e74c3c", "spot_usd": "#3498db", "spot_abc": "#2ecc71"}
TYPE_LABELS = {"perp": "Perp", "spot_usd": "Spot/USD", "spot_abc": "Spot/ABC"}

symbols = sorted(syms_data)
types   = [syms_data[s]["type"] for s in symbols]
colors  = [TYPE_COLORS[t] for t in types]

def sv(key):
    return np.array([syms_data[s][key] for s in symbols], dtype=float)

trades    = sv("trades")
limits    = sv("limits")
markets   = sv("markets")
cancels   = sv("cancels")
rejects   = sv("rejects")
fills     = sv("fills")
vol_base  = sv("vol_base")
vol_quote = sv("vol_quote")

fill_rate   = np.where(markets > 0, fills   / markets, 0.0)
reject_rate = np.where(markets > 0, rejects / markets, 0.0)

lp_exits = {s: syms_data[s]["first_lp_exit_sim_sec"]
            for s in symbols if syms_data[s]["first_lp_exit_ns"] > 0}

total_trades  = int(trades.sum())
total_fills   = int(fills.sum())
total_limits  = int(limits.sum())
total_markets = int(markets.sum())
total_rejects = int(rejects.sum())
total_cancels = int(cancels.sum())
total_vol_b   = float(vol_base.sum())
total_vol_q   = float(vol_quote.sum())

# ── Text output ───────────────────────────────────────────────────────────────

W = 80

def hr(char="─"):
    print(char * W)

def section(title):
    print()
    hr("═")
    print(f"  {title}")
    hr("═")

def flag(cond, msg, level="WARN"):
    if cond:
        print(f"  [{level}] {msg}")

def ok(msg):
    print(f"  [OK]   {msg}")

section("SIMULATION HEALTH SUMMARY")
print(f"  Duration          : {sim_dur_h:.0f}h simulated")
print(f"  Symbols           : {len(symbols)} total  ({sum(1 for t in types if t=='perp')} perp / "
      f"{sum(1 for t in types if t=='spot_usd')} spot_usd / "
      f"{sum(1 for t in types if t=='spot_abc')} spot_abc)")
print(f"  Bootstrap end     : sim +{bootstrap_sec:.0f}s ({bootstrap_sec/3600:.2f}h)")
print()
print(f"  Trades (total)    : {total_trades:,}")
print(f"  Fills  (total)    : {total_fills:,}  (fills/market = {total_fills/max(total_markets,1):.2f}x)")
print(f"  Limits placed     : {total_limits:,}")
print(f"  Markets placed    : {total_markets:,}")
print(f"  Rejects           : {total_rejects:,}  ({total_rejects/max(total_markets,1)*100:.0f}% of markets)")
print(f"  Cancels           : {total_cancels:,}")
print(f"  Base volume       : {total_vol_b:.2f} units")
print(f"  Quote volume (USD): ${total_vol_q:,.0f}")
print(f"  LP exits          : {len(lp_exits)}/{len(symbols)} symbols")

section("HEALTH FLAGS")

zero_trade_syms = [s for s in symbols if syms_data[s]["trades"] == 0]
flag(len(zero_trade_syms) > 0,
     f"Symbols with 0 trades: {zero_trade_syms}", "CRIT")
ok("All symbols traded") if not zero_trade_syms else None

# LP exits too early (< 1h after bootstrap = < 3600s sim)
early_exits = {s: t for s, t in lp_exits.items() if t < 3600}
flag(len(early_exits) > 0,
     f"LP exits within 1h of bootstrap: "
     + ", ".join(f"{s} at sim+{t:.0f}s" for s, t in sorted(early_exits.items(), key=lambda x: x[1])))

# Very early exits (< 10 min = 600s)
very_early = {s: t for s, t in lp_exits.items() if t < 600}
flag(len(very_early) > 0,
     f"LP exits within 10 min: "
     + ", ".join(f"{s} at sim+{t:.0f}s" for s, t in sorted(very_early.items(), key=lambda x: x[1])),
     "CRIT")

# Symbols with no LP exit are healthy (book never fully depleted)
stable_syms = [s for s in symbols if syms_data[s]["first_lp_exit_ns"] == 0]
ok(f"Stable LP depth throughout 25h: {stable_syms}")

# Reject rate by market type
for mtype in ("perp", "spot_usd", "spot_abc"):
    idxs = [i for i, t in enumerate(types) if t == mtype]
    r = int(rejects[idxs].sum())
    m = int(markets[idxs].sum())
    rate = r / max(m, 1) * 100
    flag(rate > 50, f"High reject rate on {mtype}: {rate:.0f}% ({r} rejects / {m} markets)")
    if rate <= 50:
        ok(f"Reject rate {mtype}: {rate:.0f}%")

# Trade uniformity — CV of trades across symbols
trade_cv = float(np.std(trades) / np.mean(trades)) if np.mean(trades) > 0 else 0
flag(trade_cv > 0.3, f"Uneven trade distribution (CV={trade_cv:.2f}); some symbols much more active")
ok(f"Trade count uniform across symbols (CV={trade_cv:.2f})") if trade_cv <= 0.3 else None

# Fill rate sanity: each market order should produce ≥1 fill on average
global_fill_rate = total_fills / max(total_markets, 1)
flag(global_fill_rate < 1.0,
     f"Fill rate < 1.0 ({global_fill_rate:.2f}) — some market orders not filling at all", "CRIT")
ok(f"Market orders filling on average {global_fill_rate:.2f}x") if global_fill_rate >= 1.0 else None

# Arb actors check (clients 7=FundingArb, 8=TriangleArb expected to have low but non-zero activity)
c7 = clients_data.get("7", {})
c8 = clients_data.get("8", {})
arb_total = (c7.get("markets", 0) + c7.get("limits", 0) +
             c8.get("markets", 0) + c8.get("limits", 0))
flag(arb_total < 10, f"Arb actors (c7 FundingArb, c8 TriangleArb) nearly inactive: "
     f"c7={c7.get('markets',0)} mkts, c8={c8.get('markets',0)} mkts — "
     f"spreads never crossing threshold or no funding rate divergence")

# ── Verbose: per-symbol table ──────────────────────────────────────────────────
if args.verbose >= 1:
    section("PER-SYMBOL BREAKDOWN")
    hdr = (f"{'Symbol':<12} {'Type':<9} {'Trades':>6} {'VolBase':>8} {'VolQuote':>12} "
           f"{'Limits':>6} {'Mkts':>5} {'Rej%':>5} {'FillX':>5} {'LPExit':>10}")
    print(f"  {hdr}")
    hr()
    for s in symbols:
        d = syms_data[s]
        rj = d["rejects"] / max(d["markets"], 1) * 100
        fx = d["fills"] / max(d["markets"], 1)
        lp = f"+{d['first_lp_exit_sim_sec']:.0f}s" if d["first_lp_exit_ns"] > 0 else "stable"
        flags = ""
        if d["trades"] == 0:
            flags += " !DEAD"
        if d["first_lp_exit_ns"] > 0 and d["first_lp_exit_sim_sec"] < 600:
            flags += " !EARLY_EXIT"
        if rj > 100:
            flags += " !HIREJ"
        row = (f"  {s:<12} {d['type']:<9} {d['trades']:>6} {d['vol_base']:>8.2f} "
               f"{d['vol_quote']:>12,.1f} {d['limits']:>6} {d['markets']:>5} "
               f"{rj:>4.0f}% {fx:>5.1f} {lp:>10}{flags}")
        print(row)
    hr()

    section("PER-CLIENT BREAKDOWN")
    # Infer roles from order patterns
    role_map = {
        "1": "PerpMM",      "2": "SpotUSDMM",   "3": "SpotABCMM",
        "4": "SpotTaker",   "5": "SpotABCTaker", "6": "PerpTaker",
        "7": "FundingArb",  "8": "TriangleArb",
        "9": "Bootstrap",   "10": "Bootstrap",   "11": "Bootstrap",
        "12": "Bootstrap",  "13": "Bootstrap",   "14": "Bootstrap",
    }
    hdr = (f"{'Client':<8} {'Role':<13} {'Limits':>6} {'Mkts':>5} "
           f"{'Fills':>5} {'Rejects':>7} {'VolBase':>8} {'Notes'}")
    print(f"  {hdr}")
    hr()
    for cid in sorted(clients_data, key=int):
        d = clients_data[cid]
        role = role_map.get(cid, "Unknown")
        notes = []
        if d["fills"] == 0 and (d["limits"] + d["markets"]) > 0:
            notes.append("no fills")
        if d["rejects"] > d["markets"] * 2 and d["markets"] > 0:
            notes.append(f"reject rate {d['rejects']/max(d['markets'],1):.0f}x")
        if role == "FundingArb" and d["markets"] < 10:
            notes.append("INACTIVE")
        if role == "TriangleArb" and d["markets"] < 20:
            notes.append("LOW")
        row = (f"  {cid:<8} {role:<13} {d['limits']:>6} {d['markets']:>5} "
               f"{d['fills']:>5} {d['rejects']:>7} {d['vol_base']:>8.2f}  "
               + ("  ".join(notes) if notes else "ok"))
        print(row)
    hr()

# ── Verbose level 2: raw JSON per symbol ──────────────────────────────────────
if args.verbose >= 2:
    section("RAW SYMBOL DATA")
    for s in symbols:
        print(f"  {s}: {json.dumps(syms_data[s])}")
    section("RAW CLIENT DATA")
    for cid in sorted(clients_data, key=int):
        print(f"  c{cid}: {json.dumps(clients_data[cid])}")

section("ASSESSMENT")
issues = []

if zero_trade_syms:
    issues.append(f"CRITICAL: {len(zero_trade_syms)} symbols with 0 trades")
if very_early:
    issues.append(f"CRITICAL: LP exits within 10min on {list(very_early)}")
if global_fill_rate < 1.0:
    issues.append(f"CRITICAL: fill rate {global_fill_rate:.2f} < 1.0")
if early_exits and not very_early:
    issues.append(f"WARN: LP book thins within 1h on {len(early_exits)} symbols")

# spot_abc reject rate
abc_idxs = [i for i, t in enumerate(types) if t == "spot_abc"]
abc_rej_rate = rejects[abc_idxs].sum() / max(markets[abc_idxs].sum(), 1) * 100
if abc_rej_rate > 100:
    issues.append(f"WARN: spot/ABC reject rate {abc_rej_rate:.0f}% — takers likely hitting balance or price constraints")

c7_mkts = clients_data.get("7", {}).get("markets", 0)
c8_mkts = clients_data.get("8", {}).get("markets", 0)
if c7_mkts < 10:
    issues.append(f"INFO: FundingArb (c7) placed only {c7_mkts} markets — funding rate spread never exceeded threshold")
if c8_mkts < 100:
    issues.append(f"INFO: TriangleArb (c8) placed only {c8_mkts} markets — triangle spread rarely profitable after fees")

if not issues:
    print("  Simulation looks healthy. All symbols active, LP stable, fill rates normal.")
else:
    for iss in issues:
        print(f"  {iss}")

print()

# ── Plots ─────────────────────────────────────────────────────────────────────

short = [s.replace("/", "/\n").replace("-", "-\n") for s in symbols]
x = np.arange(len(symbols))
w = 0.6
legend_patches = [mpatches.Patch(color=c, label=TYPE_LABELS[t]) for t, c in TYPE_COLORS.items()]

# Figure 1: Market Activity Dashboard
fig, axes = plt.subplots(2, 3, figsize=(18, 10))
fig.suptitle("Market Activity Dashboard", fontsize=15, fontweight="bold", y=1.01)

ax = axes[0, 0]
bars = ax.bar(x, trades, color=colors, width=w, edgecolor="white", linewidth=0.4)
for bar, val in zip(bars, trades):
    if val > 0:
        ax.text(bar.get_x() + bar.get_width()/2, bar.get_height() + 0.5,
                str(int(val)), ha="center", va="bottom", fontsize=7)
ax.set_xticks(x); ax.set_xticklabels(short, fontsize=6.5)
ax.set_title("Trades per Symbol", fontweight="bold")
ax.set_ylabel("count")
ax.legend(handles=legend_patches, fontsize=7, loc="upper right")

ax = axes[0, 1]
ax.bar(x, vol_base, color=colors, width=w, edgecolor="white", linewidth=0.4)
ax.set_xticks(x); ax.set_xticklabels(short, fontsize=6.5)
ax.set_title("Base Volume per Symbol", fontweight="bold")
ax.set_ylabel("base units")

ax = axes[0, 2]
ax.bar(x, vol_quote, color=colors, width=w, edgecolor="white", linewidth=0.4)
nonzero = vol_quote[vol_quote > 0]
if nonzero.size > 0 and nonzero.max() / max(nonzero.min(), 1) > 100:
    ax.set_yscale("log")
ax.set_xticks(x); ax.set_xticklabels(short, fontsize=6.5)
ax.set_title("Quote Volume per Symbol", fontweight="bold")
ax.set_ylabel("quote units")

ax = axes[1, 0]
ax.bar(x, limits,  width=w, label="Limits",  color="#3498db", edgecolor="white", linewidth=0.4)
ax.bar(x, markets, width=w, label="Markets", color="#e67e22", edgecolor="white", linewidth=0.4, bottom=limits)
ax.set_xticks(x); ax.set_xticklabels(short, fontsize=6.5)
ax.set_title("Order Types (Limits + Markets)", fontweight="bold")
ax.set_ylabel("count")
ax.legend(fontsize=8)

bw = w / 2
ax = axes[1, 1]
ax.bar(x - bw/2, fill_rate,   width=bw, label="Fill rate",   color="#2ecc71", edgecolor="white", linewidth=0.4)
ax.bar(x + bw/2, reject_rate, width=bw, label="Reject rate", color="#e74c3c", edgecolor="white", linewidth=0.4)
ax.set_xticks(x); ax.set_xticklabels(short, fontsize=6.5)
ax.set_title("Fill & Reject Rates (per market order)", fontweight="bold")
ax.set_ylabel("ratio")
ax.legend(fontsize=8)

ax = axes[1, 2]
ax.bar(x - bw/2, cancels, width=bw, label="Cancels", color="#9b59b6", edgecolor="white", linewidth=0.4)
ax.bar(x + bw/2, rejects, width=bw, label="Rejects", color="#e74c3c", edgecolor="white", linewidth=0.4)
ax.set_xticks(x); ax.set_xticklabels(short, fontsize=6.5)
ax.set_title("Cancels & Rejects per Symbol", fontweight="bold")
ax.set_ylabel("count")
ax.legend(fontsize=8)

fig.tight_layout()
p = out_dir / "symbol_overview.png"
fig.savefig(p, dpi=140, bbox_inches="tight")
print(f"saved {p}")
plt.close(fig)

# Figure 2: Client Profile
client_ids = sorted(clients_data, key=int)
cx = np.arange(len(client_ids))
cw = 0.16
fig, axes = plt.subplots(1, 2, figsize=(14, 5))
fig.suptitle("Client Activity Profile (main phase)", fontsize=14, fontweight="bold")

cl = np.array([clients_data[c]["limits"]  for c in client_ids], dtype=float)
cm = np.array([clients_data[c]["markets"] for c in client_ids], dtype=float)
cf = np.array([clients_data[c]["fills"]   for c in client_ids], dtype=float)
cc = np.array([clients_data[c]["cancels"] for c in client_ids], dtype=float)
cr = np.array([clients_data[c]["rejects"] for c in client_ids], dtype=float)

ax = axes[0]
ax.bar(cx - 2*cw, cl, width=cw, label="Limits",  color="#3498db")
ax.bar(cx -   cw, cm, width=cw, label="Markets", color="#e67e22")
ax.bar(cx,        cf, width=cw, label="Fills",   color="#2ecc71")
ax.bar(cx +   cw, cc, width=cw, label="Cancels", color="#9b59b6")
ax.bar(cx + 2*cw, cr, width=cw, label="Rejects", color="#e74c3c")
ax.set_xticks(cx); ax.set_xticklabels([f"c{c}" for c in client_ids], fontsize=8)
ax.set_title("Order Activity per Client", fontweight="bold")
ax.set_ylabel("count")
ax.legend(fontsize=8)

ax = axes[1]
cv = np.array([clients_data[c]["vol_base"] for c in client_ids], dtype=float)
bar_colors = ["#1abc9c" if int(c) <= 8 else "#e74c3c" for c in client_ids]
ax.bar(cx, cv, color=bar_colors, width=0.55)
ax.set_xticks(cx); ax.set_xticklabels([f"c{c}" for c in client_ids], fontsize=8)
ax.set_title("Base Volume per Client", fontweight="bold")
ax.set_ylabel("base units")
mm_patch  = mpatches.Patch(color="#1abc9c", label="Main actors (c1-8)")
bst_patch = mpatches.Patch(color="#e74c3c", label="Bootstrap (c9-14)")
ax.legend(handles=[mm_patch, bst_patch], fontsize=8)

fig.tight_layout()
p = out_dir / "client_stats.png"
fig.savefig(p, dpi=140, bbox_inches="tight")
print(f"saved {p}")
plt.close(fig)

# Figure 3: LP Health & Market Structure
fig, axes = plt.subplots(1, 3, figsize=(16, 5))
fig.suptitle("Market Structure & LP Health", fontsize=14, fontweight="bold")

ax = axes[0]
if lp_exits:
    exit_syms = sorted(lp_exits, key=lp_exits.get)
    exit_hrs  = [lp_exits[s] / 3600 for s in exit_syms]
    ey = np.arange(len(exit_syms))
    ecols = [TYPE_COLORS[syms_data[s]["type"]] for s in exit_syms]
    ax.barh(ey, exit_hrs, color=ecols, height=0.55)
    ax.set_yticks(ey); ax.set_yticklabels(exit_syms, fontsize=8)
    ax.axvline(bootstrap_sec / 3600, color="gray", linestyle="--", linewidth=1, label="bootstrap end")
    ax.set_xlabel("Sim hours from epoch")
    ax.set_title("First LP Level → 0 After Bootstrap", fontweight="bold")
    ax.legend(fontsize=8)
else:
    ax.text(0.5, 0.5, "No LP exits recorded", ha="center", va="center", transform=ax.transAxes)
    ax.set_title("First LP Level → 0 After Bootstrap", fontweight="bold")

ax = axes[1]
type_totals = {}
for s in symbols:
    t = syms_data[s]["type"]
    type_totals[t] = type_totals.get(t, 0) + syms_data[s]["trades"]
if sum(type_totals.values()) > 0:
    t_labels = [TYPE_LABELS[t] for t in type_totals]
    t_sizes  = list(type_totals.values())
    t_colors = [TYPE_COLORS[t] for t in type_totals]
    ax.pie(t_sizes, labels=t_labels, colors=t_colors, autopct="%1.0f%%",
           startangle=90, textprops={"fontsize": 9})
    ax.set_title("Trade Count by Market Type", fontweight="bold")

ax = axes[2]
market_types = list(TYPE_COLORS.keys())
base_vals  = [sum(syms_data[s]["vol_base"]  for s in symbols if syms_data[s]["type"] == t) for t in market_types]
quote_vals = [sum(syms_data[s]["vol_quote"] for s in symbols if syms_data[s]["type"] == t) for t in market_types]
cum_b, cum_q = 0.0, 0.0
for t, bv, qv in zip(market_types, base_vals, quote_vals):
    ax.barh(0, bv, left=cum_b, height=0.4, color=TYPE_COLORS[t], label=TYPE_LABELS[t])
    ax.barh(1, qv, left=cum_q, height=0.4, color=TYPE_COLORS[t])
    cum_b += bv; cum_q += qv
ax.set_yticks([0, 1]); ax.set_yticklabels(["Base vol", "Quote vol"])
ax.set_title("Volume Share by Market Type", fontweight="bold")
ax.set_xlabel("units")
ax.legend(fontsize=8, loc="lower right")

fig.tight_layout()
p = out_dir / "lp_exits.png"
fig.savefig(p, dpi=140, bbox_inches="tight")
print(f"saved {p}")
plt.close(fig)

# Figure 4: Quote Volume Breakdown
usd_syms = [s for s in symbols if syms_data[s]["type"] in ("spot_usd", "perp")]
abc_syms  = [s for s in symbols if syms_data[s]["type"] == "spot_abc"]
fig, axes = plt.subplots(1, 2, figsize=(14, 5))
fig.suptitle("Quote Volume by Group", fontsize=14, fontweight="bold")
for ax, group, label, col in [
    (axes[0], usd_syms, "USD (Spot + Perp)", "#3498db"),
    (axes[1], abc_syms, "ABC (Cross pairs)",  "#2ecc71"),
]:
    gx = np.arange(len(group))
    gv = np.array([syms_data[s]["vol_quote"] for s in group])
    short_g = [s.replace("/", "/\n").replace("-", "-\n") for s in group]
    ax.bar(gx, gv, color=col, width=0.55, edgecolor="white", linewidth=0.4)
    for i, val in enumerate(gv):
        ax.text(i, val + max(gv) * 0.01, f"{val:,.1f}", ha="center", va="bottom", fontsize=7)
    ax.set_xticks(gx); ax.set_xticklabels(short_g, fontsize=8)
    ax.set_title(f"Quote Volume — {label}", fontweight="bold")
    ax.set_ylabel(label.split()[0])
fig.tight_layout()
p = out_dir / "quote_volume.png"
fig.savefig(p, dpi=140, bbox_inches="tight")
print(f"saved {p}")
plt.close(fig)

# Figure 5: Summary table
fig, ax = plt.subplots(figsize=(10, 5))
ax.axis("off")
fig.suptitle("Simulation Summary", fontsize=15, fontweight="bold")
rows = [
    ("Duration",              f"{sim_dur_h:.0f}h simulated"),
    ("Symbols",               f"{len(symbols)}  ({sum(1 for t in types if t=='perp')} perp / "
                               f"{sum(1 for t in types if t=='spot_usd')} spot/USD / "
                               f"{sum(1 for t in types if t=='spot_abc')} spot/ABC)"),
    ("Total trades",          f"{total_trades:,}"),
    ("Total fills",           f"{total_fills:,}  (avg {total_fills/max(total_markets,1):.1f}x per market order)"),
    ("Limit orders",          f"{total_limits:,}"),
    ("Market orders",         f"{total_markets:,}"),
    ("Rejections",            f"{total_rejects:,}  ({total_rejects/max(total_markets,1)*100:.0f}% of markets)"),
    ("Base volume",           f"{total_vol_b:.2f} units"),
    ("Quote volume (USD)",    f"${total_vol_q:,.0f}"),
    ("Symbols with 0 trades", f"{len(zero_trade_syms)}  {zero_trade_syms if zero_trade_syms else '(none)'}"),
    ("LP exits after bootstrap", f"{len(lp_exits)}/{len(symbols)} symbols"),
]
table = ax.table(cellText=rows, colLabels=["Metric", "Value"], loc="center", cellLoc="left")
table.auto_set_font_size(False)
table.set_fontsize(11)
table.scale(1.2, 1.8)
for (row, col), cell in table.get_celld().items():
    cell.set_edgecolor("#cccccc")
    if row == 0:
        cell.set_facecolor("#2c3e50"); cell.set_text_props(color="white", fontweight="bold")
    elif row % 2 == 0:
        cell.set_facecolor("#f8f9fa")
    else:
        cell.set_facecolor("white")
fig.tight_layout()
p = out_dir / "summary.png"
fig.savefig(p, dpi=140, bbox_inches="tight")
print(f"saved {p}")
plt.close(fig)

print(f"\nAll plots saved to {out_dir}/")
