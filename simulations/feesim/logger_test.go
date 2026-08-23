package feesim

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestPersistedEventPreservesLegacyCanonicalBytes(t *testing.T) {
	tests := []struct {
		name   string
		time   int64
		client uint64
		event  string
		data   any
		want   string
	}{
		{
			name: "map payload", time: 1, client: 7, event: "one", data: map[string]int{"n": 1},
			want: `{"client_id":7,"data":{"n":1},"event":"one","sim_ts":1}`,
		},
		{
			name: "struct payload", time: -2, client: 9, event: "fill", data: struct {
				Price int64 `json:"price"`
				Qty   int64 `json:"qty"`
			}{Price: 50_000, Qty: 3},
			want: `{"client_id":9,"data":{"price":50000,"qty":3},"event":"fill","sim_ts":-2}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := json.Marshal(persistedEvent{ClientID: test.client, Data: test.data, Event: test.event, SimTS: test.time})
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := json.Marshal(map[string]any{"sim_ts": test.time, "client_id": test.client, "event": test.event, "data": test.data})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, legacy) || string(got) != test.want {
				t.Fatalf("persisted bytes changed: got %s legacy %s want %s", got, legacy, test.want)
			}
		})
	}
}

func BenchmarkPersistedEventEnvelope(b *testing.B) {
	payload := map[string]any{"price": int64(50_000), "qty": int64(3), "side": "BUY"}
	b.Run("legacy_map", func(b *testing.B) {
		for range b.N {
			if _, err := json.Marshal(map[string]any{"sim_ts": int64(1), "client_id": uint64(7), "event": "fill", "data": payload}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("struct_envelope", func(b *testing.B) {
		for range b.N {
			if _, err := json.Marshal(persistedEvent{ClientID: 7, Data: payload, Event: "fill", SimTS: 1}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestJSONLinesLoggerEvidenceDigestCountsPersistedRecordsOnce(t *testing.T) {
	path := t.TempDir() + "/evidence.jsonl"
	logger, err := NewJSONLinesLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	logger.LogEvent(1, 7, "one", map[string]int{"n": 1})
	logger.LogEvent(2, 8, "two", map[string]int{"n": 2})
	logger.Close()

	got := logger.EvidenceDigest()
	if got.Events != 2 {
		t.Fatalf("runtime record count = %d, want 2", got.Events)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var limbs [4]uint64
	scanner := bufio.NewScanner(file)
	count := int64(0)
	for scanner.Scan() {
		hash := sha256.Sum256(scanner.Bytes())
		var carry uint64
		for i := 3; i >= 0; i-- {
			limb := binary.BigEndian.Uint64(hash[i*8 : i*8+8])
			sum := limbs[i] + limb
			newCarry := uint64(0)
			if sum < limbs[i] {
				newCarry = 1
			}
			sum += carry
			if sum < carry {
				newCarry = 1
			}
			limbs[i] = sum
			carry = newCarry
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	var raw [32]byte
	for i := range limbs {
		binary.BigEndian.PutUint64(raw[i*8:i*8+8], limbs[i])
	}
	want := hex.EncodeToString(raw[:])
	if count != got.Events || want != got.Hex() {
		t.Fatalf("offline evidence %d/%s, runtime %d/%s", count, want, got.Events, got.Hex())
	}
}
