package feesim

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"testing"
)

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
