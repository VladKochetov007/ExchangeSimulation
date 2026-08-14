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

func TestBaseMarketReconnectClosesPriorGateway(t *testing.T) {
	market := NewBaseMarket()
	old := market.ConnectNewClient(1, nil, nil)
	current := market.ConnectNewClient(1, nil, nil)
	if old.IsRunning() {
		t.Fatal("prior session remains live after reconnect")
	}
	market.Shutdown()
	if current.IsRunning() {
		t.Fatal("current session remains live after shutdown")
	}
}
