package probe

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Result struct {
	Timestamp time.Time      `json:"timestamp"`
	Success   bool           `json:"success"`
	Latency   time.Duration  `json:"latency"`
	Message   string         `json:"message,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type Probe interface {
	Execute(ctx context.Context) Result
}

type Factory func(params map[string]any) (Probe, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

func Register(probeType string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[probeType] = factory
}

func Create(probeType string, params map[string]any) (Probe, error) {
	registryMu.RLock()
	factory, ok := registry[probeType]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown probe type %q", probeType)
	}
	return factory(params)
}
