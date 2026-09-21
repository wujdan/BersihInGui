package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event types recorded in the audit trail.
const (
	EventScanStarted   = "scan_started"
	EventScanComplete  = "scan_complete"
	EventQuarantineMove = "quarantine_move"
	EventRestore       = "restore"
	EventPurge         = "purge"
	EventValidationSkip = "validation_skip"
	EventError         = "error"
)

// Entry is a single audit/log record.
type Entry struct {
	Timestamp time.Time              `json:"timestamp"`
	Event     string                 `json:"event"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// Logger writes audit trail entries to rotating JSONL files.
type Logger struct {
	mu       sync.Mutex
	dir      string
	file     *os.File
	dayKey   string
}

// New creates a logger writing into dir.
func New(dir string) (*Logger, error) {
	l := &Logger{dir: dir}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	if err := l.rotate(); err != nil {
		return nil, err
	}
	return l, nil
}

// rotate opens a new daily file if the day changed.
func (l *Logger) rotate() error {
	key := time.Now().Format("2006-01-02")
	if l.file != nil && l.dayKey == key {
		return nil
	}
	name := filepath.Join(l.dir, fmt.Sprintf("audit-%s.jsonl", key))
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open audit file: %w", err)
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	l.file = f
	l.dayKey = key
	return nil
}

// write serializes and appends an entry.
func (l *Logger) write(level, event, msg string, data map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.rotate(); err != nil {
		return
	}
	entry := Entry{
		Timestamp: time.Now().UTC(),
		Event:     event,
		Level:     level,
		Message:   msg,
		Data:      data,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = l.file.Write(append(line, '\n'))
}

// Info records an informational event.
func (l *Logger) Info(event, msg string, data map[string]interface{}) {
	l.write("info", event, msg, data)
}

// Warn records a warning event.
func (l *Logger) Warn(event, msg string, data map[string]interface{}) {
	l.write("warn", event, msg, data)
}

// Error records an error event.
func (l *Logger) Error(event, msg string, data map[string]interface{}) {
	l.write("error", event, msg, data)
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}