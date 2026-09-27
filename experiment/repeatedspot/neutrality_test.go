package repeatedspot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"exchange_sim/simulation"
	worldspot "exchange_sim/simulations/repeatedspot"
)

type fixtureState struct {
	balances map[uint64]map[string]int64
	bids     []int64
	asks     []int64
}

func TestFreshProcessEvidenceIdentity(t *testing.T) {
	if destination := os.Getenv("REPEATED_SPOT_CHILD_IDENTITY"); destination != "" {
		writeFixtureIdentity(t, destination, fixtureWorld(t))
		return
	}
	compareFreshProcessIdentity(t, "REPEATED_SPOT_CHILD_IDENTITY", "TestFreshProcessEvidenceIdentity")
}

func TestFreshProcessLocalReferenceIdentity(t *testing.T) {
	if destination := os.Getenv("REPEATED_SPOT_CHILD_REFERENCE_IDENTITY"); destination != "" {
		writeFixtureIdentity(t, destination, localReferenceFixture(t))
		return
	}
	compareFreshProcessIdentity(t, "REPEATED_SPOT_CHILD_REFERENCE_IDENTITY", "TestFreshProcessLocalReferenceIdentity")
}

func localReferenceFixture(t *testing.T) *worldspot.World {
	t.Helper()
	return fixtureWorldWithReferencePolicy(t, true, false, 1, 2*time.Second,
		true, 2, 5*time.Second, true, 15*time.Second)
}

func writeFixtureIdentity(t *testing.T, destination string, world *worldspot.World) {
	t.Helper()
	state, identity, raw, directory := capturedStateWorld(t, world)
	streamHash := sha256.Sum256(raw)
	artifact := struct {
		State      fixtureState      `json:"state"`
		Identity   EvidenceIdentity  `json:"identity"`
		StreamHash string            `json:"stream_sha256"`
		Sidecars   map[string]string `json:"sidecars"`
	}{State: state, Identity: identity, StreamHash: hex.EncodeToString(streamHash[:]), Sidecars: make(map[string]string)}
	for _, name := range []string{"market-data-evidence-v2.json", "market-data-schedules-v2.bin", "market-data-receipts-v2.bin", "market-data-decisions-v2.bin"} {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(content)
		artifact.Sidecars[name] = hex.EncodeToString(digest[:])
	}
	encoded, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, encoded, 0600); err != nil {
		t.Fatal(err)
	}
}

func compareFreshProcessIdentity(t *testing.T, environment, testName string) {
	t.Helper()
	var outputs [2][]byte
	for index := range outputs {
		path := filepath.Join(t.TempDir(), "identity.json")
		command := exec.Command(os.Args[0], "-test.run=^"+testName+"$")
		command.Env = append(os.Environ(), environment+"="+path)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("fresh-process control %d failed: %v: %s", index, err, output)
		}
		var err error
		outputs[index], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(outputs[0], outputs[1]) {
		t.Fatalf("fresh-process evidence identities differ: %s != %s", outputs[0], outputs[1])
	}
}

func observeFixtureState(world *worldspot.World) fixtureState {
	state := fixtureState{balances: make(map[uint64]map[string]int64)}
	for clientID, client := range world.Venue().Clients {
		state.balances[clientID] = copyBalances(client.Balances)
	}
	book := world.Venue().GetBook("ABC/USD")
	for _, level := range book.Bids.GetPublicSnapshot() {
		state.bids = append(state.bids, level.Price, level.VisibleQty)
	}
	for _, level := range book.Asks.GetPublicSnapshot() {
		state.asks = append(state.asks, level.Price, level.VisibleQty)
	}
	return state
}

func capturedState(t *testing.T) (fixtureState, EvidenceIdentity, []byte, string) {
	t.Helper()
	return capturedStateWorld(t, fixtureWorld(t))
}

func capturedStateWorld(t *testing.T, world *worldspot.World) (fixtureState, EvidenceIdentity, []byte, string) {
	t.Helper()
	directory := t.TempDir()
	receipts, err := simulation.NewMarketDataReceiptRecorder(directory)
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	capture, err := NewCapture(world, &raw, receipts)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := capture.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return observeFixtureState(world), identity, bytes.Clone(raw.Bytes()), directory
}

func TestLocalReferenceEconomicStateUnaffectedByCapture(t *testing.T) {
	plain := localReferenceFixture(t)
	if err := plain.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	expected := observeFixtureState(plain)
	captured, _, _, _ := capturedStateWorld(t, localReferenceFixture(t))
	if !reflect.DeepEqual(expected, captured) {
		t.Fatalf("local-reference capture changed economic state: plain=%+v captured=%+v", expected, captured)
	}
}

func TestEconomicStateUnaffectedByEvidenceAndRepeatedBuilds(t *testing.T) {
	plain := fixtureWorld(t)
	if err := plain.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	expected := observeFixtureState(plain)
	first, firstID, firstRaw, firstDirectory := capturedState(t)
	second, secondID, secondRaw, secondDirectory := capturedState(t)
	if !reflect.DeepEqual(expected, first) || !reflect.DeepEqual(first, second) {
		t.Fatalf("capture changed economic state: plain=%+v first=%+v second=%+v", expected, first, second)
	}
	if firstID != secondID || !bytes.Equal(firstRaw, secondRaw) {
		t.Fatal("two fresh captures of the same synthetic world differ")
	}
	for _, name := range []string{"market-data-evidence-v2.json", "market-data-schedules-v2.bin", "market-data-receipts-v2.bin", "market-data-decisions-v2.bin"} {
		left, err := os.ReadFile(filepath.Join(firstDirectory, name))
		if err != nil {
			t.Fatal(err)
		}
		right, err := os.ReadFile(filepath.Join(secondDirectory, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(left, right) {
			t.Fatalf("fresh capture sidecar %s differs", name)
		}
	}
}
