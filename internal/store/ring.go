package store

import (
	"sync"
	"time"
)

type Point struct {
	Timestamp time.Time `json:"time"`
	Success   bool      `json:"success"`
	LatencyMs int64     `json:"latency_ms"`
	Message   string    `json:"message,omitempty"`
}

type RingBuffer struct {
	mu       sync.RWMutex
	capacity int
	data     []Point
	head     int
	count    int
}

func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 60
	}
	return &RingBuffer{
		capacity: capacity,
		data:     make([]Point, capacity),
	}
}

func (r *RingBuffer) Push(p Point) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[r.head] = p
	r.head = (r.head + 1) % r.capacity
	if r.count < r.capacity {
		r.count++
	}
}

// GetAll returns items in chronological order (oldest to newest)
func (r *RingBuffer) GetAll() []Point {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]Point, r.count)
	if r.count == 0 {
		return res
	}

	start := 0
	if r.count == r.capacity {
		start = r.head
	}

	for i := 0; i < r.count; i++ {
		idx := (start + i) % r.capacity
		res[i] = r.data[idx]
	}
	return res
}
