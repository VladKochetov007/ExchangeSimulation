package exchange

import (
	"testing"
)

// TestBalanceSnapshotEmpty verifies snapshot for client with no balances
func TestBalanceSnapshotEmpty(t *testing.T) {
	client := NewClient(1, &FixedFee{})
	timestamp := int64(1000000000)

	snapshot := client.GetBalanceSnapshot(timestamp)

	if snapshot.Timestamp != timestamp {
		t.Errorf("Expected timestamp %d, got %d", timestamp, snapshot.Timestamp)
	}

	if snapshot.ClientID != 1 {
		t.Errorf("Expected client ID 1, got %d", snapshot.ClientID)
	}

	if len(snapshot.SpotBalances) != 0 {
		t.Errorf("Expected 0 spot balances, got %d", len(snapshot.SpotBalances))
	}

	if len(snapshot.PerpBalances) != 0 {
		t.Errorf("Expected 0 perp balances, got %d", len(snapshot.PerpBalances))
	}

	if len(snapshot.Borrowed) != 0 {
		t.Errorf("Expected 0 borrowed, got %d", len(snapshot.Borrowed))
	}
}

// TestBalanceSnapshotSpotOnly verifies snapshot with only spot balances
func TestBalanceSnapshotSpotOnly(t *testing.T) {
	client := NewClient(1, &FixedFee{})
	client.Balances["BTC"] = 5 * SATOSHI
	client.Balances["USD"] = 10000 * USD_PRECISION
	client.Reserved["USD"] = 1000 * USD_PRECISION

	snapshot := client.GetBalanceSnapshot(int64(2000000000))

	if len(snapshot.SpotBalances) != 2 {
		t.Fatalf("Expected 2 spot balances, got %d", len(snapshot.SpotBalances))
	}

	// Find BTC balance
	var btcBalance *AssetBalance
	for i := range snapshot.SpotBalances {
		if snapshot.SpotBalances[i].Asset == "BTC" {
			btcBalance = &snapshot.SpotBalances[i]
			break
		}
	}

	if btcBalance == nil {
		t.Fatal("BTC balance not found")
	}

	if btcBalance.Total != 5*SATOSHI {
		t.Errorf("Expected BTC total %d, got %d", 5*SATOSHI, btcBalance.Total)
	}

	if btcBalance.Reserved != 0 {
		t.Errorf("Expected BTC reserved 0, got %d", btcBalance.Reserved)
	}

	if btcBalance.Available != 5*SATOSHI {
		t.Errorf("Expected BTC available %d, got %d", 5*SATOSHI, btcBalance.Available)
	}

	// Find USD balance
	var usdBalance *AssetBalance
	for i := range snapshot.SpotBalances {
		if snapshot.SpotBalances[i].Asset == "USD" {
			usdBalance = &snapshot.SpotBalances[i]
			break
		}
	}

	if usdBalance == nil {
		t.Fatal("USD balance not found")
	}

	if usdBalance.Total != 10000*USD_PRECISION {
		t.Errorf("Expected USD total %d, got %d", 10000*USD_PRECISION, usdBalance.Total)
	}

	if usdBalance.Reserved != 1000*USD_PRECISION {
		t.Errorf("Expected USD reserved %d, got %d", 1000*USD_PRECISION, usdBalance.Reserved)
	}

	expectedAvailable := int64(9000 * USD_PRECISION)
	if usdBalance.Available != expectedAvailable {
		t.Errorf("Expected USD available %d, got %d", expectedAvailable, usdBalance.Available)
	}

	// Verify equation: Available = Total - Reserved
	if usdBalance.Available != usdBalance.Total-usdBalance.Reserved {
		t.Errorf("Balance equation violated: %d != %d - %d",
			usdBalance.Available, usdBalance.Total, usdBalance.Reserved)
	}
}

