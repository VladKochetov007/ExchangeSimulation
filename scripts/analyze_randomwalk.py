#!/usr/bin/env python3
import polars as pl
import numpy as np
import matplotlib.pyplot as plt
from scipy import stats
from statsmodels.tsa.stattools import adfuller, acf
import sys
from pathlib import Path

LOG_FILE = Path("logs/randomwalk/_global.log")
OUTPUT_FILE = Path("logs/randomwalk/random_walk_analysis.png")


def load_price_data(log_file):
    df = pl.read_ndjson(log_file)
    start_time = df["sim_time"].min()

    trades = df.filter(pl.col("event") == "Trade").with_columns([
        ((pl.col("sim_time") - start_time) / 1e9).alias("time_sec"),
        (pl.col("price") / 1e8).alias("price_usd"),
        (pl.col("qty") / 1e8).alias("qty_btc")
    ]).sort("time_sec")

    return trades


def test_random_walk(prices):
    results = {}

    adf_result = adfuller(prices, autolag='AIC')
    results['adf_statistic'] = adf_result[0]
    results['adf_pvalue'] = adf_result[1]
    results['adf_pass'] = adf_result[1] > 0.05

    returns = np.diff(prices) / prices[:-1]
    acf_values = acf(returns, nlags=40, fft=True)
    n = len(returns)
    conf_interval = 1.96 / np.sqrt(n)
    significant_lags = np.sum(np.abs(acf_values[1:]) > conf_interval)
    results['acf_significant_lags'] = significant_lags
    results['acf_pass'] = significant_lags <= 2

    shapiro_stat, shapiro_p = stats.shapiro(returns)
    results['shapiro_statistic'] = shapiro_stat
    results['shapiro_pvalue'] = shapiro_p
    results['shapiro_pass'] = shapiro_p > 0.05

    def variance_ratio(prices, k):
        returns_1 = np.diff(prices)
        returns_k = prices[k:] - prices[:-k]
        var_1 = np.var(returns_1)
        var_k = np.var(returns_k)
        return var_k / (k * var_1)

    vr_results = {}
    for k in [5, 10, 20]:
        if len(prices) > k:
            vr = variance_ratio(prices, k)
            vr_results[k] = vr
    results['variance_ratios'] = vr_results
    results['vr_pass'] = all(0.8 < vr < 1.2 for vr in vr_results.values())

    def runs_test(sequence):
        n = len(sequence)
        n_runs = 1 + np.sum(sequence[:-1] != sequence[1:])
        n_pos = np.sum(sequence)
        n_neg = n - n_pos
        if n_pos == 0 or n_neg == 0:
            return 0.5
        expected_runs = 1 + (2 * n_pos * n_neg) / n
        var_runs = (2 * n_pos * n_neg * (2 * n_pos * n_neg - n)) / (n * n * (n - 1))
        z = (n_runs - expected_runs) / np.sqrt(var_runs)
        pvalue = 2 * (1 - stats.norm.cdf(abs(z)))
        return pvalue

    runs_above = (returns > 0).astype(int)
    runs_pvalue = runs_test(runs_above)
    results['runs_pvalue'] = runs_pvalue
    results['runs_pass'] = runs_pvalue > 0.05

    return results, returns


def visualize_results(trades_df):
    fig, ax = plt.subplots(1, 1, figsize=(16, 8))

    time_series = trades_df["time_sec"].to_numpy()
    price_series = trades_df["price_usd"].to_numpy()
    sides = trades_df["side"].to_numpy()
    quantities = trades_df["qty_btc"].to_numpy()

    ax.plot(time_series, price_series, linewidth=0.5, alpha=0.3, color='gray', zorder=1)

    buy_mask = sides == "BUY"
    sell_mask = sides == "SELL"

    sizes = quantities * 50

    ax.scatter(time_series[buy_mask], price_series[buy_mask],
              c='green', s=sizes[buy_mask], alpha=0.6,
              edgecolors='darkgreen', linewidth=0.5, zorder=2, label='Buy')
    ax.scatter(time_series[sell_mask], price_series[sell_mask],
              c='red', s=sizes[sell_mask], alpha=0.6,
              edgecolors='darkred', linewidth=0.5, zorder=2, label='Sell')

    ax.set_xlabel('Time (seconds)', fontsize=12)
    ax.set_ylabel('Price (USD)', fontsize=12)
    ax.set_title('BTC-PERP Price and Trades', fontsize=14)
    ax.legend(loc='best')
    ax.grid(True, alpha=0.3)

    plt.tight_layout()
    return fig


def main():
    if len(sys.argv) > 1:
        log_file = Path(sys.argv[1])
    else:
        log_file = LOG_FILE

    print(f"Loading data from {log_file}")
    trades_df = load_price_data(log_file)

    time_series = trades_df["time_sec"].to_numpy()
    price_series = trades_df["price_usd"].to_numpy()

    print(f"\nTotal trades: {len(price_series)}")
    print(f"Duration: {time_series[-1]:.1f} seconds")
    print(f"Price range: ${price_series.min():.2f} - ${price_series.max():.2f}")

    print("\n=== Running Statistical Tests ===")
    results, returns = test_random_walk(price_series)

    print(f"\n1. ADF Test (Unit Root)")
    print(f"   Statistic: {results['adf_statistic']:.4f}")
    print(f"   p-value: {results['adf_pvalue']:.4f}")
    print(f"   Result: {'PASS' if results['adf_pass'] else 'FAIL'} (p > 0.05 indicates random walk)")

    print(f"\n2. ACF Test")
    print(f"   Significant lags: {results['acf_significant_lags']}/40")
    print(f"   Result: {'PASS' if results['acf_pass'] else 'FAIL'} (<=2 expected)")

    print(f"\n3. Shapiro-Wilk (Normality)")
    print(f"   Statistic: {results['shapiro_statistic']:.4f}")
    print(f"   p-value: {results['shapiro_pvalue']:.4f}")
    print(f"   Result: {'PASS' if results['shapiro_pass'] else 'FAIL'} (p > 0.05 indicates normal)")

    print(f"\n4. Variance Ratio Test")
    for k, vr in results['variance_ratios'].items():
        print(f"   k={k}: {vr:.4f} (expect ~1.0)")
    print(f"   Result: {'PASS' if results['vr_pass'] else 'FAIL'} (all in [0.8, 1.2])")

    print(f"\n5. Runs Test")
    print(f"   p-value: {results['runs_pvalue']:.4f}")
    print(f"   Result: {'PASS' if results['runs_pass'] else 'FAIL'} (p > 0.05 indicates randomness)")

    overall_pass = sum([
        results['adf_pass'],
        results['acf_pass'],
        results['shapiro_pass'],
        results['vr_pass'],
        results['runs_pass']
    ])
    print(f"\n=== OVERALL: {overall_pass}/5 tests passed ===")

    if overall_pass >= 4:
        print("CONCLUSION: Price behavior is consistent with random walk hypothesis")
    else:
        print("CONCLUSION: Price behavior deviates from random walk")

    print(f"\n=== Returns Summary ===")
    print(f"Mean: {returns.mean():.6f} (expect ~0)")
    print(f"Std: {returns.std():.6f}")
    print(f"Skewness: {stats.skew(returns):.4f} (expect ~0)")
    print(f"Kurtosis: {stats.kurtosis(returns):.4f} (expect ~0)")

    fig = visualize_results(trades_df)
    fig.savefig(OUTPUT_FILE, dpi=150)
    print(f"\nVisualization saved to {OUTPUT_FILE}")


if __name__ == "__main__":
    main()
