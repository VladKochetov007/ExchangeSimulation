"""Render human-readable views of a multi-venue derivatives run.

The simulator writes one JSONL stream per venue. This tool reads those logs and
draws the panels a derivatives desk would look at first:

  * implied volatility by strike and expiry, inverted from traded option prices
  * spot versus perpetual basis over time
  * the dated-futures term structure against time to expiry
  * the funding rate path and the funding actually accrued per period

Implied volatility is inverted from *traded* prices with Black-76 against the
concurrent underlying mark. It is therefore what the dealer's spread and
inventory skew produced, not the flat parameter the dealer quotes around; the
two differing is the interesting part.

Usage:
    python tools/plot_derivatives.py --logdir logs/research/<run> --out plots/
"""

from __future__ import annotations

import argparse
import bisect
import json
import math
import re
from dataclasses import dataclass, field
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt

QUOTE_PRECISION = 100_000
BASE_PRECISION = 100_000_000
SECONDS_PER_YEAR = 365.0 * 24.0 * 3600.0

OPTION_SYMBOL = re.compile(r"^(?P<under>[A-Z]+)-(?P<expiry>\d+)-(?P<strike>\d+)-(?P<kind>[CP])$")
FUTURE_SYMBOL = re.compile(r"^(?P<under>[A-Z]+)-FUT-(?P<expiry>\d+)$")


@dataclass
class Series:
    ts: list[float] = field(default_factory=list)
    value: list[float] = field(default_factory=list)

    def add(self, ts: float, value: float) -> None:
        self.ts.append(ts)
        self.value.append(value)

    def at(self, ts: float) -> float | None:
        """Most recent value at or before ts."""
        if not self.ts:
            return None
        i = bisect.bisect_right(self.ts, ts) - 1
        return self.value[i] if i >= 0 else None


@dataclass
class OptionTrade:
    ts: float
    expiry: float
    strike: float
    kind: str
    price: float


@dataclass
class VenueData:
    name: str
    spot: Series = field(default_factory=Series)
    perp_mark: Series = field(default_factory=Series)
    perp_trades: Series = field(default_factory=Series)
    funding: Series = field(default_factory=Series)
    funding_period_hours: float | None = None
    future_marks: dict[str, Series] = field(default_factory=dict)
    future_trades: dict[str, Series] = field(default_factory=dict)
    future_expiry: dict[str, float] = field(default_factory=dict)
    option_trades: list[OptionTrade] = field(default_factory=list)


def _iter_records(path: Path):
    """Yield parsed records, tolerating a truncated final line."""
    with path.open() as handle:
        for line in handle:
            try:
                yield json.loads(line)
            except json.JSONDecodeError:
                continue


def _payload(record: dict) -> dict:
    data = record.get("data") or {}
    payload = data.get("payload") or {}
    inner = payload.get("payload")
    return inner if isinstance(inner, dict) else payload


def load_venue(venue_dir: Path) -> VenueData:
    venue = VenueData(name=venue_dir.name)

    spot_log = venue_dir / "spot" / "ABC-USD.jsonl"
    if spot_log.exists():
        for record in _iter_records(spot_log):
            if record.get("event") != "BookSnapshot" or record.get("client_id") != 0:
                continue
            book = _payload(record)
            bids, asks = book.get("bids") or [], book.get("asks") or []
            if bids and asks:
                mid = (bids[0]["price"] + asks[0]["price"]) / 2 / QUOTE_PRECISION
                venue.spot.add(record["sim_ts"] / 1e9, mid)

    deriv_log = venue_dir / "derivatives.jsonl"
    if not deriv_log.exists():
        return venue

    for record in _iter_records(deriv_log):
        event = record.get("event")
        body = _payload(record)
        symbol = body.get("symbol") or (record.get("data") or {}).get("payload", {}).get("symbol")
        ts = record["sim_ts"] / 1e9
        if event == "mark_price_update":
            mark = body.get("mark_price", 0) / QUOTE_PRECISION
            if symbol == "ABC-PERP":
                venue.perp_mark.add(ts, mark)
            elif (m := FUTURE_SYMBOL.match(symbol or "")) is not None:
                venue.future_marks.setdefault(symbol, Series()).add(ts, mark)
                venue.future_expiry[symbol] = int(m.group("expiry"))
        elif event == "funding_rate_update" and symbol == "ABC-PERP":
            # Rate is in basis points charged once per funding period.
            venue.funding.add(ts, body.get("rate", 0))
            if venue.funding_period_hours is None and body.get("next_funding"):
                venue.funding_period_hours = (body["next_funding"] / 1e9 - ts) / 3600.0
        elif event == "Trade" and symbol == "ABC-PERP":
            venue.perp_trades.add(ts, body.get("price", 0) / QUOTE_PRECISION)
        elif event == "Trade" and (m := FUTURE_SYMBOL.match(symbol or "")) is not None:
            venue.future_trades.setdefault(symbol, Series()).add(ts, body.get("price", 0) / QUOTE_PRECISION)
            venue.future_expiry[symbol] = int(m.group("expiry"))
        elif event == "Trade" and (m := OPTION_SYMBOL.match(symbol or "")) is not None:
            venue.option_trades.append(
                OptionTrade(
                    ts=ts,
                    expiry=int(m.group("expiry")),
                    strike=int(m.group("strike")) * 1.0,
                    kind=m.group("kind"),
                    price=body.get("price", 0) / QUOTE_PRECISION,
                )
            )
    return venue


