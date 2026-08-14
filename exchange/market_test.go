package exchange

import "testing"

func TestBaseMarketShutdownClosesExistingAndNewGateways(t *testing.T) {
	market := NewBaseMarket()
	existing := market.ConnectNewClient(1, nil, nil)
	market.Shutdown()
	market.Shutdown()

	if existing.IsRunning() {
		t.Fatal("existing gateway remains live after market shutdown")
	}
	if market.IsRunning() {
		t.Fatal("market remains running after shutdown")
	}
	if gateway := market.ConnectNewClient(2, nil, nil); gateway.IsRunning() {
		t.Fatal("shutdown market accepted a new live gateway")
	}
}
