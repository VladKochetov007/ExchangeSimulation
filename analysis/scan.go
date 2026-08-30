package analysis

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
)

// Event is one decoded log record.
//
// The multivenue logger nests differently for spot and derivative books: a spot
// record carries its fields directly under data.payload, while a derivative
// record wraps them one level deeper under data.payload.payload with the symbol
// beside them. Reading only the outer level silently yields zero values for
// every derivative field, which is a mistake this type exists to prevent.
type Event struct {
	SimTS    int64
	ClientID uint64
	Name     string
	VenueID  string
	Symbol   string
	File     string
	// Ordinal is the one-based physical record position in File. It permits
	// analyzers to distinguish causal order among same-timestamp records in one
	// persisted log; SimTS alone is not sufficient at lifecycle boundaries.
	Ordinal int64

	payload json.RawMessage
}

// Decode unmarshals the event's innermost payload.
func (e *Event) Decode(target any) error { return json.Unmarshal(e.payload, target) }

// Raw returns the innermost payload without decoding it.
func (e *Event) Raw() json.RawMessage { return e.payload }

// decodeRequiredJSON decodes a payload and verifies that the fields which carry
// its identity and outcome are present. json.Unmarshal intentionally leaves a
// missing numeric field at zero; that is useful for optional fields but unsafe
// for evidence audits, where a dropped field can otherwise become a plausible
// zero-valued record.
func decodeRequiredJSON(raw json.RawMessage, target any, required ...string) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	// Fast path: derive presence directly from the already validated bytes.
	// The fallback below stays the semantic reference and still runs whenever
	// the scanner declines to decide.
	if missing, decided := scanRequiredFields(raw, required); decided {
		if missing != "" {
			return fmt.Errorf("missing required payload field %q", missing)
		}
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("payload is not a JSON object")
	}
	for _, name := range required {
		value, present := fields[name]
		if !present || string(bytes.TrimSpace(value)) == "null" {
			return fmt.Errorf("missing required payload field %q", name)
		}
	}
	return nil
}

type envelope struct {
	SimTS    int64           `json:"sim_ts"`
	ClientID uint64          `json:"client_id"`
	Event    string          `json:"event"`
	Data     json.RawMessage `json:"data"`
}

type dataLayer struct {
	VenueID string          `json:"venue_id"`
	Symbol  string          `json:"symbol"`
	Payload json.RawMessage `json:"payload"`
}

// ScanOptions narrows a scan before any decoding happens, which is what keeps a
// multi-million-event pass cheap.
type ScanOptions struct {
	// Events, when non-empty, keeps only these event names.
	Events []string
	// Files, when non-nil, replaces the run's full file list.
	Files []string
	// FilesSelected marks that the caller performed a selection, so an empty
	// Files means "no matches" rather than "not specified".
	FilesSelected bool
	// Workers overrides the degree of parallelism. Zero uses GOMAXPROCS.
	Workers int
}

// Scan walks the run's events and calls visit for each one that passes opts.
//
// visit is called concurrently from several goroutines, one per file, so it
// must be safe for concurrent use. Callers that need ordering should collect
// per-file and merge, or restrict Files to one file.
func (r *Run) Scan(opts ScanOptions, visit func(Event)) error {
	if r.fuse != nil {
		return r.fuse.scanFused(r, opts, visit)
	}
	// A caller that selected files and matched none must scan nothing. Treating
	// an empty selection as "everything" turns a mistyped venue or symbol into
	// a blend of every book in the run, which reports a full-looking tape and
	// passes any emptiness check downstream.
	files := opts.Files
	if files == nil && !opts.FilesSelected {
		files = r.files
	}
	keep := make(map[string]bool, len(opts.Events))
	for _, name := range opts.Events {
		keep[name] = true
	}
	// Matching the quoted event name against the raw line before decoding skips
	// the JSON parse for the overwhelming majority of records.
	needles := make([][]byte, 0, len(opts.Events))
	for _, name := range opts.Events {
		needles = append(needles, []byte(`"`+name+`"`))
	}

	if scanStatsEnabled {
		scanCalls.Add(1)
		scanFiles.Add(int64(len(files)))
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	// Queue paths before workers start. A worker intentionally stops at the
	// first malformed evidence record; a buffered, closed queue prevents that
	// failure from stranding the producer on an unbuffered send.
	jobs := make(chan string, len(files))
	for _, path := range files {
		jobs <- path
	}
	close(jobs)
	var wg sync.WaitGroup
	var once sync.Once
	var failure error

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				if err := scanFile(path, keep, needles, visit); err != nil {
					once.Do(func() { failure = err })
					return
				}
			}
		}()
	}
	wg.Wait()
	return failure
}

func scanFile(path string, keep map[string]bool, needles [][]byte, visit func(Event)) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 1<<20)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	var ordinal int64
	// The envelope is reused across records so RawMessage decoding appends into
	// the buffer it already has instead of allocating one per record. Copying
	// every record's data value was 27.4% of analyzer allocation on a 10.89GB
	// cell. Struct decoding leaves absent fields untouched, so every field is
	// reset first and a record missing "data" still presents an empty value
	// rather than the previous record's.
	//
	// This ties an Event's payload to its visit call. No consumer retains one:
	// every Raw() result feeds a decode immediately, and decoding into a
	// RawMessage field copies.
	var env envelope
	for scanner.Scan() {
		ordinal++
		line := scanner.Bytes()
		if scanStatsEnabled {
			scanLines.Add(1)
			scanBytes.Add(int64(len(line)) + 1)
		}
		if len(needles) > 0 && !containsAny(line, needles) {
			if scanStatsEnabled {
				scanPrefilter.Add(1)
			}
			continue
		}
		if scanStatsEnabled {
			scanEnvelopes.Add(1)
		}
		env.SimTS, env.ClientID, env.Event = 0, 0, ""
		env.Data = env.Data[:0]
		if err := json.Unmarshal(line, &env); err != nil {
			return fmt.Errorf("analysis: parse evidence record in %s: %w", path, err)
		}
		if len(keep) > 0 && !keep[env.Event] {
			continue
		}
		venueID, symbol, payload, err := decodeEventLayers(env.Data)
		if err != nil {
			return fmt.Errorf("analysis: parse evidence data layer in %s: %w", path, err)
		}
		event := Event{
			SimTS: env.SimTS, ClientID: env.ClientID, Name: env.Event,
			VenueID: venueID, Symbol: symbol, File: path, Ordinal: ordinal,
			payload: payload,
		}
		if scanStatsEnabled {
			scanVisits.Add(1)
		}
		visit(event)
	}
	return scanner.Err()
}

// mayNestPayload reports whether payload can possibly carry a nested "payload"
// key, which is the only case where the derivative unwrap changes the event.
//
// It is a deliberately conservative superset of the real test, so a false
// positive costs one decode and a false negative is impossible. A JSON object
// key that unescapes to "payload" either appears literally in the raw bytes or
// uses a backslash escape, and a backslash can only occur inside a string, so
// admitting every payload containing a backslash covers escaped spellings such
// as "\u0070ayload". Records without either cannot nest, so their full decode
// (previously performed on every visited record only to be discarded) is
// skipped.
func mayNestPayload(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}
	return bytes.Contains(payload, nestedPayloadKey) || bytes.IndexByte(payload, '\\') >= 0
}

var nestedPayloadKey = []byte(`"payload"`)

func containsAny(line []byte, needles [][]byte) bool {
	for _, needle := range needles {
		if bytes.Contains(line, needle) {
			return true
		}
	}
	return false
}