def _norm_cdf(x: float) -> float:
    return 0.5 * (1.0 + math.erf(x / math.sqrt(2.0)))


def black76(forward: float, strike: float, tau: float, vol: float, kind: str) -> float:
    """Undiscounted Black-76 premium; the simulator uses a zero rate."""
    if tau <= 0 or vol <= 0 or forward <= 0 or strike <= 0:
        return max(0.0, forward - strike) if kind == "C" else max(0.0, strike - forward)
    d1 = (math.log(forward / strike) + 0.5 * vol * vol * tau) / (vol * math.sqrt(tau))
    d2 = d1 - vol * math.sqrt(tau)
    if kind == "C":
        return forward * _norm_cdf(d1) - strike * _norm_cdf(d2)
    return strike * _norm_cdf(-d2) - forward * _norm_cdf(-d1)


def implied_vol(price: float, forward: float, strike: float, tau: float, kind: str) -> float | None:
    """Bisection inversion; returns None when the price is outside the no-arbitrage range."""
    if tau <= 0 or price <= 0:
        return None
    intrinsic = max(0.0, forward - strike) if kind == "C" else max(0.0, strike - forward)
    upper_bound = forward if kind == "C" else strike
    if price <= intrinsic + 1e-9 or price >= upper_bound:
        return None
    # 5.0 = 500% annualised. Values pinned at the ceiling mean the price is not
    # explicable by Black-76 at any plausible vol, which is a finding rather
    # than a measurement to plot.
    low, high = 1e-4, 5.0
    if black76(forward, strike, tau, high, kind) < price:
        return None
    for _ in range(80):
        mid = 0.5 * (low + high)
        if black76(forward, strike, tau, mid, kind) < price:
            low = mid
        else:
            high = mid
    return 0.5 * (low + high)


def plot_vol_surface(ax, venue: VenueData, cutoff: float | None) -> None:
    by_expiry: dict[float, list[tuple[float, float]]] = {}
    for trade in venue.option_trades:
        if cutoff is not None and trade.ts > cutoff:
            continue
        # Use the spot midpoint, because the dealer prices its options from
        # spot. Taking the perpetual mark instead mixes in the perpetual basis
        # and distorts moneyness whenever the two markets disagree.
        forward = venue.spot.at(trade.ts) or venue.perp_mark.at(trade.ts)
        if not forward:
            continue
        strike = trade.strike
        tau = (trade.expiry - trade.ts) / SECONDS_PER_YEAR
        vol = implied_vol(trade.price, forward, strike, tau, trade.kind)
        if vol is None:
            continue
        by_expiry.setdefault(trade.expiry, []).append((strike / forward, vol))

    if not by_expiry:
        ax.text(0.5, 0.5, "no invertible option trades", ha="center", va="center", transform=ax.transAxes)
    for expiry in sorted(by_expiry):
        points = sorted(by_expiry[expiry])
        hours = (expiry - venue.option_trades[0].ts) / 3600.0
        ax.plot([p[0] for p in points], [p[1] * 100 for p in points], "o-", ms=3, lw=1, label=f"T+{hours:.1f}h")
    ax.set_xlabel("moneyness  K / F")
    ax.set_ylabel("implied volatility (%)")
    ax.set_title(f"{venue.name}: implied vol from traded option prices")
    ax.legend(fontsize=7)
    ax.grid(alpha=0.3)


def plot_basis(ax, venue: VenueData, cutoff: float | None) -> None:
    ts, bps = [], []
    for t, perp in zip(venue.perp_mark.ts, venue.perp_mark.value):
        if cutoff is not None and t > cutoff:
            continue
        spot = venue.spot.at(t)
        if not spot:
            continue
        ts.append(t / 60.0)
        bps.append((perp - spot) / spot * 10_000)
    ax.plot(ts, bps, lw=1)
    ax.axhline(0, color="k", lw=0.6)
    ax.set_xlabel("simulated minutes")
    ax.set_ylabel("perp − spot (bps)")
    ax.set_title(f"{venue.name}: perpetual basis")
    ax.grid(alpha=0.3)


