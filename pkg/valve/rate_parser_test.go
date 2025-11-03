package valve_test

import (
	"testing"

	"github.com/gregory-chatelier/valve/pkg/valve"
)

func TestParseRate(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantRate  float64
		wantBytes bool
		wantErr   bool
	}{
		{"Items per second", "10/s", 10, false, false},
		{"Items per minute", "60/m", 1, false, false},
		{"Items per hour", "3600/h", 1, false, false},
		{"Kilobytes per second", "10KB/s", 10 * 1024, true, false},
		{"Megabytes per second", "10MB/s", 10 * 1024 * 1024, true, false},
		{"Gigabytes per second", "10GB/s", 10 * 1024 * 1024 * 1024, true, false},
		{"Invalid format", "10", 0, false, true},
		{"Invalid number", "abc/s", 0, false, true},
		{"Invalid unit", "10/x", 0, false, true},
		{"Invalid byte unit", "10XB/s", 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRate, gotBytes, err := valve.ParseRate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotRate != tt.wantRate {
				t.Errorf("ParseRate() gotRate = %v, want %v", gotRate, tt.wantRate)
			}
			if gotBytes != tt.wantBytes {
				t.Errorf("ParseRate() gotBytes = %v, want %v", gotBytes, tt.wantBytes)
			}
		})
	}
}
