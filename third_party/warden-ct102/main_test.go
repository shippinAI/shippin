package main

import (
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    uint64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.input)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h 30m"},
		{25 * time.Hour, "1d 1h 0m"},
		{49*time.Hour + 30*time.Minute, "2d 1h 30m"},
	}

	for _, tt := range tests {
		result := formatUptime(tt.input)
		if result != tt.expected {
			t.Errorf("formatUptime(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
