package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"bulubula/substat/internal/alert"
	"bulubula/substat/internal/config"
	"bulubula/substat/internal/probe"
	"bulubula/substat/internal/store"
)

type MonitorState struct {
	Name              string         `json:"name"`
	Type              string         `json:"type"`
	Interval          string         `json:"interval"`
	Status            string         `json:"status"` // "UP", "DEGRADED", "DOWN", "PENDING"
	LastCheck         time.Time      `json:"last_check"`
	LastLatencyMs     int64          `json:"last_latency_ms"`
	AvgLatencyMs      int64          `json:"avg_latency_ms"`
	LastMessage       string         `json:"last_message,omitempty"`
	ConsecutiveFails  int            `json:"consecutive_fails"`
	UptimeDay         float64        `json:"uptime_day"`
	DegradedLatencyMs int64          `json:"degraded_latency_ms"`
	Details           map[string]any `json:"details,omitempty"`
}

type Scheduler struct {
	cfg       *config.Config
	store     *store.Store
	notifiers map[string]alert.Notifier
	probes    map[string]probe.Probe
	statesMu  sync.RWMutex
	states    map[string]*MonitorState
	cancel    context.CancelFunc
}

func NewScheduler(cfg *config.Config, s *store.Store) (*Scheduler, error) {
	// Initialize notifiers
	notifiers := make(map[string]alert.Notifier)
	for _, a := range cfg.Alerts {
		n, err := alert.Create(a.Type, a.Params)
		if err != nil {
			log.Printf("[WARN] Failed to initialize alert %s (%s): %v", a.ID, a.Type, err)
			continue
		}
		notifiers[a.ID] = n
	}

	// Initialize probes
	probes := make(map[string]probe.Probe)
	states := make(map[string]*MonitorState)

	for _, m := range cfg.Monitors {
		p, err := probe.Create(m.Probe.Type, m.Probe.Params)
		if err != nil {
			return nil, err
		}
		probes[m.Name] = p
		states[m.Name] = &MonitorState{
			Name:              m.Name,
			Type:              m.Probe.Type,
			Interval:          m.Interval,
			DegradedLatencyMs: m.DegradedLatencyMs,
			Status:            "PENDING",
		}
	}

	return &Scheduler{
		cfg:       cfg,
		store:     s,
		notifiers: notifiers,
		probes:    probes,
		states:    states,
	}, nil
}

func (s *Scheduler) Start(parentCtx context.Context) {
	ctx, cancel := context.WithCancel(parentCtx)
	s.cancel = cancel

	for _, m := range s.cfg.Monitors {
		go s.runWorker(ctx, m)
	}
}

func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Scheduler) runWorker(ctx context.Context, m config.MonitorConfig) {
	pr := s.probes[m.Name]
	ticker := time.NewTicker(m.IntervalDuration)
	defer ticker.Stop()

	// Initial immediate check
	s.executeCheck(ctx, m, pr)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.executeCheck(ctx, m, pr)
		}
	}
}

func (s *Scheduler) executeCheck(parentCtx context.Context, m config.MonitorConfig, pr probe.Probe) {
	timeout := m.TimeoutDuration
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	res := pr.Execute(ctx)

	// Save to store
	s.store.Record(m.Name, res.Success, res.Latency, res.Message)

	// Calculate rolling average latency from ring buffer
	recentPoints := s.store.GetRecent(m.Name)
	var sumLatency int64
	var validCount int64
	for _, p := range recentPoints {
		if p.Success && p.LatencyMs > 0 {
			sumLatency += p.LatencyMs
			validCount++
		}
	}
	var avgLatency int64
	if validCount > 0 {
		avgLatency = sumLatency / validCount
	}

	// Evaluate state transition
	s.statesMu.Lock()
	state := s.states[m.Name]
	state.LastCheck = res.Timestamp
	state.LastLatencyMs = res.Latency.Milliseconds()
	state.AvgLatencyMs = avgLatency
	state.LastMessage = res.Message
	state.Details = res.Details

	var alertToSend *alert.Event

	if res.Success {
		newStatus := "UP"
		if state.LastLatencyMs > m.DegradedLatencyMs {
			newStatus = "DEGRADED"
		}

		if state.Status == "DOWN" {
			// Recovered
			state.Status = newStatus
			alertToSend = &alert.Event{
				MonitorName: m.Name,
				EventType:   alert.EventRecovered,
				Timestamp:   time.Now(),
				Latency:     res.Latency,
				Message:     "Service recovered.",
			}
		} else {
			state.Status = newStatus
		}
		state.ConsecutiveFails = 0
	} else {
		state.ConsecutiveFails++
		if state.ConsecutiveFails >= m.ConsecutiveFailures {
			if state.Status != "DOWN" {
				state.Status = "DOWN"
				alertToSend = &alert.Event{
					MonitorName: m.Name,
					EventType:   alert.EventDown,
					Timestamp:   time.Now(),
					Latency:     res.Latency,
					Message:     res.Message,
					FailCount:   state.ConsecutiveFails,
				}
			}
		}
	}
	s.statesMu.Unlock()

	// Dispatch alerts
	if alertToSend != nil {
		for _, alertID := range m.Alerts {
			if notifier, ok := s.notifiers[alertID]; ok {
				go func(n alert.Notifier, ev alert.Event) {
					dispatchCtx, dCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer dCancel()
					if err := n.Send(dispatchCtx, ev); err != nil {
						log.Printf("[ALERT-ERR] %s send error: %v", alertID, err)
					}
				}(notifier, *alertToSend)
			}
		}
	}
}

func (s *Scheduler) GetStates() []MonitorState {
	s.statesMu.RLock()
	defer s.statesMu.RUnlock()

	res := make([]MonitorState, 0, len(s.cfg.Monitors))
	for _, m := range s.cfg.Monitors {
		if st, ok := s.states[m.Name]; ok {
			res = append(res, *st)
		}
	}
	return res
}
