package analysis

import (
	"bufio"
	"encoding/json"
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

	payload json.RawMessage
}

// Decode unmarshals the event's innermost payload.
func (e *Event) Decode(target any) error { return json.Unmarshal(e.payload, target) }

// Raw returns the innermost payload without decoding it.
func (e *Event) Raw() json.RawMessage { return e.payload }

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

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	jobs := make(chan string)
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
	for _, path := range files {
		jobs <- path
	}
	close(jobs)
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
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(needles) > 0 && !containsAny(line, needles) {
			continue
		}
		var env envelope
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}
		if len(keep) > 0 && !keep[env.Event] {
			continue
		}
		var outer dataLayer
		if err := json.Unmarshal(env.Data, &outer); err != nil {
			continue
		}
		event := Event{
			SimTS: env.SimTS, ClientID: env.ClientID, Name: env.Event,
			VenueID: outer.VenueID, Symbol: outer.Symbol, File: path,
			payload: outer.Payload,
		}
		// Unwrap the derivative nesting: an inner payload means the fields sit
		// one level down and the symbol travels with them.
		var inner dataLayer
		if len(outer.Payload) > 0 && json.Unmarshal(outer.Payload, &inner) == nil && len(inner.Payload) > 0 {
			if inner.Symbol != "" {
				event.Symbol = inner.Symbol
			}
			event.payload = inner.Payload
		}
		visit(event)
	}
	return scanner.Err()
}

func containsAny(line []byte, needles [][]byte) bool {
	for _, needle := range needles {
		if bytesContains(line, needle) {
			return true
		}
	}
	return false
}

func bytesContains(haystack, needle []byte) bool {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}
	first := needle[0]
	limit := len(haystack) - len(needle)
	for i := 0; i <= limit; i++ {
		if haystack[i] != first {
			continue
		}
		match := true
		for j := 1; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
