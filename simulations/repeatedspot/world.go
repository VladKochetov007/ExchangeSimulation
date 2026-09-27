// Package repeatedspot composes a deterministic, one-book spot ecology from
// externally supplied actor policies. It is an implementation fixture, not a
// registered economic study or a source of market-realism claims.
package repeatedspot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

type InstrumentConfig struct {
	Symbol         string `json:"symbol"`
	BaseAsset      string `json:"base_asset"`
	QuoteAsset     string `json:"quote_asset"`
	BasePrecision  int64  `json:"base_precision"`
	QuotePrecision int64  `json:"quote_precision"`
	TickSize       int64  `json:"tick_size"`
	MinOrderSize   int64  `json:"min_order_size"`
}

type Deployment struct {
	RequestLatency    time.Duration `json:"request_latency_ns"`
	ResponseLatency   time.Duration `json:"response_latency_ns"`
	MarketDataLatency time.Duration `json:"market_data_latency_ns"`
}

// PolicyDefinition snapshots JSON-serializable parameters before an actor is
// built. A caller can inject a new policy without changing this package.
type PolicyDefinition struct {
	name       string
	parameters json.RawMessage
	build      func(uint64, actor.Gateway, exchange.TickerFactory, json.RawMessage) (actor.Actor, error)
}

func DefinePolicy[T any](name string, parameters T, build func(uint64, actor.Gateway, exchange.TickerFactory, T) (actor.Actor, error)) (PolicyDefinition, error) {
	if name == "" || build == nil {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: policy name and builder are required")
	}
	encoded, err := json.Marshal(parameters)
	if err != nil {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: encode policy %q: %w", name, err)
	}
	var roundTrip T
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: policy %q parameters do not round-trip: %w", name, err)
	}
	return PolicyDefinition{name: name, parameters: encoded, build: func(id uint64, gateway actor.Gateway, timers exchange.TickerFactory, raw json.RawMessage) (actor.Actor, error) {
		var decoded T
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, err
		}
		return build(id, gateway, timers, decoded)
	}}, nil
}

type FeeDefinition struct {
	name       string
	parameters json.RawMessage
	build      func(json.RawMessage) (exchange.FeeModel, error)
}

func DefineFee[T any](name string, parameters T, build func(T) (exchange.FeeModel, error)) (FeeDefinition, error) {
	if name == "" || build == nil {
		return FeeDefinition{}, fmt.Errorf("repeatedspot: fee name and builder are required")
	}
	encoded, err := json.Marshal(parameters)
	if err != nil {
		return FeeDefinition{}, fmt.Errorf("repeatedspot: encode fee %q: %w", name, err)
	}
	var roundTrip T
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		return FeeDefinition{}, fmt.Errorf("repeatedspot: fee %q parameters do not round-trip: %w", name, err)
	}
	return FeeDefinition{name: name, parameters: encoded, build: func(raw json.RawMessage) (exchange.FeeModel, error) {
		var decoded T
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, err
		}
		return build(decoded)
	}}, nil
}

type Participant struct {
	ActorID  uint64
	ClientID uint64
	Role     string
	Balances map[string]int64
	Fees     FeeDefinition
	Latency  Deployment
	Policy   PolicyDefinition
}

type Config struct {
	VenueID                          string
	Instrument                       InstrumentConfig
	StartUnixNano                    int64
	Step                             time.Duration
	Iterations                       int
	SnapshotInterval                 time.Duration
	ForbidBorrowing                  bool
	RecordSnapshotProjectionEvidence bool
	Participants                     []Participant
}

type participantContract struct {
	ActorID  uint64           `json:"actor_id"`
	ClientID uint64           `json:"client_id"`
	Role     string           `json:"role"`
	Balances map[string]int64 `json:"balances"`
	Fees     namedDefinition  `json:"fees"`
	Latency  Deployment       `json:"latency"`
	Policy   namedDefinition  `json:"policy"`
}

type namedDefinition struct {
	Name       string          `json:"name"`
	Parameters json.RawMessage `json:"parameters"`
}

type worldContract struct {
	SchemaVersion                    int                   `json:"schema_version"`
	VenueID                          string                `json:"venue_id"`
	Instrument                       InstrumentConfig      `json:"instrument"`
	MatchingRule                     string                `json:"matching_rule"`
	ClockMode                        string                `json:"clock_mode"`
	DeterministicPhases              bool                  `json:"deterministic_phases"`
	PhaseMaxRounds                   int                   `json:"phase_max_rounds"`
	AutomationEnabled                bool                  `json:"automation_enabled"`
	StartUnixNano                    int64                 `json:"start_unix_nano"`
	Step                             time.Duration         `json:"step_ns"`
	Iterations                       int                   `json:"iterations"`
	SnapshotInterval                 time.Duration         `json:"snapshot_interval_ns"`
	ForbidBorrowing                  bool                  `json:"forbid_borrowing"`
	RecordSnapshotProjectionEvidence bool                  `json:"record_snapshot_projection_evidence"`
	Participants                     []participantContract `json:"participants"`
}

const phaseMaxRounds = 100_000

