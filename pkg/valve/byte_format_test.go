package valve_test

import (
	"testing"

	"github.com/gregory-chatelier/valve/pkg/valve"
)

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantBytes int
		wantErr   bool
	}{
		{"Lowercase k", "1k", 0, true},
		{"Uppercase K", "1K", 0, true},
		{"Uppercase KB", "2KB", 0, true},
		{"Lowercase m", "1m", 1024 * 1024, false},
		{"Uppercase M", "1M", 1024 * 1024, false},
		{"Uppercase MB", "2MB", 2 * 1024 * 1024, false},
		{"Simple GB", "1GB", 0, true}, // Over MaxBufferSize
		{"Simple B", "512B", 0, true}, // Below MinBufferSize
		{"Decimal KB", "1.5KB", 0, true},
		{"Decimal M", "0.5M", 512 * 1024, false},
		{"Whitespace", "  2 MB  ", 2 * 1024 * 1024, false},
		{"Max size allowed", "5MB", 5 * 1024 * 1024, false},
		{"Over max size", "6MB", 0, true},
		{"Invalid format", "10 M B", 0, true},
		{"Invalid unit", "10TB", 0, true},
		{"No number", "MB", 0, true},
		{"No unit", "1024", 0, true},
		{"Zero bytes", "0B", 0, true},
		{"Below min bytes (1B)", "1B", 0, true},
		{"Below min bytes (3KB)", "3KB", 0, true},
		{"Exactly min bytes (4KB)", "4KB", 4 * 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBytes, err := valve.ParseByteSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseByteSize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotBytes != tt.wantBytes {
				t.Errorf("ParseByteSize() = %v, want %v", gotBytes, tt.wantBytes)
			}
		})
	}
}
