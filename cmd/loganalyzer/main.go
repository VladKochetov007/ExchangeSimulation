package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Raw JSONL envelope written by feesim.JSONLinesLogger.
type logLine struct {
	SimTS    int64           `json:"sim_ts"`
	ClientID uint64          `json:"client_id"`
	Event    string          `json:"event"`
	Data     json.RawMessage `json:"data"`
}

type priceLevel struct {
	Price      int64 `json:"price"`
	VisibleQty int64 `json:"visible_qty"`
}

type bookSnapshot struct {
	Bids []priceLevel `json:"bids"`
	Asks []priceLevel `json:"asks"`
}

type trade struct {
	TradeID uint64 `json:"trade_id"`
	Price   int64  `json:"price"`
	Qty     int64  `json:"qty"`
	// Side is the TAKER's side, which is what makes order flow signable:
	// a buy-initiated trade is demand lifting the offer.
	Side string `json:"side"`
}

type assetBalance struct {
	Asset    string `json:"asset"`
	Free     int64  `json:"free"`
	Locked   int64  `json:"locked"`
	Borrowed int64  `json:"borrowed"`
	Interest int64  `json:"interest"`
}

type balanceSnapshot struct {
	ClientID     uint64         `json:"client_id"`
	SpotBalances []assetBalance `json:"spot_balances"`
	PerpBalances []assetBalance `json:"perp_balances"`
}

// SymbolMetrics summarizes market quality for one instrument.
type SymbolMetrics struct {
	Trades        int64   `json:"trades"`
	VolumeQty     int64   `json:"volume_qty"`
	VWAP          float64 `json:"vwap"`
	LastPrice     int64   `json:"last_price"`
	MeanSpreadBps float64 `json:"mean_spread_bps"`
	P95SpreadBps  float64 `json:"p95_spread_bps"`
	MeanDepthBid  float64 `json:"mean_depth_bid"`
	MeanDepthAsk  float64 `json:"mean_depth_ask"`
	Snapshots     int64   `json:"snapshots"`
	EmptySideSnap int64   `json:"empty_side_snapshots"`
	MidStdBps     float64 `json:"mid_std_bps"`
	TradeHash     string  `json:"trade_hash"`

	// Stylized-fact statistics on 1s mid log-returns (Cont 2001 checklist).
	ReturnKurtosis float64 `json:"return_excess_kurtosis"`
	ACFR1          float64 `json:"acf_r_lag1"`
	ACFAbsR1       float64 `json:"acf_absr_lag1"`
	ACFAbsR5       float64 `json:"acf_absr_lag5"`
	ACFAbsR10      float64 `json:"acf_absr_lag10"`
	TradeFano      float64 `json:"trade_count_fano"`

	// Order-flow imbalance against contemporaneous returns. Out-of-sample:
	// no parameter in the ecology was tuned against it, so unlike kurtosis
	// or the Fano factor it is evidence rather than a restated input. A
	// healthy market gives a positive beta — net buying pressure moves price
	// up — and a value near zero would say price formation is decoupled from
	// flow, which for an order-driven market means something is wrong.
	OFIBeta float64 `json:"ofi_return_beta_bps_per_unit"`
	OFICorr float64 `json:"ofi_return_corr"`
	OFIObs  int     `json:"ofi_observations"`

	// Cumulative price response to order flow, index k = seconds after the
	// flow. A contemporaneous correlation cannot tell informed flow from
	// mechanical impact: both move price with the trade. They separate over
	// time. Flow that carried information leaves the price where it put it,
	// so the curve stays flat; flow that merely consumed resting liquidity
	// is refilled by market makers and the curve decays back toward zero.
	OFIResponseBps  []float64 `json:"ofi_cumulative_response_bps_per_unit"`
	OFIResponseCorr []float64 `json:"ofi_cumulative_response_corr"`

	midSeries   map[int64]float64
	tradeCounts map[int64]int64
	// signedFlow is taker-signed volume per second: buys positive.
	signedFlow map[int64]int64
}

