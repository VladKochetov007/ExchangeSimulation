package simulation

import (
	"exchange_sim/exchange"
	"fmt"
	"sync"
	"sync/atomic"
)

type VenueID string

type VenueRegistry struct {
	venues map[VenueID]*exchange.Exchange
	mu     sync.RWMutex
}

func NewVenueRegistry() *VenueRegistry {
	return &VenueRegistry{
		venues: make(map[VenueID]*exchange.Exchange),
	}
}

func (r *VenueRegistry) Register(id VenueID, ex *exchange.Exchange) {
	r.mu.Lock()
	r.venues[id] = ex
	r.mu.Unlock()
}

func (r *VenueRegistry) Get(id VenueID) *exchange.Exchange {
	r.mu.RLock()
	ex := r.venues[id]
	r.mu.RUnlock()
	return ex
}

func (r *VenueRegistry) ListVenues() []VenueID {
	r.mu.RLock()
	ids := make([]VenueID, 0, len(r.venues))
	for id := range r.venues {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	return ids
}

type VenueResponse struct {
	Venue    VenueID
	Response exchange.Response
}

type VenueMarketData struct {
	Venue VenueID
	Data  *exchange.MarketDataMsg
}

type MultiVenueGateway struct {
	clientID     uint64
	gateways     map[VenueID]*exchange.ClientGateway
	registry     *VenueRegistry
	responseCh   chan VenueResponse
	marketDataCh chan VenueMarketData
	running      atomic.Bool
	stopCh       chan struct{}
	mu           sync.RWMutex
}

func NewMultiVenueGateway(
	clientID uint64,
	registry *VenueRegistry,
	initialBalances map[VenueID]map[string]int64,
	feePlans map[VenueID]exchange.FeeModel,
) *MultiVenueGateway {
	gateways := make(map[VenueID]*exchange.ClientGateway)

	for venue, ex := range registry.venues {
		balances := initialBalances[venue]
		if balances == nil {
			balances = make(map[string]int64)
		}
		feePlan := feePlans[venue]
		if feePlan == nil {
			feePlan = &exchange.FixedFee{}
		}
		gateways[venue] = ex.ConnectClient(clientID, balances, feePlan)
	}

	mgw := &MultiVenueGateway{
		clientID:     clientID,
		gateways:     gateways,
		registry:     registry,
		responseCh:   make(chan VenueResponse, 100),
		marketDataCh: make(chan VenueMarketData, 1000),
		stopCh:       make(chan struct{}),
	}

	return mgw
}

func (m *MultiVenueGateway) Start() {
	if !m.running.CompareAndSwap(false, true) {
		return
	}

	for venue, gw := range m.gateways {
		go m.forwardResponses(venue, gw)
		go m.forwardMarketData(venue, gw)
	}
}

func (m *MultiVenueGateway) Stop() {
	if !m.running.CompareAndSwap(true, false) {
		return
	}
	close(m.stopCh)
}

func (m *MultiVenueGateway) SubmitOrder(venue VenueID, req *exchange.OrderRequest) {
	m.mu.RLock()
	gw := m.gateways[venue]
	m.mu.RUnlock()

	if gw == nil {
		return
	}

	gw.RequestCh <- exchange.Request{
		Type:     exchange.ReqPlaceOrder,
		OrderReq: req,
	}
}

func (m *MultiVenueGateway) CancelOrder(venue VenueID, req *exchange.CancelRequest) {
	m.mu.RLock()
	gw := m.gateways[venue]
	m.mu.RUnlock()

	if gw == nil {
		return
	}

	gw.RequestCh <- exchange.Request{
		Type:      exchange.ReqCancelOrder,
		CancelReq: req,
	}
}

func (m *MultiVenueGateway) QueryBalance(venue VenueID, req *exchange.QueryRequest) {
	m.mu.RLock()
	gw := m.gateways[venue]
	m.mu.RUnlock()

	if gw == nil {
		return
	}

	gw.RequestCh <- exchange.Request{
		Type:     exchange.ReqQueryBalance,
		QueryReq: req,
	}
}

func (m *MultiVenueGateway) Subscribe(venue VenueID, symbol string) {
	m.mu.RLock()
	gw := m.gateways[venue]
	m.mu.RUnlock()

	if gw == nil {
		return
	}

	gw.RequestCh <- exchange.Request{
		Type: exchange.ReqSubscribe,
		QueryReq: &exchange.QueryRequest{
			RequestID: 0,
			Symbol:    symbol,
		},
	}
}

func (m *MultiVenueGateway) Unsubscribe(venue VenueID, symbol string) {
	m.mu.RLock()
	gw := m.gateways[venue]
	m.mu.RUnlock()

	if gw == nil {
		return
	}

	gw.RequestCh <- exchange.Request{
		Type: exchange.ReqUnsubscribe,
		QueryReq: &exchange.QueryRequest{
			RequestID: 0,
			Symbol:    symbol,
		},
	}
}

func (m *MultiVenueGateway) GetGateway(venue VenueID) *exchange.ClientGateway {
	m.mu.RLock()
	gw := m.gateways[venue]
	m.mu.RUnlock()
	return gw
}

func (m *MultiVenueGateway) ResponseCh() <-chan VenueResponse {
	return m.responseCh
}

func (m *MultiVenueGateway) MarketDataCh() <-chan VenueMarketData {
	return m.marketDataCh
}

func (m *MultiVenueGateway) forwardResponses(venue VenueID, gw *exchange.ClientGateway) {
	for {
		select {
		case <-m.stopCh:
			return
		case resp, ok := <-gw.ResponseCh:
			if !ok {
				return
			}
			select {
			case m.responseCh <- VenueResponse{Venue: venue, Response: resp}:
			default:
			}
		}
	}
}

func (m *MultiVenueGateway) forwardMarketData(venue VenueID, gw *exchange.ClientGateway) {
	for {
		select {
		case <-m.stopCh:
			return
		case msg, ok := <-gw.MarketData:
			if !ok {
				return
			}
			select {
			case m.marketDataCh <- VenueMarketData{Venue: venue, Data: msg}:
			default:
			}
		}
	}
}

func (m *MultiVenueGateway) String() string {
	return fmt.Sprintf("MultiVenueGateway(clientID=%d, venues=%d)", m.clientID, len(m.gateways))
}
