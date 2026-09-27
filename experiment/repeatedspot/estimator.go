package repeatedspot

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"

	worldspot "exchange_sim/simulations/repeatedspot"
)

type makerParameters struct {
	kind                string
	workingLimit        int64
	quoteIntervalNanos  int64
	tickSize            int64
	initialVariance     float64
	halfLifeNanos       int64
	sampleIntervalNanos int64
	maxVarianceMultiple float64
	spreadBps           int64
	quotePrecision      int64
	riskAversion        float64
	fillDecay           float64
	horizonNanos        int64
	minHalfSpreadTicks  int64
}

type deliveredVariance struct {
	value        float64
	lastPrice    int64
	lastSourceAt int64
	samples      uint64
}

func parseMakerParameters(participant replayParticipant) (*makerParameters, error) {
	if participant.Policy.Name != "bounded_fixed_maker_v2" && participant.Policy.Name != "bounded_stoikov_maker_v2" {
		return nil, nil
	}
	var raw struct {
		Maker struct {
			WorkingLimit                int64   `json:"working_limit"`
			QuoteInterval               int64   `json:"quote_interval_ns"`
			TickSize                    int64   `json:"tick_size"`
			InitialLogVariancePerSecond float64 `json:"initial_log_variance_per_second"`
			VolatilityHalfLife          int64   `json:"volatility_half_life_ns"`
			VolatilitySampleInterval    int64   `json:"volatility_sample_interval_ns"`
			MaxLogVarianceMultiple      float64 `json:"max_log_variance_multiple"`
		} `json:"maker"`
		SpreadBps          int64   `json:"spread_bps"`
		QuotePrecision     int64   `json:"quote_precision"`
		RelativeRisk       float64 `json:"relative_risk_aversion"`
		RelativeFillDecay  float64 `json:"relative_fill_decay"`
		InventoryHorizon   int64   `json:"inventory_horizon_ns"`
		MinHalfSpreadTicks int64   `json:"min_half_spread_ticks"`
	}
	if err := json.Unmarshal(participant.Policy.Parameters, &raw); err != nil {
		return nil, err
	}
	parameters := &makerParameters{kind: participant.Policy.Name,
		workingLimit: raw.Maker.WorkingLimit, quoteIntervalNanos: raw.Maker.QuoteInterval,
		tickSize:        raw.Maker.TickSize,
		initialVariance: raw.Maker.InitialLogVariancePerSecond,
		halfLifeNanos:   raw.Maker.VolatilityHalfLife, sampleIntervalNanos: raw.Maker.VolatilitySampleInterval,
		maxVarianceMultiple: raw.Maker.MaxLogVarianceMultiple, spreadBps: raw.SpreadBps,
		quotePrecision: raw.QuotePrecision, riskAversion: raw.RelativeRisk,
		fillDecay: raw.RelativeFillDecay, horizonNanos: raw.InventoryHorizon,
		minHalfSpreadTicks: raw.MinHalfSpreadTicks}
	if parameters.workingLimit <= 0 || parameters.quoteIntervalNanos <= 0 || parameters.tickSize <= 0 ||
		!finiteNumber(parameters.initialVariance) || parameters.initialVariance < 0 ||
		parameters.halfLifeNanos < 0 || parameters.sampleIntervalNanos < 0 ||
		!finiteNumber(parameters.maxVarianceMultiple) || parameters.maxVarianceMultiple < 0 ||
		!finiteNumber(parameters.initialVariance*parameters.maxVarianceMultiple) {
		return nil, errors.New("repeated spot: malformed maker estimator parameters")
	}
	if parameters.kind == "bounded_fixed_maker_v2" && (parameters.spreadBps < 0 || parameters.spreadBps >= 10_000) {
		return nil, errors.New("repeated spot: malformed fixed-maker spread")
	}
	if parameters.kind == "bounded_stoikov_maker_v2" &&
		(parameters.quotePrecision <= 0 || parameters.riskAversion <= 0 || parameters.fillDecay <= 0 ||
			parameters.horizonNanos <= 0 || parameters.minHalfSpreadTicks <= 0 ||
			!finiteNumber(parameters.riskAversion) || !finiteNumber(parameters.fillDecay)) {
		return nil, errors.New("repeated spot: malformed AS-style quote parameters")
	}
	return parameters, nil
}

