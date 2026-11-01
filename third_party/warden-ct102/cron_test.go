package main

import (
	"testing"
)

func TestParseCronLine(t *testing.T) {
	tests := []struct {
		line    string
		isNil   bool
		minute  string
		hour    string
		day     string
		month   string
		weekday string
		command string
	}{
		{"* * * * * /usr/bin/backup.sh", false, "*", "*", "*", "*", "*", "/usr/bin/backup.sh"},
		{"0 2 * * * /usr/bin/cleanup.sh --force", false, "0", "2", "*", "*", "*", "/usr/bin/cleanup.sh --force"},
		{"*/5 * * * * echo hello world", false, "*/5", "*", "*", "*", "*", "echo hello world"},
		{"0 0 1 * * /monthly/task", false, "0", "0", "1", "*", "*", "/monthly/task"},
		{"bad line", true, "", "", "", "", "", ""},
		{"", true, "", "", "", "", "", ""},
		{"# comment line", true, "", "", "", "", "", ""},
	}

	for _, tt := range tests {
		result := parseCronLine(tt.line, 0)
		if tt.isNil {
			if result != nil {
				t.Errorf("parseCronLine(%q) should be nil, got %+v", tt.line, result)
			}
			continue
		}
		if result == nil {
			t.Errorf("parseCronLine(%q) = nil, want non-nil", tt.line)
			continue
		}
		if result.Minute != tt.minute {
			t.Errorf("parseCronLine(%q).Minute = %q, want %q", tt.line, result.Minute, tt.minute)
		}
		if result.Hour != tt.hour {
			t.Errorf("parseCronLine(%q).Hour = %q, want %q", tt.line, result.Hour, tt.hour)
		}
		if result.Day != tt.day {
			t.Errorf("parseCronLine(%q).Day = %q, want %q", tt.line, result.Day, tt.day)
		}
		if result.Month != tt.month {
			t.Errorf("parseCronLine(%q).Month = %q, want %q", tt.line, result.Month, tt.month)
		}
		if result.Weekday != tt.weekday {
			t.Errorf("parseCronLine(%q).Weekday = %q, want %q", tt.line, result.Weekday, tt.weekday)
		}
		if result.Command != tt.command {
			t.Errorf("parseCronLine(%q).Command = %q, want %q", tt.line, result.Command, tt.command)
		}
	}
}

func TestParseCronLineSchedule(t *testing.T) {
	result := parseCronLine("30 2 * * 0 /weekly/backup", 5)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Index != 5 {
		t.Errorf("Index = %d, want 5", result.Index)
	}
	if result.Schedule != "30 2 * * 0" {
		t.Errorf("Schedule = %q, want %q", result.Schedule, "30 2 * * 0")
	}
	if result.Raw != "30 2 * * 0 /weekly/backup" {
		t.Errorf("Raw = %q, want %q", result.Raw, "30 2 * * 0 /weekly/backup")
	}
}
