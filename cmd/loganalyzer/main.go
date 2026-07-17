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
	midSeries     map[int64]float64
}

// Report is the analyzer output for one experiment run.
type Report struct {
	Symbols      map[string]*SymbolMetrics   `json:"symbols"`
	ClientDeltas map[string]map[string]int64 `json:"client_deltas"`              // clientID -> asset -> Δ(total incl perp+locked-borrowed)
	SpotResidual map[string]int64            `json:"spot_conservation_residual"` // asset -> Σclient Δ (spot only, ex fees)
	BasisRmsBps  map[string]float64          `json:"basis_rms_bps"`              // perp symbol -> RMS mid basis vs spot
	Rejects      int64                       `json:"order_rejects"`
	Liquidations int64                       `json:"liquidation_checks_logged"`
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

	m := &SymbolMetrics{midSeries: make(map[int64]float64)}
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
	return m, nil
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

	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, b, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))
}