type World struct {
	runner       *simulation.Runner
	mount        *simulation.Mount
	exchange     *exchange.Exchange
	actors       []actor.Actor
	contractJSON []byte
	contractHash string
	runOnce      sync.Once
	runErr       error
	closed       atomic.Bool
	clock        *simulation.SimulatedClock
	expectedEnd  int64
}

// ContractJSON returns a copy of the assembly inputs. It does not attest a
// source revision, external builder implementation, or any research result.
func (world *World) ContractJSON() []byte {
	return append([]byte(nil), world.contractJSON...)
}

func (world *World) ContractSHA256() string { return world.contractHash }

// SimulatedNowUnixNano is an observation-only clock read for evidence sinks.
func (world *World) SimulatedNowUnixNano() int64 { return world.clock.NowUnixNano() }

// Venue and Actors allow externally composed evidence observers to be
// installed before Run. Mutating economic state through these handles is
// outside the exported assembly contract and requires a new protocol identity.
func (world *World) Venue() *exchange.Exchange { return world.exchange }

func (world *World) Actors() []actor.Actor {
	return append([]actor.Actor(nil), world.actors...)
}

func (world *World) Run(ctx context.Context) error {
	if world.closed.Load() {
		return fmt.Errorf("repeatedspot: world is closed")
	}
	called := false
	world.runOnce.Do(func() {
		called = true
		world.runErr = world.runner.Run(ctx)
		if world.runErr == nil && world.clock.NowUnixNano() < world.expectedEnd {
			world.runErr = fmt.Errorf("repeatedspot: incomplete world: stopped at %d before %d", world.clock.NowUnixNano(), world.expectedEnd)
		}
		if world.runErr == nil {
			for _, participant := range world.actors {
				if checked, ok := participant.(interface{ Fault() error }); ok && checked.Fault() != nil {
					world.runErr = fmt.Errorf("repeatedspot: actor %d failed: %w", participant.ID(), checked.Fault())
					break
				}
			}
		}
		world.closed.Store(true)
	})
	if !called {
		return fmt.Errorf("repeatedspot: a world can run only once")
	}
	return world.runErr
}

// Close releases a built world that was never run. Do not call it concurrently
// with Run; the runner already closes its mount after execution.
func (world *World) Close() {
	if world.closed.CompareAndSwap(false, true) {
		world.mount.Shutdown()
	}
}

func Build(cfg Config) (*World, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	contract := worldContract{SchemaVersion: 2, VenueID: cfg.VenueID, Instrument: cfg.Instrument,
		MatchingRule: "price_time_fifo", ClockMode: "simulated_event_scheduler",
		DeterministicPhases: true, PhaseMaxRounds: phaseMaxRounds, AutomationEnabled: false,
		StartUnixNano: cfg.StartUnixNano, Step: cfg.Step, Iterations: cfg.Iterations,
		SnapshotInterval: cfg.SnapshotInterval, ForbidBorrowing: cfg.ForbidBorrowing,
		RecordSnapshotProjectionEvidence: cfg.RecordSnapshotProjectionEvidence,
		Participants:                     make([]participantContract, 0, len(cfg.Participants))}
	for _, participant := range cfg.Participants {
		balances := make(map[string]int64, len(participant.Balances))
		for asset, balance := range participant.Balances {
			balances[asset] = balance
		}
		contract.Participants = append(contract.Participants, participantContract{
			ActorID: participant.ActorID, ClientID: participant.ClientID, Role: participant.Role,
			Balances: balances, Latency: participant.Latency,
			Fees:   namedDefinition{participant.Fees.name, append(json.RawMessage(nil), participant.Fees.parameters...)},
			Policy: namedDefinition{participant.Policy.name, append(json.RawMessage(nil), participant.Policy.parameters...)},
		})
	}
	contractJSON, err := json.Marshal(contract)
	if err != nil {
		return nil, fmt.Errorf("repeatedspot: encode world contract: %w", err)
	}

	clock := simulation.NewSimulatedClock(cfg.StartUnixNano)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := simulation.NewSimTimerFactory(scheduler)
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		ID: cfg.VenueID, Clock: clock, TickerFactory: timers,
		SnapshotInterval: cfg.SnapshotInterval, EstimatedClients: len(cfg.Participants),
		DeterministicPhases: true, ForbidBorrowing: cfg.ForbidBorrowing,
		RecordSnapshotProjectionEvidence: cfg.RecordSnapshotProjectionEvidence,
	})
	instrument := cfg.Instrument
	ex.AddInstrument(exchange.NewSpotInstrument(instrument.Symbol, instrument.BaseAsset, instrument.QuoteAsset,
		instrument.BasePrecision, instrument.QuotePrecision, instrument.TickSize, instrument.MinOrderSize))
	deployments := make(map[uint64]Deployment, len(cfg.Participants))
	for _, participant := range cfg.Participants {
		deployments[participant.ClientID] = participant.Latency
	}
	mount := simulation.NewMount(ex, simulation.LatencyConfig{
		Scheduler: scheduler, Clock: clock,
		PerClient: func(clientID uint64) (simulation.LatencyProvider, simulation.LatencyProvider, simulation.LatencyProvider) {
			deployment := deployments[clientID]
			return fixedLatency(deployment.RequestLatency), fixedLatency(deployment.ResponseLatency), fixedLatency(deployment.MarketDataLatency)
		},
	})
	runner := simulation.NewRunner(clock, simulation.RunnerConfig{Iterations: cfg.Iterations, Step: cfg.Step,
		DeterministicPhases: true, PhaseMaxRounds: phaseMaxRounds})
	runner.AddMount(mount)
	runner.AddIdler(timers)
	actors := make([]actor.Actor, 0, len(cfg.Participants))
	for index, participant := range cfg.Participants {
		feeModel, err := participant.Fees.build(contract.Participants[index].Fees.Parameters)
		if err != nil || feeModel == nil {
			mount.Shutdown()
			return nil, fmt.Errorf("repeatedspot: build fee for client %d: %v", participant.ClientID, err)
		}
		gateway := mount.ConnectNewClient(participant.ClientID, contract.Participants[index].Balances, feeModel)
		policy, err := participant.Policy.build(participant.ActorID, gateway, timers, contract.Participants[index].Policy.Parameters)
		if err != nil || policy == nil {
			mount.Shutdown()
			return nil, fmt.Errorf("repeatedspot: build actor %d: %v", participant.ActorID, err)
		}
		if policy.ID() != participant.ActorID || policy.Gateway() == nil {
			mount.Shutdown()
			return nil, fmt.Errorf("repeatedspot: actor %d returned mismatched identity or no gateway", participant.ActorID)
		}
		clocked, ok := policy.(interface{ SetTickerFactory(exchange.TickerFactory) })
		if !ok {
			mount.Shutdown()
			return nil, fmt.Errorf("repeatedspot: actor %d cannot use the simulated decision clock", participant.ActorID)
		}
		clocked.SetTickerFactory(timers)
		runner.AddActor(policy)
		actors = append(actors, policy)
	}
	checksum := sha256.Sum256(contractJSON)
	return &World{runner: runner, mount: mount, exchange: ex, actors: actors, contractJSON: contractJSON,
		contractHash: hex.EncodeToString(checksum[:]), clock: clock,
		expectedEnd: cfg.StartUnixNano + int64(cfg.Iterations)*int64(cfg.Step)}, nil
}

