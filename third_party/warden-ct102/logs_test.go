package main

import (
	"testing"
)

func TestFilterOutput(t *testing.T) {
	input := "Jan 01 error: disk full\nJan 02 info: all good\nJan 03 error: timeout\nJan 04 warning: low memory"

	tests := []struct {
		filter   string
		expected int // number of lines in output
	}{
		{"error", 2},
		{"info", 1},
		{"warning", 1},
		{"jan", 4},   // case insensitive
		{"ERROR", 2}, // case insensitive
		{"nonexistent", 0},
		{"", 4}, // empty filter returns all
	}

	for _, tt := range tests {
		result := filterOutput(input, tt.filter)
		lines := 0
		if result != "" {
			for _, l := range splitLines(result) {
				if l != "" {
					lines++
				}
			}
		}
		if lines != tt.expected {
			t.Errorf("filterOutput(input, %q) returned %d lines, want %d", tt.filter, lines, tt.expected)
		}
	}
}

func TestFilterOutputEmpty(t *testing.T) {
	result := filterOutput("", "anything")
	if result != "" {
		t.Errorf("filterOutput empty input should return empty, got %q", result)
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
