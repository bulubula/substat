package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	filePath string
	mu       sync.Mutex
	ringsMu  sync.RWMutex
	rings    map[string]*RingBuffer
	ringCap  int
	file     *os.File
}

type RecordEntry struct {
	MonitorName string    `json:"monitor_name"`
	Timestamp   time.Time `json:"timestamp"`
	Success     bool      `json:"success"`
	LatencyMs   int64     `json:"latency_ms"`
	Message     string    `json:"message,omitempty"`
}

func NewStore(filePath string, ringCap int) (*Store, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create store dir error: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open store file error: %w", err)
	}

	return &Store{
		filePath: filePath,
		rings:    make(map[string]*RingBuffer),
		ringCap:  ringCap,
		file:     f,
	}, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

func (s *Store) Record(monitorName string, success bool, latency time.Duration, msg string) {
	pt := Point{
		Timestamp: time.Now(),
		Success:   success,
		LatencyMs: latency.Milliseconds(),
		Message:   msg,
	}

	// 1. Update in-memory ring
	s.ringsMu.Lock()
	r, exists := s.rings[monitorName]
	if !exists {
		r = NewRingBuffer(s.ringCap)
		s.rings[monitorName] = r
	}
	s.ringsMu.Unlock()
	r.Push(pt)

	// 2. Append to JSONL history file
	entry := RecordEntry{
		MonitorName: monitorName,
		Timestamp:   pt.Timestamp,
		Success:     pt.Success,
		LatencyMs:   pt.LatencyMs,
		Message:     pt.Message,
	}

	go func() {
		data, err := json.Marshal(entry)
		if err != nil {
			return
		}
		data = append(data, '\n')

		s.mu.Lock()
		defer s.mu.Unlock()
		if s.file != nil {
			_, _ = s.file.Write(data)
		}
	}()
}

func (s *Store) GetRecent(monitorName string) []Point {
	s.ringsMu.RLock()
	r, exists := s.rings[monitorName]
	s.ringsMu.RUnlock()

	if !exists {
		return nil
	}
	return r.GetAll()
}
