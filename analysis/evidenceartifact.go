package analysis

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"sync"
)

// EvidenceArtifactHash is a digest of the records physically persisted in a
// run's JSONL evidence files. It is intentionally separate from StreamHash:
// StreamHash hashes normalized observable event fields, whereas this hashes
// each exact JSON record and is therefore directly comparable to the runtime
// evidence-artifact-hash.json attestation.
//
// There is no canonical global file-write order across venue loggers, so this
// is an unordered multiset digest, not an execution-stream digest.
type EvidenceArtifactHash struct {
	Domain   string `json:"domain"`
	Ordering string `json:"ordering"`
	Events   int64  `json:"events"`
	Digest   string `json:"digest"`
}

type artifactSum256 struct {
	limbs  [4]uint64
	events int64
}

func (s *artifactSum256) add(record []byte) {
	hash := sha256.Sum256(record)
	var carry uint64
	for i := 3; i >= 0; i-- {
		limb := binary.BigEndian.Uint64(hash[i*8 : i*8+8])
		sum := s.limbs[i] + limb
		newCarry := uint64(0)
		if sum < s.limbs[i] {
			newCarry = 1
		}
		sum += carry
		if sum < carry {
			newCarry = 1
		}
		s.limbs[i] = sum
		carry = newCarry
	}
	s.events++
}

func (s *artifactSum256) merge(other artifactSum256) {
	var carry uint64
	for i := 3; i >= 0; i-- {
		sum := s.limbs[i] + other.limbs[i]
		newCarry := uint64(0)
		if sum < s.limbs[i] {
			newCarry = 1
		}
		sum += carry
		if sum < carry {
			newCarry = 1
		}
		s.limbs[i] = sum
		carry = newCarry
	}
	s.events += other.events
}

func (s artifactSum256) hex() string {
	var raw [32]byte
	for i := range s.limbs {
		binary.BigEndian.PutUint64(raw[i*8:i*8+8], s.limbs[i])
	}
	return hex.EncodeToString(raw[:])
}

// MeasureEvidenceArtifactHash independently scans the JSONL files rather
// than using decoded events. A malformed or overlong evidence record is an
// error: silently skipping it would turn evidence loss into a matching hash.
func (r *Run) MeasureEvidenceArtifactHash() (*EvidenceArtifactHash, error) {
	workers := runtime.GOMAXPROCS(0)
	// See Run.Scan: failures must not deadlock the dispatcher after an evidence
	// corruption is detected.
	jobs := make(chan string, len(r.files))
	for _, path := range r.files {
		jobs <- path
	}
	close(jobs)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var total artifactSum256
	var once sync.Once
	var failure error
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				part, err := hashEvidenceFile(path)
				if err != nil {
					once.Do(func() { failure = err })
					return
				}
				mu.Lock()
				total.merge(part)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if failure != nil {
		return nil, failure
	}
	return &EvidenceArtifactHash{
		Domain:   "persisted_json_records",
		Ordering: "unordered_multiset",
		Events:   total.events,
		Digest:   total.hex(),
	}, nil
}

func hashEvidenceFile(path string) (artifactSum256, error) {
	file, err := os.Open(path)
	if err != nil {
		return artifactSum256{}, err
	}
	defer file.Close()
	var result artifactSum256
	scanner := bufio.NewScanner(bufio.NewReaderSize(file, 1<<20))
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			return artifactSum256{}, fmt.Errorf("analysis: empty evidence record in %s", path)
		}
		result.add(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		return artifactSum256{}, fmt.Errorf("analysis: scan evidence %s: %w", path, err)
	}
	return result, nil
}
