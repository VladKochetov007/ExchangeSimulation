package executionlab

import (
	"time"

	"exchange_sim/exchange"
	"exchange_sim/simulations/feesim"
)

type InstrumentContract struct {
	Symbol         string `json:"symbol"`
	BaseAsset      string `json:"base_asset"`
	QuoteAsset     string `json:"quote_asset"`
	BasePrecision  int64  `json:"base_precision"`
	QuotePrecision int64  `json:"quote_precision"`
	TickSize       int64  `json:"tick_size"`
	LotSize        int64  `json:"lot_size"`
}

type AccountContract struct {
	ClientID        uint64                 `json:"client_id"`
	Role            string                 `json:"role"`
	InitialBalances map[string]int64       `json:"initial_balances"`
	Fee             exchange.PercentageFee `json:"fee"`
}

type MakerContract struct {
	ClientID              uint64          `json:"client_id"`
	Config                feesim.MMConfig `json:"config"`
	RealizedLevelCadences []time.Duration `json:"realized_level_cadences_nanos"`
	Latency               time.Duration   `json:"latency_nanos"`
}

type NoiseContract struct {
	ClientID            uint64        `json:"client_id"`
	Symbol              string        `json:"symbol"`
	TargetQty           int64         `json:"target_qty"`
	TakeInterval        time.Duration `json:"take_interval_nanos"`
	DecisionPhaseOffset time.Duration `json:"decision_phase_offset_nanos"`
	Seed                int64         `json:"seed"`
	Latency             time.Duration `json:"latency_nanos"`
	ImbalanceCoupling   float64       `json:"imbalance_coupling"`
	ExciteAlpha         float64       `json:"excite_alpha"`
	ExciteBetaPerSec    float64       `json:"excite_beta_per_sec"`
	SizeParetoAlpha     float64       `json:"size_pareto_alpha"`
	SizeCapMultiple     float64       `json:"size_cap_multiple"`
}

type ParentContract struct {
	ClientID   uint64            `json:"client_id"`
	Config     ParentOrderConfig `json:"config"`
	Latency    time.Duration     `json:"latency_nanos"`
	Deployment *ParentDeployment `json:"deployment,omitempty"`
}

type RunnerContract struct {
	Iterations           int           `json:"iterations"`
	Step                 time.Duration `json:"step_nanos"`
	DeterministicIngress bool          `json:"deterministic_ingress"`
	DeterministicPhases  bool          `json:"deterministic_phases"`
}

type WorldContract struct {
	SchemaVersion int                `json:"schema_version"`
	Config        SimConfig          `json:"config"`
	Instrument    InstrumentContract `json:"instrument"`
	Bootstrap     int64              `json:"bootstrap_price"`
	Accounts      []AccountContract  `json:"accounts"`
	Makers        []MakerContract    `json:"makers"`
	Noise         []NoiseContract    `json:"noise"`
	Parents       []ParentContract   `json:"parents"`
	Runner        RunnerContract     `json:"runner"`
}

func (c WorldContract) clone() WorldContract {
	c.Accounts = append([]AccountContract(nil), c.Accounts...)
	for index := range c.Accounts {
		balances := make(map[string]int64, len(c.Accounts[index].InitialBalances))
		for asset, balance := range c.Accounts[index].InitialBalances {
			balances[asset] = balance
		}
		c.Accounts[index].InitialBalances = balances
	}
	c.Makers = append([]MakerContract(nil), c.Makers...)
	for index := range c.Makers {
		c.Makers[index].RealizedLevelCadences = append([]time.Duration(nil), c.Makers[index].RealizedLevelCadences...)
	}
	c.Noise = append([]NoiseContract(nil), c.Noise...)
	c.Parents = append([]ParentContract(nil), c.Parents...)
	for index := range c.Parents {
		if c.Parents[index].Deployment != nil {
			copyOfDeployment := *c.Parents[index].Deployment
			c.Parents[index].Deployment = &copyOfDeployment
		}
		if c.Parents[index].Config.Instruction != nil {
			copyOfInstruction := *c.Parents[index].Config.Instruction
			c.Parents[index].Config.Instruction = &copyOfInstruction
		}
	}
	if c.Config.ParentDeployment != nil {
		copyOfDeployment := *c.Config.ParentDeployment
		c.Config.ParentDeployment = &copyOfDeployment
	}
	if c.Config.Parent.Instruction != nil {
		copyOfInstruction := *c.Config.Parent.Instruction
		c.Config.Parent.Instruction = &copyOfInstruction
	}
	return c
}
