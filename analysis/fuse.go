package analysis

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
)

// Fused extraction removes the read amplification of one-metric-per-process
// extraction without merging any scientific conclusion.
//
// A registered extraction computes about thirty derived artifacts, each in its
// own process, each performing its own physical scan. Measured on the retained
// 684MB seed-101 development cell that is 18.00GB read and 55.95M line scans to
// examine 2.13M records: every byte is read and prefiltered about twenty-six
// times. The cost is entirely in re-reading, re-prefiltering and re-decoding the
// shared record envelope.
//
// The fusion below shares exactly that envelope work and nothing else. Each task
// still runs its own reducer over its own immutable Event and reaches its own
// verdict, so the independent-reconstruction property that makes the evidence
// gates able to catch a bug is preserved: no task can observe, and therefore
// cannot borrow, another task's derived state.
//
// The fused pass reproduces the per-task semantics of Scan exactly:
//
//   - a task sees a record if and only if its own event filter admits it;
//   - a task's records arrive in physical order within each file, and files are
//     visited concurrently, which is what Scan already promises;
//   - Ordinal remains the physical record position, counting filtered records;
//   - a task whose own prefilter admits a malformed record fails with the same
//     error and stops consuming that file, while tasks whose prefilter rejects
//     it are unaffected, as they are today.
//
// Parallelism is the one deliberate difference: workers are chosen once for the
// fused pass rather than per Scan call. Scan already documents that visit is
// called concurrently from several goroutines, so no task may depend on the
// worker count.

// FusedTask is one metric computation to run inside a fused extraction. Compute
// must perform the whole metric and may call Scan any number of times.
type FusedTask struct {
	Name    string
	Compute func(*Run) error
}

// RunFused executes tasks concurrently over shared physical passes and returns
// each task's error positionally.
//
// The returned slice always has one entry per task. Tasks are independent: one
// task's failure neither aborts nor alters another's.
func (r *Run) RunFused(tasks []FusedTask, workers int) []error {
	errors := make([]error, len(tasks))
	if len(tasks) == 0 {
		return errors
	}
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	pass := &fusedPass{live: len(tasks), workers: workers}
	pass.ready = sync.NewCond(&pass.mu)

	// Every task observes the same run, differing only in that Scan routes
	// through the coordinator. Run is immutable after Open, so sharing it adds
	// no coupling between tasks.
	fused := *r
	fused.fuse = pass

	var wg sync.WaitGroup
	for i := range tasks {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer pass.retire()
			errors[index] = tasks[index].Compute(&fused)
		}(i)
	}
	go pass.coordinate(r)
	wg.Wait()
	return errors
}

// fusedPass collects the scan requests of parked tasks and serves them in
// rounds, one physical pass per round.
type fusedPass struct {
	mu      sync.Mutex
	ready   *sync.Cond
	parked  []*fusedRequest
	live    int
	workers int
}

type fusedRequest struct {
	files   []string
	keep    map[string]bool
	needles [][]byte
	visit   func(Event)
	// serial marks a request that asked for a single scan worker. Such a
	// metric relies on seeing every file from one goroutine in file order,
	// both for unsynchronised reducer state and for cross-file causal order,
	// so it may only be fused with other serial requests.
	serial bool

	// once and failure record the first error observed for this request, which
	// is what Scan reports.
	once    sync.Once
	failure error

	done chan struct{}
}

func (p *fusedPass) retire() {
	p.mu.Lock()
	p.live--
	p.mu.Unlock()
	p.ready.Broadcast()
}

// submit parks the calling task until the coordinator has served its request.
func (p *fusedPass) submit(request *fusedRequest) error {
	p.mu.Lock()
	p.parked = append(p.parked, request)
	p.mu.Unlock()
	p.ready.Broadcast()
	<-request.done
	return request.failure
}

// coordinate serves rounds until no task remains. A round begins once every
// task that is still running is parked in Scan, which maximises how much
// envelope work a single physical pass can serve.
func (p *fusedPass) coordinate(source *Run) {
	for {
		p.mu.Lock()
		for p.live > 0 && len(p.parked) < p.live {
			p.ready.Wait()
		}
		if p.live == 0 && len(p.parked) == 0 {
			p.mu.Unlock()
			return
		}
		round := p.parked
		p.parked = nil
		p.mu.Unlock()

		runFusedRound(source, round, p.workers)
		for _, request := range round {
			close(request.done)
		}
	}
}