func (estimate *deliveredVariance) observe(parameters *makerParameters, observation worldspot.MakerObservation) error {
	if observation.TradePrice <= 0 || observation.SourceAt < 0 ||
		estimate.lastPrice != 0 && observation.SourceAt < estimate.lastSourceAt {
		return errors.New("repeated spot: delivered trade estimator received an invalid price or regressed source time")
	}
	if estimate.lastPrice != 0 && observation.SourceAt-estimate.lastSourceAt < parameters.sampleIntervalNanos {
		return nil
	}
	if estimate.lastPrice != 0 && observation.SourceAt > estimate.lastSourceAt {
		seconds := float64(observation.SourceAt-estimate.lastSourceAt) / 1e9
		logMove := math.Log(float64(observation.TradePrice) / float64(estimate.lastPrice))
		newVariance := logMove * logMove / seconds
		weight := 1.0
		if parameters.halfLifeNanos > 0 {
			weight = 1 - math.Exp(-math.Ln2*float64(observation.SourceAt-estimate.lastSourceAt)/float64(parameters.halfLifeNanos))
		}
		estimate.value += weight * (newVariance - estimate.value)
		if parameters.maxVarianceMultiple > 0 && parameters.initialVariance > 0 {
			estimate.value = math.Min(estimate.value, parameters.maxVarianceMultiple*parameters.initialVariance)
		}
		if !finiteNumber(estimate.value) || estimate.value < 0 {
			return errors.New("repeated spot: delivered variance is non-finite")
		}
	}
	estimate.lastPrice, estimate.lastSourceAt = observation.TradePrice, observation.SourceAt
	estimate.samples++
	return nil
}

func (estimate deliveredVariance) verifyDecision(decision worldspot.MakerDecision) error {
	tolerance := math.Max(1e-20, math.Abs(estimate.value)*1e-9)
	if decision.DeliveredTradeSamples != estimate.samples || !finiteNumber(decision.LogVariancePerSecond) ||
		math.Abs(decision.LogVariancePerSecond-estimate.value) > tolerance {
		return fmt.Errorf("repeated spot: maker decision variance/samples differ from delivered-trade reconstruction")
	}
	return nil
}

func expectedMakerQuote(parameters *makerParameters, decision worldspot.MakerDecision) (int64, int64, bool) {
	if decision.BestBid <= 0 || decision.BestAsk <= decision.BestBid {
		return 0, 0, false
	}
	mid := decision.BestBid + (decision.BestAsk-decision.BestBid)/2
	if parameters.kind == "bounded_fixed_maker_v2" {
		halfSpread := new(big.Int).Mul(big.NewInt(mid), big.NewInt(parameters.spreadBps))
		halfSpread.Quo(halfSpread, big.NewInt(10_000))
		if !halfSpread.IsInt64() || halfSpread.Int64() >= mid {
			return 0, 0, false
		}
		unroundedAsk, ok := checkedAdd(mid, halfSpread.Int64())
		if !ok {
			return 0, 0, false
		}
		bid := (mid - halfSpread.Int64()) / parameters.tickSize * parameters.tickSize
		ask := unroundedAsk / parameters.tickSize * parameters.tickSize
		if unroundedAsk%parameters.tickSize != 0 {
			ask, ok = checkedAdd(ask, parameters.tickSize)
		}
		return bid, ask, ok && bid > 0 && ask > bid
	}
	forward := float64(mid) / float64(parameters.quotePrecision)
	if forward <= 0 {
		return 0, 0, false
	}
	gamma := parameters.riskAversion / forward
	kappa := parameters.fillDecay / forward
	priceVariance := decision.LogVariancePerSecond * forward * forward
	risk := gamma * priceVariance * float64(parameters.horizonNanos) / 1e9
	reservation := forward - float64(decision.FilledInventory)/float64(parameters.workingLimit)*risk
	halfSpread := math.Max(risk/2+math.Log1p(gamma/kappa)/gamma,
		float64(parameters.minHalfSpreadTicks)*float64(parameters.tickSize)/float64(parameters.quotePrecision))
	if !finiteNumber(reservation) || !finiteNumber(halfSpread) {
		return 0, 0, false
	}
	bid := roundedQuoteTicks(reservation-halfSpread, parameters.quotePrecision, parameters.tickSize, false)
	ask := roundedQuoteTicks(reservation+halfSpread, parameters.quotePrecision, parameters.tickSize, true)
	return bid, ask, bid > 0 && ask > bid
}

func roundedQuoteTicks(price float64, precision, tickSize int64, up bool) int64 {
	if !finiteNumber(price) || price <= 0 || precision <= 0 || tickSize <= 0 {
		return 0
	}
	ticks := price * float64(precision) / float64(tickSize)
	if up {
		ticks = math.Ceil(ticks)
	} else {
		ticks = math.Floor(ticks)
	}
	if !finiteNumber(ticks) || ticks <= 0 || ticks >= float64(math.MaxInt64)/float64(tickSize) {
		return 0
	}
	return int64(ticks) * tickSize
}

func finiteNumber(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
