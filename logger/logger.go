package logger

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

type Logger struct {
	w  io.Writer
	mu sync.Mutex
}

func New(w io.Writer) *Logger {
	if w == nil {
		return nil
	}
	return &Logger{w: w}
}

// LogEvent serializes any struct with JSON tags
func (l *Logger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Build log entry
	entry := map[string]any{
		"sim_time":    simTime,
		"server_time": time.Now().UnixNano(),
		"event":       eventName,
		"client_id":   clientID,
	}

	// Merge event fields via JSON
	if event != nil {
		eventBytes, _ := json.Marshal(event)
		var eventFields map[string]any
		json.Unmarshal(eventBytes, &eventFields)
		for k, v := range eventFields {
			entry[k] = v
		}
	}

	// Write JSON line
	line, _ := json.Marshal(entry)
	l.w.Write(line)
	l.w.Write([]byte("\n"))
}