// scanFused is the Scan implementation used inside a fused extraction.
func (p *fusedPass) scanFused(source *Run, opts ScanOptions, visit func(Event)) error {
	files := opts.Files
	if files == nil && !opts.FilesSelected {
		files = source.files
	}
	keep := make(map[string]bool, len(opts.Events))
	for _, name := range opts.Events {
		keep[name] = true
	}
	needles := make([][]byte, 0, len(opts.Events))
	for _, name := range opts.Events {
		needles = append(needles, []byte(`"`+name+`"`))
	}
	return p.submit(&fusedRequest{
		files: files, keep: keep, needles: needles, visit: visit,
		serial: opts.Workers == 1,
		done:   make(chan struct{}),
	})
}

// runFusedRound performs one round, serving every request in it.
//
// A request that asked for one worker keeps that guarantee: such requests are
// served by a single goroutine walking files in the run's indexed order, which
// is exactly the sequence a one-worker Scan produces today. They are fused with
// each other but never with the parallel group, and the two groups run
// concurrently because they share no state.
func runFusedRound(source *Run, round []*fusedRequest, workers int) {
	var serial, parallel, isolated []*fusedRequest
	for _, request := range round {
		switch {
		case !request.serial:
			parallel = append(parallel, request)
		case isFileSubsequence(source.files, request.files):
			serial = append(serial, request)
		default:
			// A serial request whose file order is not the run's own order
			// cannot be merged without changing the sequence it observes.
			isolated = append(isolated, request)
		}
	}
	var wg sync.WaitGroup
	launch := func(group []*fusedRequest, groupWorkers int) {
		if len(group) == 0 {
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			runFusedGroup(source, group, groupWorkers)
		}()
	}
	launch(parallel, workers)
	launch(serial, 1)
	for _, request := range isolated {
		launch([]*fusedRequest{request}, 1)
	}
	wg.Wait()
}

// isFileSubsequence reports whether files appears within order in the same
// relative sequence, which is what lets a serial request share a pass driven by
// the run's file order.
func isFileSubsequence(order, files []string) bool {
	next := 0
	for _, path := range files {
		found := false
		for next < len(order) {
			if order[next] == path {
				next++
				found = true
				break
			}
			next++
		}
		if !found {
			return false
		}
	}
	return true
}

// runFusedGroup performs one physical pass serving every request in the group.
func runFusedGroup(source *Run, group []*fusedRequest, workers int) {
	// Index requests by file so a file is read once for everyone that wants it.
	byFile := make(map[string][]*fusedRequest)
	for _, request := range group {
		for _, path := range request.files {
			byFile[path] = append(byFile[path], request)
		}
	}
	if len(byFile) == 0 {
		return
	}
	// Visit files in the run's indexed order so the pass is reproducible.
	paths := make([]string, 0, len(byFile))
	for _, path := range source.files {
		if _, wanted := byFile[path]; wanted {
			paths = append(paths, path)
		}
	}
	for _, request := range group {
		for _, path := range request.files {
			if !containsString(paths, path) {
				paths = append(paths, path)
			}
		}
	}

	if scanStatsEnabled {
		scanCalls.Add(1)
		scanFiles.Add(int64(len(paths)))
	}
	jobs := make(chan string, len(paths))
	for _, path := range paths {
		jobs <- path
	}
	close(jobs)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				scanFileFused(path, byFile[path])
			}
		}()
	}
	wg.Wait()
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// fileConsumer is one request's participation in one file. Stopped records that
// the request has already failed on this file and must receive nothing further
// from it, which is how a per-file scan behaves today once it returns an error.
type fileConsumer struct {
	request *fusedRequest
	stopped bool
}