// TestBalanceSnapshotPerpOnly verifies snapshot with only perpetual balances
func TestBalanceSnapshotPerpOnly(t *testing.T) {
	client := NewClient(1, &FixedFee{})
	client.PerpBalances["USD"] = 50000 * USD_PRECISION
	client.PerpReserved["USD"] = 10000 * USD_PRECISION

	snapshot := client.GetBalanceSnapshot(int64(3000000000))

	if len(snapshot.SpotBalances) != 0 {
		t.Errorf("Expected 0 spot balances, got %d", len(snapshot.SpotBalances))
	}

	if len(snapshot.PerpBalances) != 1 {
		t.Fatalf("Expected 1 perp balance, got %d", len(snapshot.PerpBalances))
	}

	perpUSD := snapshot.PerpBalances[0]
	if perpUSD.Asset != "USD" {
		t.Errorf("Expected USD, got %s", perpUSD.Asset)
	}

	if perpUSD.Total != 50000*USD_PRECISION {
		t.Errorf("Expected total %d, got %d", 50000*USD_PRECISION, perpUSD.Total)
	}

	if perpUSD.Reserved != 10000*USD_PRECISION {
		t.Errorf("Expected reserved %d, got %d", 10000*USD_PRECISION, perpUSD.Reserved)
	}

	expectedAvailable := int64(40000 * USD_PRECISION)
	if perpUSD.Available != expectedAvailable {
		t.Errorf("Expected available %d, got %d", expectedAvailable, perpUSD.Available)
	}
}

// TestBalanceSnapshotMixed verifies snapshot with all wallet types
func TestBalanceSnapshotMixed(t *testing.T) {
	client := NewClient(1, &FixedFee{})

	// Spot wallet
	client.Balances["BTC"] = 10 * SATOSHI
	client.Balances["ETH"] = 100 * SATOSHI
	client.Reserved["BTC"] = 2 * SATOSHI

	// Perp wallet
	client.PerpBalances["USD"] = 100000 * USD_PRECISION
	client.PerpBalances["USDT"] = 50000 * USD_PRECISION
	client.PerpReserved["USD"] = 20000 * USD_PRECISION

	// Borrowed
	client.Borrowed["USD"] = 5000 * USD_PRECISION
	client.Borrowed["BTC"] = 1 * SATOSHI

	snapshot := client.GetBalanceSnapshot(int64(4000000000))

	// Check spot balances
	if len(snapshot.SpotBalances) != 2 {
		t.Errorf("Expected 2 spot balances, got %d", len(snapshot.SpotBalances))
	}

	// Check perp balances
	if len(snapshot.PerpBalances) != 2 {
		t.Errorf("Expected 2 perp balances, got %d", len(snapshot.PerpBalances))
	}

	// Check borrowed
	if len(snapshot.Borrowed) != 2 {
		t.Errorf("Expected 2 borrowed entries, got %d", len(snapshot.Borrowed))
	}

	if snapshot.Borrowed["USD"] != 5000*USD_PRECISION {
		t.Errorf("Expected borrowed USD %d, got %d", 5000*USD_PRECISION, snapshot.Borrowed["USD"])
	}

	if snapshot.Borrowed["BTC"] != 1*SATOSHI {
		t.Errorf("Expected borrowed BTC %d, got %d", 1*SATOSHI, snapshot.Borrowed["BTC"])
	}
}

// TestBalanceSnapshotAvailableCalculation verifies Available = Total - Reserved
func TestBalanceSnapshotAvailableCalculation(t *testing.T) {
	client := NewClient(1, &FixedFee{})

	testCases := []struct {
		asset    string
		total    int64
		reserved int64
	}{
		{"BTC", 10 * SATOSHI, 3 * SATOSHI},
		{"USD", 100000 * USD_PRECISION, 25000 * USD_PRECISION},
		{"ETH", 50 * SATOSHI, 0},
		{"USDT", 75000 * USD_PRECISION, 75000 * USD_PRECISION}, // All reserved
	}

	for _, tc := range testCases {
		client.Balances[tc.asset] = tc.total
		client.Reserved[tc.asset] = tc.reserved
	}

	snapshot := client.GetBalanceSnapshot(int64(5000000000))

	for _, tc := range testCases {
		var found *AssetBalance
		for i := range snapshot.SpotBalances {
			if snapshot.SpotBalances[i].Asset == tc.asset {
				found = &snapshot.SpotBalances[i]
				break
			}
		}

		if found == nil {
			t.Errorf("Asset %s not found in snapshot", tc.asset)
			continue
		}

		expectedAvailable := tc.total - tc.reserved
		if found.Available != expectedAvailable {
			t.Errorf("%s: expected available %d, got %d (total=%d, reserved=%d)",
				tc.asset, expectedAvailable, found.Available, tc.total, tc.reserved)
		}

		// Verify equation
		if found.Available != found.Total-found.Reserved {
			t.Errorf("%s: balance equation violated", tc.asset)
		}
	}
}

