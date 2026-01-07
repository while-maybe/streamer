package config

import (
	"testing"
)

func TestParseBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected int64
		wantErr  bool
	}{
		{"ok - unit MB", "10MB", 10 * 1024 * 1024, false},
		{"ok - case insesitive", "10mb", 10 * 1024 * 1024, false},
		{"ok - unit KB", "5kb", 5 * 1024, false},
		{"ok - unit GB", "1GB", 1 * 1024 * 1024 * 1024, false},
		{"ok - no unit", "1024", 1024, false},
		{"ok - handles space", "10 MB", 10 * 1024 * 1024, false},
		{"fail - bad unit", "10XiB", 0, true},
		{"fail - rubbish", "invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBytes(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}

			if got != tt.expected {
				t.Errorf("parseBytes(%q) = %d, want %d", tt.input, got, tt.expected)
			}

		})
	}
}