// Report is the analyzer output for one experiment run.
type Report struct {
	Symbols      map[string]*SymbolMetrics   `json:"symbols"`
	ClientDeltas map[string]map[string]int64 `json:"client_deltas"`              // clientID -> asset -> Δ(total incl perp+locked-borrowed)
	SpotResidual map[string]int64            `json:"spot_conservation_residual"` // asset -> Σclient Δ (spot only, ex fees)
	BasisRmsBps  map[string]float64          `json:"basis_rms_bps"`              // perp symbol -> RMS mid basis vs spot
	// Epps: cross-asset return correlation at increasing sampling intervals.
	// Measured correlation should RISE with the interval purely because
	// trades arrive asynchronously, with no change in the underlying
	// dependence. Out-of-sample — nothing in the ecology is tuned to it —
	// so a rising profile is evidence the microstructure is behaving, and a
	// flat one says the assets are either perfectly synchronous or unlinked.
	EppsCorrByInterval map[string]float64 `json:"epps_corr_by_interval_sec"`
	Rejects            int64              `json:"order_rejects"`
	Liquidations       int64              `json:"liquidation_checks_logged"`
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}

func analyzeSymbolFile(path string) (*SymbolMetrics, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m := &SymbolMetrics{
		midSeries:   make(map[int64]float64),
		tradeCounts: make(map[int64]int64),
		signedFlow:  make(map[int64]int64),
	}
	hasher := sha256.New()
	var spreads, mids, depthB, depthA []float64
	var notional float64

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for sc.Scan() {
		var ln logLine
		if err := json.Unmarshal(sc.Bytes(), &ln); err != nil {
			continue
		}
		switch ln.Event {
		case "Trade":
			var t trade
			if json.Unmarshal(ln.Data, &t) != nil {
				continue
			}
			m.Trades++
			m.VolumeQty += t.Qty
			m.LastPrice = t.Price
			m.tradeCounts[ln.SimTS/1e9]++
			if t.Side == "SELL" {
				m.signedFlow[ln.SimTS/1e9] -= t.Qty
			} else {
				m.signedFlow[ln.SimTS/1e9] += t.Qty
			}
			notional += float64(t.Price) * float64(t.Qty)
			fmt.Fprintf(hasher, "%d|%d|%d|%d\n", ln.SimTS, t.TradeID, t.Price, t.Qty)
		case "BookSnapshot":
			var b bookSnapshot
			if json.Unmarshal(ln.Data, &b) != nil {
				continue
			}
			m.Snapshots++
			if len(b.Bids) == 0 || len(b.Asks) == 0 {
				m.EmptySideSnap++
				continue
			}
			bid, ask := b.Bids[0].Price, b.Asks[0].Price
			mid := float64(bid+ask) / 2
			spreads = append(spreads, float64(ask-bid)/mid*10000)
			mids = append(mids, mid)
			m.midSeries[ln.SimTS/1e9] = mid
			var db, da float64
			for _, l := range b.Bids {
				db += float64(l.VisibleQty)
			}
			for _, l := range b.Asks {
				da += float64(l.VisibleQty)
			}
			depthB = append(depthB, db)
			depthA = append(depthA, da)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	if m.VolumeQty > 0 {
		m.VWAP = notional / float64(m.VolumeQty)
	}
	if len(spreads) > 0 {
		var sum float64
		for _, s := range spreads {
			sum += s
		}
		m.MeanSpreadBps = sum / float64(len(spreads))
		sorted := append([]float64(nil), spreads...)
		sort.Float64s(sorted)
		m.P95SpreadBps = percentile(sorted, 0.95)
	}
	if len(mids) > 1 {
		var mean, varSum float64
		for _, v := range mids {
			mean += v
		}
		mean /= float64(len(mids))
		for _, v := range mids {
			varSum += (v - mean) * (v - mean)
		}
		m.MidStdBps = math.Sqrt(varSum/float64(len(mids))) / mean * 10000
	}
	mean := func(xs []float64) float64 {
		if len(xs) == 0 {
			return 0
		}
		var s float64
		for _, x := range xs {
			s += x
		}
		return s / float64(len(xs))
	}
	m.MeanDepthBid = mean(depthB)
	m.MeanDepthAsk = mean(depthA)
	m.TradeHash = hex.EncodeToString(hasher.Sum(nil))[:16]
	m.computeStylizedFacts()
	return m, nil
}

// computeStylizedFacts fills return-distribution statistics from the 1s mid
// series and the per-second trade counts.
func (m *SymbolMetrics) computeStylizedFacts() {
	secs := make([]int64, 0, len(m.midSeries))
	for ts := range m.midSeries {
		secs = append(secs, ts)
	}
	sort.Slice(secs, func(i, j int) bool { return secs[i] < secs[j] })

	// Log returns over consecutive seconds only (gaps break the chain).
	var rets []float64
	for i := 1; i < len(secs); i++ {
		if secs[i] != secs[i-1]+1 {
			continue
		}
		prev, cur := m.midSeries[secs[i-1]], m.midSeries[secs[i]]
		if prev > 0 && cur > 0 {
			rets = append(rets, math.Log(cur/prev))
		}
	}
	// Regress the same-second return on signed order flow. Both series are
	// built from the consecutive-second pairs above so they stay aligned.
	var flows, flowRets []float64
	for i := 1; i < len(secs); i++ {
		if secs[i] != secs[i-1]+1 {
			continue
		}
		prev, cur := m.midSeries[secs[i-1]], m.midSeries[secs[i]]
		if prev <= 0 || cur <= 0 {
			continue
		}
		flows = append(flows, float64(m.signedFlow[secs[i]]))
		flowRets = append(flowRets, math.Log(cur/prev)*10000)
	}
	if len(flows) >= 30 {
		m.OFIBeta, m.OFICorr = linregress(flows, flowRets)
		m.OFIObs = len(flows)
	}
	m.computeImpactResponse(secs)

	if len(rets) >= 30 {
		m.ReturnKurtosis = excessKurtosis(rets)
		m.ACFR1 = autocorr(rets, 1)
		abs := make([]float64, len(rets))
		for i, r := range rets {
			abs[i] = math.Abs(r)
		}
		m.ACFAbsR1 = autocorr(abs, 1)
		m.ACFAbsR5 = autocorr(abs, 5)
		m.ACFAbsR10 = autocorr(abs, 10)
	}

	if len(m.tradeCounts) > 0 {
		var minSec, maxSec int64 = math.MaxInt64, math.MinInt64
		for ts := range m.tradeCounts {
			if ts < minSec {
				minSec = ts
			}
			if ts > maxSec {
				maxSec = ts
			}
		}
		n := float64(maxSec - minSec + 1)
		var sum, sumSq float64
		for _, c := range m.tradeCounts {
			sum += float64(c)
			sumSq += float64(c) * float64(c)
		}
		meanC := sum / n
		if meanC > 0 {
			m.TradeFano = (sumSq/n - meanC*meanC) / meanC
		}
	}
}

// computeImpactResponse builds the cumulative price-response curve: for each
// horizon k, regress the total return over the k seconds following a second's
// order flow on that flow. Only windows whose seconds are all present are
// used, so a logging gap cannot be read as a large move.
func (m *SymbolMetrics) computeImpactResponse(secs []int64) {
	const maxLag = 5
	retAt := make(map[int64]float64, len(secs))
	for i := 1; i < len(secs); i++ {
		if secs[i] != secs[i-1]+1 {
			continue
		}
		prev, cur := m.midSeries[secs[i-1]], m.midSeries[secs[i]]
		if prev > 0 && cur > 0 {
			retAt[secs[i]] = math.Log(cur/prev) * 10000
		}
	}

	m.OFIResponseBps = make([]float64, 0, maxLag+1)
	m.OFIResponseCorr = make([]float64, 0, maxLag+1)
	for k := 0; k <= maxLag; k++ {
		var xs, ys []float64
		for _, t := range secs {
			flow, ok := m.signedFlow[t]
			if !ok {
				continue
			}
			// Require every second of the window to be present.
			cum, complete := 0.0, true
			for j := 0; j <= k; j++ {
				r, ok := retAt[t+int64(j)]
				if !ok {
					complete = false
					break
				}
				cum += r
			}
			if !complete {
				continue
			}
			xs = append(xs, float64(flow))
			ys = append(ys, cum)
		}
		if len(xs) < 30 {
			break
		}
		beta, corr := linregress(xs, ys)
		m.OFIResponseBps = append(m.OFIResponseBps, beta)
		m.OFIResponseCorr = append(m.OFIResponseCorr, corr)
	}
}

// linregress returns the slope of ys on xs and their correlation. Slope
// answers "how far does price move per unit of net flow"; correlation
// answers "how reliably", and the two together are what distinguish a real
// impact relationship from a coincidence of scale.
func linregress(xs, ys []float64) (slope, corr float64) {
	n := float64(len(xs))
	if n < 2 {
		return 0, 0
	}
	var mx, my float64
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	mx /= n
	my /= n
	var sxy, sxx, syy float64
	for i := range xs {
		dx, dy := xs[i]-mx, ys[i]-my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 || syy == 0 {
		return 0, 0
	}
	return sxy / sxx, sxy / math.Sqrt(sxx*syy)
}

func excessKurtosis(xs []float64) float64 {
	var mean float64
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	var m2, m4 float64
	for _, x := range xs {
		d := x - mean
		m2 += d * d
		m4 += d * d * d * d
	}
	m2 /= float64(len(xs))
	m4 /= float64(len(xs))
	if m2 == 0 {
		return 0
	}
	return m4/(m2*m2) - 3
}

func autocorr(xs []float64, lag int) float64 {
	if len(xs) <= lag {
		return 0
	}
	var mean float64
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	var num, den float64
	for i := 0; i < len(xs); i++ {
		d := xs[i] - mean
		den += d * d
		if i+lag < len(xs) {
			num += d * (xs[i+lag] - mean)
		}
	}
	if den == 0 {
		return 0
	}
	return num / den
}

// analyzeGeneral extracts client balance deltas and reject/liquidation counts.
func analyzeGeneral(path string, rep *Report) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	first := make(map[uint64]map[string]int64)
	last := make(map[uint64]map[string]int64)
	firstSpot := make(map[uint64]map[string]int64)
	lastSpot := make(map[uint64]map[string]int64)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for sc.Scan() {
		var ln logLine
		if err := json.Unmarshal(sc.Bytes(), &ln); err != nil {
			continue
		}
		switch ln.Event {
		case "balance_snapshot":
			var b balanceSnapshot
			if json.Unmarshal(ln.Data, &b) != nil {
				continue
			}
			total := make(map[string]int64)
			spotOnly := make(map[string]int64)
			for _, ab := range b.SpotBalances {
				net := ab.Free + ab.Locked - ab.Borrowed - ab.Interest
				total[ab.Asset] += net
				spotOnly[ab.Asset] += net
			}
			for _, ab := range b.PerpBalances {
				total[ab.Asset] += ab.Free + ab.Locked
			}
			if _, ok := first[b.ClientID]; !ok {
				first[b.ClientID] = total
				firstSpot[b.ClientID] = spotOnly
			}
			last[b.ClientID] = total
			lastSpot[b.ClientID] = spotOnly
		case "OrderRejected":
			rep.Rejects++
		case "liquidation_check":
			rep.Liquidations++
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	rep.ClientDeltas = make(map[string]map[string]int64)
	rep.SpotResidual = make(map[string]int64)
	for cid, lastBal := range last {
		deltas := make(map[string]int64)
		for asset, v := range lastBal {
			deltas[asset] = v - first[cid][asset]
		}
		rep.ClientDeltas[fmt.Sprintf("%d", cid)] = deltas
		for asset, v := range lastSpot[cid] {
			rep.SpotResidual[asset] += v - firstSpot[cid][asset]
		}
	}
	return nil
}

// computeBasis fills BasisRmsBps: RMS of (perpMid − spotMid)/spotMid per aligned second.
func computeBasis(rep *Report, pairs map[string]string) {
	rep.BasisRmsBps = make(map[string]float64)
	for perpSym, spotSym := range pairs {
		p, pok := rep.Symbols[perpSym]
		s, sok := rep.Symbols[spotSym]
		if !pok || !sok {
			continue
		}
		var sumSq float64
		var n int
		for ts, pm := range p.midSeries {
			if sm, ok := s.midSeries[ts]; ok && sm > 0 {
				d := (pm - sm) / sm * 10000
				sumSq += d * d
				n++
			}
		}
		if n > 0 {
			rep.BasisRmsBps[perpSym] = math.Sqrt(sumSq / float64(n))
		}
	}
}

// computeEpps fills EppsCorrByInterval for the first two symbols that have
// mid series, sampling returns at 1, 2, 5, 10, 30 and 60 second intervals.
func computeEpps(rep *Report, symA, symB string) {
	a, aok := rep.Symbols[symA]
	b, bok := rep.Symbols[symB]
	if !aok || !bok {
		return
	}
	rep.EppsCorrByInterval = make(map[string]float64)
	for _, step := range []int64{1, 2, 5, 10, 30, 60} {
		ra := sampledReturns(a.midSeries, step)
		rb := sampledReturns(b.midSeries, step)
		n := min(len(ra), len(rb))
		if n < 30 {
			continue
		}
		_, corr := linregress(ra[:n], rb[:n])
		rep.EppsCorrByInterval[fmt.Sprintf("%d", step)] = corr
	}
}

// sampledReturns builds log returns over a fixed sampling interval, skipping
// windows where either endpoint is missing so a gap cannot masquerade as a
// large move.
func sampledReturns(series map[int64]float64, step int64) []float64 {
	if len(series) == 0 {
		return nil
	}
	var lo, hi int64
	first := true
	for ts := range series {
		if first || ts < lo {
			lo = ts
		}
		if first || ts > hi {
			hi = ts
		}
		first = false
	}
	var out []float64
	for t := lo; t+step <= hi; t += step {
		p0, ok0 := series[t]
		p1, ok1 := series[t+step]
		if !ok0 || !ok1 || p0 <= 0 || p1 <= 0 {
			continue
		}
		out = append(out, math.Log(p1/p0))
	}
	return out
}

func main() {
	dir := flag.String("dir", "", "experiment log directory (contains spot/, perp/, general.jsonl)")
	out := flag.String("out", "", "output metrics JSON path (default: <dir>/metrics.json)")
	flag.Parse()
	if *dir == "" {
		log.Fatal("-dir required")
	}
	if *out == "" {
		*out = filepath.Join(*dir, "metrics.json")
	}

	rep := &Report{Symbols: make(map[string]*SymbolMetrics)}
	for _, sub := range []string{"spot", "perp"} {
		files, _ := filepath.Glob(filepath.Join(*dir, sub, "*.jsonl"))
		for _, fp := range files {
			sym := strings.TrimSuffix(filepath.Base(fp), ".jsonl")
			if sub == "spot" {
				sym = strings.Replace(sym, "-", "/", 1)
			}
			m, err := analyzeSymbolFile(fp)
			if err != nil {
				log.Fatalf("%s: %v", fp, err)
			}
			rep.Symbols[sym] = m
		}
	}
	if err := analyzeGeneral(filepath.Join(*dir, "general.jsonl"), rep); err != nil {
		log.Fatalf("general.jsonl: %v", err)
	}
	computeBasis(rep, map[string]string{"ABC-PERP": "ABC/USD", "Q-PERP": "Q/USD"})
	// Two independently-traded USD spot books: the Epps profile is about the
	// asynchrony between them, so a perp and its own spot would not test it.
	computeEpps(rep, "ABC/USD", "Q/USD")

	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, b, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))
}