// TestBalanceSnapshotBorrowedFiltering verifies only > 0 borrowed amounts included
func TestBalanceSnapshotBorrowedFiltering(t *testing.T) {
	client := NewClient(1, &FixedFee{})

	client.Borrowed["USD"] = 1000 * USD_PRECISION  // Should be included
	client.Borrowed["BTC"] = 0                     // Should be filtered out
	client.Borrowed["ETH"] = -100 * SATOSHI        // Should be filtered out (negative)

	snapshot := client.GetBalanceSnapshot(int64(6000000000))

	if len(snapshot.Borrowed) != 1 {
		t.Errorf("Expected 1 borrowed entry (filtered > 0), got %d", len(snapshot.Borrowed))
	}

	if snapshot.Borrowed["USD"] != 1000*USD_PRECISION {
		t.Errorf("Expected borrowed USD %d, got %d", 1000*USD_PRECISION, snapshot.Borrowed["USD"])
	}

	if _, exists := snapshot.Borrowed["BTC"]; exists {
		t.Error("Zero borrowed BTC should be filtered out")
	}

	if _, exists := snapshot.Borrowed["ETH"]; exists {
		t.Error("Negative borrowed ETH should be filtered out")
	}
}

// TestBalanceSnapshotTimestamp verifies timestamp is set correctly
func TestBalanceSnapshotTimestamp(t *testing.T) {
	client := NewClient(1, &FixedFee{})
	client.Balances["USD"] = 1000 * USD_PRECISION

	timestamps := []int64{
		1000000000,
		1707925123456789000,
		1707925124000000000,
	}

	for _, ts := range timestamps {
		snapshot := client.GetBalanceSnapshot(ts)
		if snapshot.Timestamp != ts {
			t.Errorf("Expected timestamp %d, got %d", ts, snapshot.Timestamp)
		}
	}
}

// TestBalanceSnapshotClientID verifies client ID is set correctly
func TestBalanceSnapshotClientID(t *testing.T) {
	clientIDs := []uint64{1, 42, 999, 18446744073709551615} // max uint64

	for _, id := range clientIDs {
		client := NewClient(id, &FixedFee{})
		client.Balances["USD"] = 1000 * USD_PRECISION

		snapshot := client.GetBalanceSnapshot(int64(7000000000))
		if snapshot.ClientID != id {
			t.Errorf("Expected client ID %d, got %d", id, snapshot.ClientID)
		}
	}
}

// TestBalanceSnapshotImmutability verifies snapshot doesn't share state with client
func TestBalanceSnapshotImmutability(t *testing.T) {
	client := NewClient(1, &FixedFee{})
	client.Balances["BTC"] = 10 * SATOSHI
	client.Borrowed["USD"] = 1000 * USD_PRECISION

	snapshot := client.GetBalanceSnapshot(int64(8000000000))

	// Modify client balances after snapshot
	client.Balances["BTC"] = 20 * SATOSHI
	client.Borrowed["USD"] = 2000 * USD_PRECISION
	client.Borrowed["ETH"] = 100 * SATOSHI

	// Snapshot should be unchanged
	var btcBal *AssetBalance
	for i := range snapshot.SpotBalances {
		if snapshot.SpotBalances[i].Asset == "BTC" {
			btcBal = &snapshot.SpotBalances[i]
			break
		}
	}

	if btcBal == nil {
		t.Fatal("BTC balance not found")
	}

	if btcBal.Total != 10*SATOSHI {
		t.Errorf("Snapshot BTC modified: expected %d, got %d", 10*SATOSHI, btcBal.Total)
	}

	if snapshot.Borrowed["USD"] != 1000*USD_PRECISION {
		t.Errorf("Snapshot borrowed USD modified: expected %d, got %d",
			1000*USD_PRECISION, snapshot.Borrowed["USD"])
	}

	if _, exists := snapshot.Borrowed["ETH"]; exists {
		t.Error("Snapshot borrowed map shares state with client (ETH shouldn't exist)")
	}
}

