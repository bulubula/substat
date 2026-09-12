package config

import (
	"testing"
	"time"
)

func TestParseInterval(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"1m", 1 * time.Minute, false},
		{"5m", 5 * time.Minute, false},
		{"1h", 60 * time.Minute, false},
		{"24h", 24 * 60 * time.Minute, false},
		{"1d", 24 * time.Minute * 60, false},
		{"30d", 30 * 24 * 60 * time.Minute, false},
		{"0m", 0, true},
		{"10s", 0, true},        // 's' not allowed
		{"800h", 0, true},       // 800h > 720h (30d)
		{"31d", 0, true},        // > 30d
		{"invalid", 0, true},
		{"-5m", 0, true},
	}

	for _, tt := range tests {
		res, err := ParseInterval(tt.input)
		if tt.wantErr && err == nil {
			t.Errorf("ParseInterval(%q) expected error, got nil", tt.input)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("ParseInterval(%q) unexpected error: %v", tt.input, err)
		}
		if !tt.wantErr && res != tt.expected {
			t.Errorf("ParseInterval(%q) = %v; want %v", tt.input, res, tt.expected)
		}
	}
}