func fixedLatency(delay time.Duration) simulation.LatencyProvider {
	if delay == 0 {
		return nil
	}
	return simulation.NewConstantLatency(delay)
}

func validateConfig(cfg Config) error {
	instrument := cfg.Instrument
	if cfg.VenueID == "" || instrument.Symbol == "" || instrument.BaseAsset == "" || instrument.QuoteAsset == "" ||
		instrument.BaseAsset == instrument.QuoteAsset || instrument.BasePrecision <= 0 || instrument.QuotePrecision <= 0 ||
		instrument.TickSize <= 0 || instrument.MinOrderSize <= 0 || cfg.Step <= 0 || cfg.Iterations <= 0 ||
		cfg.SnapshotInterval <= 0 || cfg.StartUnixNano < 0 || int64(cfg.Iterations) > math.MaxInt64/int64(cfg.Step) || len(cfg.Participants) == 0 {
		return fmt.Errorf("repeatedspot: invalid one-book world configuration")
	}
	if cfg.StartUnixNano > math.MaxInt64-int64(cfg.Iterations)*int64(cfg.Step) {
		return fmt.Errorf("repeatedspot: world end time overflows int64")
	}
	actorIDs := make(map[uint64]struct{}, len(cfg.Participants))
	clientIDs := make(map[uint64]struct{}, len(cfg.Participants))
	for _, participant := range cfg.Participants {
		if participant.ActorID == 0 || participant.ClientID == 0 || participant.Role == "" ||
			participant.Policy.name == "" || participant.Policy.build == nil ||
			participant.Fees.name == "" || participant.Fees.build == nil ||
			participant.Latency.RequestLatency < 0 || participant.Latency.ResponseLatency < 0 || participant.Latency.MarketDataLatency < 0 {
			return fmt.Errorf("repeatedspot: invalid participant %d", participant.ActorID)
		}
		if _, exists := actorIDs[participant.ActorID]; exists {
			return fmt.Errorf("repeatedspot: duplicate actor ID %d", participant.ActorID)
		}
		if _, exists := clientIDs[participant.ClientID]; exists {
			return fmt.Errorf("repeatedspot: duplicate client ID %d", participant.ClientID)
		}
		actorIDs[participant.ActorID] = struct{}{}
		clientIDs[participant.ClientID] = struct{}{}
		if len(participant.Balances) == 0 {
			return fmt.Errorf("repeatedspot: client %d has no finite endowment", participant.ClientID)
		}
		for asset, balance := range participant.Balances {
			if (asset != instrument.BaseAsset && asset != instrument.QuoteAsset) || balance < 0 {
				return fmt.Errorf("repeatedspot: client %d has invalid %s balance", participant.ClientID, asset)
			}
		}
	}
	return nil
}