func scanFileFused(path string, requests []*fusedRequest) {
	file, err := os.Open(path)
	if err != nil {
		for _, request := range requests {
			request.fail(err)
		}
		return
	}
	defer file.Close()

	consumers := make([]fileConsumer, len(requests))
	for i, request := range requests {
		consumers[i] = fileConsumer{request: request}
	}
	// A request with no event filter admits every record, so the union
	// prefilter cannot skip any line while one is present.
	unionNeedles, filterAll := unionPrefilter(requests)
	// Dispatch by event name; the per-request needle test still runs before a
	// record is delivered, because an event name that JSON-escapes differently
	// than its needle spelling must remain filtered exactly as it is today.
	byEvent := make(map[string][]int, 16)
	var always []int
	for i, request := range requests {
		if len(request.keep) == 0 {
			always = append(always, i)
			continue
		}
		for name := range request.keep {
			byEvent[name] = append(byEvent[name], i)
		}
	}

	reader := bufio.NewReaderSize(file, 1<<20)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	var ordinal int64
	for scanner.Scan() {
		ordinal++
		if allStopped(consumers) {
			return
		}
		line := scanner.Bytes()
		if scanStatsEnabled {
			scanLines.Add(1)
			scanBytes.Add(int64(len(line)) + 1)
		}
		if filterAll && !containsAny(line, unionNeedles) {
			if scanStatsEnabled {
				scanPrefilter.Add(1)
			}
			continue
		}
		if scanStatsEnabled {
			scanEnvelopes.Add(1)
		}
		var env envelope
		if err := json.Unmarshal(line, &env); err != nil {
			// Only a request whose own prefilter admitted this line would have
			// decoded it, and therefore only such a request fails.
			wrapped := fmt.Errorf("analysis: parse evidence record in %s: %w", path, err)
			for i := range consumers {
				if consumers[i].stopped || !admits(consumers[i].request, line) {
					continue
				}
				consumers[i].request.fail(wrapped)
				consumers[i].stopped = true
			}
			continue
		}
		selected := selectConsumers(consumers, byEvent, always, env.Event, line)
		if len(selected) == 0 {
			continue
		}
		var outer dataLayer
		if err := json.Unmarshal(env.Data, &outer); err != nil {
			wrapped := fmt.Errorf("analysis: parse evidence data layer in %s: %w", path, err)
			for _, i := range selected {
				consumers[i].request.fail(wrapped)
				consumers[i].stopped = true
			}
			continue
		}
		event := Event{
			SimTS: env.SimTS, ClientID: env.ClientID, Name: env.Event,
			VenueID: outer.VenueID, Symbol: outer.Symbol, File: path, Ordinal: ordinal,
			payload: outer.Payload,
		}
		if mayNestPayload(outer.Payload) {
			var inner dataLayer
			if json.Unmarshal(outer.Payload, &inner) == nil && len(inner.Payload) > 0 {
				if inner.Symbol != "" {
					event.Symbol = inner.Symbol
				}
				event.payload = inner.Payload
			}
		}
		if scanStatsEnabled {
			scanVisits.Add(int64(len(selected)))
		}
		for _, i := range selected {
			consumers[i].request.visit(event)
		}
	}
	if err := scanner.Err(); err != nil {
		for i := range consumers {
			if !consumers[i].stopped {
				consumers[i].request.fail(err)
			}
		}
	}
}

// selectConsumers returns the indices of consumers that must receive this
// record, reusing a scratch slice is deliberately avoided because visit may
// retain nothing but the event, and the slice is small.
func selectConsumers(consumers []fileConsumer, byEvent map[string][]int, always []int, name string, line []byte) []int {
	var selected []int
	for _, i := range always {
		if !consumers[i].stopped {
			selected = append(selected, i)
		}
	}
	for _, i := range byEvent[name] {
		if consumers[i].stopped {
			continue
		}
		if !admits(consumers[i].request, line) {
			continue
		}
		selected = append(selected, i)
	}
	return selected
}

// admits reports whether a request's own raw prefilter accepts the line, which
// is the gate a per-metric scan applies before it decodes anything.
func admits(request *fusedRequest, line []byte) bool {
	return len(request.needles) == 0 || containsAny(line, request.needles)
}

func allStopped(consumers []fileConsumer) bool {
	for i := range consumers {
		if !consumers[i].stopped {
			return false
		}
	}
	return true
}

// unionPrefilter builds the deduplicated needle set for a round. filterAll is
// false when some request admits every record, in which case no line may be
// skipped before decoding.
func unionPrefilter(requests []*fusedRequest) (needles [][]byte, filterAll bool) {
	seen := make(map[string]bool)
	for _, request := range requests {
		if len(request.needles) == 0 {
			return nil, false
		}
		for _, needle := range request.needles {
			if seen[string(needle)] {
				continue
			}
			seen[string(needle)] = true
			needles = append(needles, needle)
		}
	}
	return needles, true
}

func (r *fusedRequest) fail(err error) {
	r.once.Do(func() { r.failure = err })
}