// TestBalanceSnapshotZeroReserved verifies zero reserved amounts are included
func TestBalanceSnapshotZeroReserved(t *testing.T) {
	client := NewClient(1, &FixedFee{})
	client.Balances["BTC"] = 5 * SATOSHI
	client.Reserved["BTC"] = 0 // Explicitly zero

	snapshot := client.GetBalanceSnapshot(int64(9000000000))

	if len(snapshot.SpotBalances) != 1 {
		t.Fatalf("Expected 1 spot balance, got %d", len(snapshot.SpotBalances))
	}

	btc := snapshot.SpotBalances[0]
	if btc.Reserved != 0 {
		t.Errorf("Expected reserved 0, got %d", btc.Reserved)
	}

	if btc.Available != btc.Total {
		t.Errorf("With zero reserved, available should equal total: %d != %d",
			btc.Available, btc.Total)
	}
}

// TestBalanceSnapshotLargePrecision verifies handling of large precision values
func TestBalanceSnapshotLargePrecision(t *testing.T) {
	client := NewClient(1, &FixedFee{})

	// Large BTC amount (21 million BTC in satoshis)
	maxBTC := int64(21_000_000) * SATOSHI
	client.Balances["BTC"] = maxBTC

	// Large USD amount (1 trillion USD in micro-USD)
	largeUSD := int64(1_000_000_000_000) * USD_PRECISION
	client.PerpBalances["USD"] = largeUSD

	snapshot := client.GetBalanceSnapshot(int64(10000000000))

	var btcBal *AssetBalance
	for i := range snapshot.SpotBalances {
		if snapshot.SpotBalances[i].Asset == "BTC" {
			btcBal = &snapshot.SpotBalances[i]
			break
		}
	}

	if btcBal == nil {
		t.Fatal("BTC balance not found")
	}

	if btcBal.Total != maxBTC {
		t.Errorf("Large BTC value incorrect: expected %d, got %d", maxBTC, btcBal.Total)
	}

	var usdBal *AssetBalance
	for i := range snapshot.PerpBalances {
		if snapshot.PerpBalances[i].Asset == "USD" {
			usdBal = &snapshot.PerpBalances[i]
			break
		}
	}

	if usdBal == nil {
		t.Fatal("USD balance not found")
	}

	if usdBal.Total != largeUSD {
		t.Errorf("Large USD value incorrect: expected %d, got %d", largeUSD, usdBal.Total)
	}
}

// TestBalanceSnapshotMultipleAssets verifies handling of many assets
func TestBalanceSnapshotMultipleAssets(t *testing.T) {
	client := NewClient(1, &FixedFee{})

	assets := []string{"BTC", "ETH", "SOL", "AVAX", "MATIC", "DOT", "ATOM", "NEAR"}
	for i, asset := range assets {
		client.Balances[asset] = int64(i+1) * SATOSHI
		if i%2 == 0 {
			client.Reserved[asset] = int64(i+1) * SATOSHI / 2
		}
	}

	snapshot := client.GetBalanceSnapshot(int64(11000000000))

	if len(snapshot.SpotBalances) != len(assets) {
		t.Errorf("Expected %d spot balances, got %d", len(assets), len(snapshot.SpotBalances))
	}

	// Verify all assets present
	assetMap := make(map[string]bool)
	for _, bal := range snapshot.SpotBalances {
		assetMap[bal.Asset] = true

		// Verify balance equation
		if bal.Available != bal.Total-bal.Reserved {
			t.Errorf("%s: balance equation violated", bal.Asset)
		}
	}

	for _, asset := range assets {
		if !assetMap[asset] {
			t.Errorf("Asset %s missing from snapshot", asset)
		}
	}
}