def plot_term_structure(ax, venue: VenueData, cutoff: float | None) -> None:
    """Traded futures basis against time to expiry.

    Marks are deliberately excluded: the engine sets a dated future's mark to
    the underlying index, so a mark-based term structure is flat by
    construction and says nothing about what the market paid for carry.
    """
    drawn = False
    for symbol, series in sorted(venue.future_trades.items()):
        expiry = venue.future_expiry[symbol]
        xs, ys = [], []
        for t, price in zip(series.ts, series.value):
            if cutoff is not None and t > cutoff:
                continue
            spot = venue.spot.at(t)
            if not spot or expiry <= t:
                continue
            xs.append((expiry - t) / 3600.0)
            ys.append((price - spot) / spot * 10_000)
        if xs:
            drawn = True
            ax.plot(xs, ys, ".", ms=3, label=f"expiry in {(expiry - series.ts[0]) / 3600:.0f}h")
    if not drawn:
        ax.text(0.5, 0.5, "no dated-futures trades in window", ha="center", va="center", transform=ax.transAxes)
    ax.axhline(0, color="k", lw=0.6)
    ax.invert_xaxis()
    ax.set_xlabel("hours to expiry")
    ax.set_ylabel("traded future − spot (bps)")
    ax.set_title(f"{venue.name}: dated futures carry")
    if drawn:
        ax.legend(fontsize=7)
    ax.grid(alpha=0.3)


def plot_funding(ax, venue: VenueData, cutoff: float | None) -> None:
    """Funding rate against the basis that drives it.

    Only the rate is plotted. Summing rate samples would not be the funding
    actually paid: the rate is charged once per funding period, while the log
    records it every automation tick.
    """
    ts, rates = [], []
    for t, rate in zip(venue.funding.ts, venue.funding.value):
        if cutoff is not None and t > cutoff:
            continue
        ts.append(t / 60.0)
        rates.append(rate)
    ax.plot(ts, rates, lw=1, color="tab:blue")
    ax.set_xlabel("simulated minutes")
    ax.set_ylabel("funding rate (bps per period)")
    period = "unknown" if venue.funding_period_hours is None else f"{venue.funding_period_hours:.1f}h"
    ax.set_title(f"{venue.name}: perpetual funding, period {period}")
    ax.grid(alpha=0.3)

    twin = ax.twinx()
    bts, bps = [], []
    for t, price in zip(venue.perp_trades.ts, venue.perp_trades.value):
        if cutoff is not None and t > cutoff:
            continue
        spot = venue.spot.at(t)
        if spot:
            bts.append(t / 60.0)
            bps.append((price - spot) / spot * 10_000)
    twin.plot(bts, bps, ".", ms=2, color="tab:orange", alpha=0.6)
    twin.set_ylabel("traded perp − spot (bps)", color="tab:orange")


def render(venue: VenueData, out: Path, cutoff: float | None) -> Path:
    fig, axes = plt.subplots(2, 2, figsize=(13, 9))
    plot_vol_surface(axes[0][0], venue, cutoff)
    plot_basis(axes[0][1], venue, cutoff)
    plot_term_structure(axes[1][0], venue, cutoff)
    plot_funding(axes[1][1], venue, cutoff)
    subtitle = "full run" if cutoff is None else f"first {cutoff / 60:.0f} simulated minutes"
    fig.suptitle(f"venue {venue.name} — {subtitle}")
    fig.tight_layout()
    path = out / f"derivatives-{venue.name}.png"
    fig.savefig(path, dpi=130)
    plt.close(fig)
    return path


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--logdir", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument(
        "--max-minutes",
        type=float,
        default=None,
        help="ignore data after this many simulated minutes, e.g. to stay inside a validated window",
    )
    args = parser.parse_args()

    venues_dir = args.logdir / "venues"
    if not venues_dir.is_dir():
        raise SystemExit(f"no venues directory under {args.logdir}")
    args.out.mkdir(parents=True, exist_ok=True)

    for venue_dir in sorted(p for p in venues_dir.iterdir() if p.is_dir()):
        venue = load_venue(venue_dir)
        if not venue.spot.ts:
            print(f"{venue.name}: no spot snapshots, skipped")
            continue
        start = venue.spot.ts[0]
        cutoff = start + args.max_minutes * 60 if args.max_minutes else None
        # Panels are drawn against absolute simulated seconds; rebase for display.
        for series in [venue.spot, venue.perp_mark, venue.perp_trades, venue.funding,
                       *venue.future_marks.values(), *venue.future_trades.values()]:
            series.ts = [t - start for t in series.ts]
        for trade in venue.option_trades:
            trade.ts -= start
            trade.expiry -= start
        for symbol in venue.future_expiry:
            venue.future_expiry[symbol] -= start
        rebased = None if cutoff is None else cutoff - start
        print(f"{venue.name}: {render(venue, args.out, rebased)}")


if __name__ == "__main__":
    main()
