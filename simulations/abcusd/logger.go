package abcusd

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
)

type JSONLinesLogger struct {
	mu sync.Mutex
	w  *bufio.Writer
	f  *os.File
}

func NewJSONLinesLogger(path string) (*JSONLinesLogger, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &JSONLinesLogger{f: f, w: bufio.NewWriterSize(f, 64*1024)}, nil
}

func (l *JSONLinesLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	b, _ := json.Marshal(map[string]any{
		"sim_ts":    simTime,
		"client_id": clientID,
		"event":     eventName,
		"data":      event,
	})
	l.mu.Lock()
	l.w.Write(b)
	l.w.WriteByte('\n')
	l.mu.Unlock()
}

func (l *JSONLinesLogger) Flush() {
	l.mu.Lock()
	l.w.Flush()
	l.mu.Unlock()
}

func (l *JSONLinesLogger) Close() {
	l.Flush()
	l.f.Close()
}
