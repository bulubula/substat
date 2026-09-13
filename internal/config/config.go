package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig     `yaml:"server"`
	Storage  StorageConfig    `yaml:"storage"`
	Alerts   []AlertConfig    `yaml:"alerts"`
	Monitors []MonitorConfig  `yaml:"monitors"`
}

type ServerConfig struct {
	Listen   string `yaml:"listen"`
	BasePath string `yaml:"base_path"`
}

type StorageConfig struct {
	Path           string `yaml:"path"`
	RingBufferSize int    `yaml:"ring_buffer_size"`
}

type AlertConfig struct {
	ID     string         `yaml:"id"`
	Type   string         `yaml:"type"`
	URL    string         `yaml:"url"`
	Group  string         `yaml:"group"`
	Method string         `yaml:"method"`
	Headers map[string]string `yaml:"headers"`
	Params map[string]any `yaml:",inline"`
}

type MonitorConfig struct {
	Name                string         `yaml:"name"`
	Interval            string         `yaml:"interval"`
	IntervalDuration    time.Duration  `yaml:"-"`
	Timeout             string         `yaml:"timeout"`
	TimeoutDuration     time.Duration  `yaml:"-"`
	ConsecutiveFailures int            `yaml:"consecutive_failures"`
	DegradedLatencyMs   int64          `yaml:"degraded_latency_ms"` // 延迟超过该值(ms)标黄(DEGRADED)
	Alerts              []string       `yaml:"alerts"`
	Probe               ProbeConfig    `yaml:"probe"`
}

type ProbeConfig struct {
	Type   string         `yaml:"type"`
	Params map[string]any `yaml:",inline"`
}

const (
	MinMinutes = 1
	MaxMinutes = 30 * 24 * 60 // 43200 minutes (30 days)
)

// ParseInterval converts strings like 1m, 10h, 7d into time.Duration.
// Only 'm', 'h', 'd' are allowed. Max range is [1m, 30d].
func ParseInterval(raw string) (time.Duration, error) {
	s := strings.TrimSpace(raw)
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid interval format %q: too short", raw)
	}

	unit := s[len(s)-1:]
	numStr := s[:len(s)-1]

	val, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil || val <= 0 {
		return 0, fmt.Errorf("invalid interval number in %q: must be positive integer", raw)
	}

	var minutes int64
	switch unit {
	case "m":
		minutes = val
	case "h":
		minutes = val * 60
	case "d":
		minutes = val * 24 * 60
	default:
		return 0, fmt.Errorf("invalid interval unit %q in %q: only 'm', 'h', 'd' allowed", unit, raw)
	}

	if minutes < MinMinutes || minutes > MaxMinutes {
		return 0, fmt.Errorf("interval %q (%d minutes) out of bounds: range must be 1m to 30d (1 to 43200 minutes)", raw, minutes)
	}

	return time.Duration(minutes) * time.Minute, nil
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file error: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("yaml parse error: %w", err)
	}

	if cfg.Server.Listen == "" {
		cfg.Server.Listen = "0.0.0.0:8080"
	}
	cfg.Server.BasePath = strings.TrimSuffix(cfg.Server.BasePath, "/")
	if cfg.Server.BasePath != "" && !strings.HasPrefix(cfg.Server.BasePath, "/") {
		cfg.Server.BasePath = "/" + cfg.Server.BasePath
	}

	if cfg.Storage.Path == "" {
		cfg.Storage.Path = "./data/substat.db"
	}
	if cfg.Storage.RingBufferSize <= 0 {
		cfg.Storage.RingBufferSize = 120
	}

	names := make(map[string]bool)
	for i := range cfg.Monitors {
		m := &cfg.Monitors[i]
		if m.Name == "" {
			return nil, fmt.Errorf("monitor at index %d has empty name", i)
		}
		if names[m.Name] {
			return nil, fmt.Errorf("duplicate monitor name %q", m.Name)
		}
		names[m.Name] = true

		d, err := ParseInterval(m.Interval)
		if err != nil {
			return nil, fmt.Errorf("monitor %q interval error: %w", m.Name, err)
		}
		m.IntervalDuration = d

		if m.Timeout == "" {
			m.TimeoutDuration = 10 * time.Second
		} else {
			td, err := time.ParseDuration(m.Timeout)
			if err != nil {
				return nil, fmt.Errorf("monitor %q timeout parse error: %w", m.Name, err)
			}
			m.TimeoutDuration = td
		}

		if m.ConsecutiveFailures <= 0 {
			m.ConsecutiveFailures = 1
		}
		if m.DegradedLatencyMs <= 0 {
			m.DegradedLatencyMs = 1000 // 默认 1000ms
		}
	}

	return &cfg, nil
}
