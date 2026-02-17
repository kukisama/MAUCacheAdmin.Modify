package logging

import (
	"testing"
)

func TestNewLoggerLevels(t *testing.T) {
	levels := []struct {
		input string
		valid bool
	}{
		{"debug", true},
		{"info", true},
		{"warn", true},
		{"error", true},
		{"unknown", true}, // defaults to info
	}
	for _, tt := range levels {
		t.Run(tt.input, func(t *testing.T) {
			logger := New(tt.input, "json")
			if logger == nil {
				t.Fatalf("New(%q, %q) returned nil", tt.input, "json")
			}
		})
	}
}

func TestNewLoggerFormats(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			logger := New("info", format)
			if logger == nil {
				t.Fatalf("New(%q, %q) returned nil", "info", format)
			}
		})
	}
}

func TestNewLoggerDebugEnabled(t *testing.T) {
	logger := New("debug", "json")
	if !logger.Enabled(nil, -4) { // slog.LevelDebug = -4
		t.Error("debug logger should have debug level enabled")
	}
}

func TestNewLoggerInfoDoesNotEnableDebug(t *testing.T) {
	logger := New("info", "json")
	if logger.Enabled(nil, -4) { // slog.LevelDebug = -4
		t.Error("info logger should NOT have debug level enabled")
	}
}
