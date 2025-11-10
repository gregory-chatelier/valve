package valve_test

import (
	"testing"

	"github.com/gregory-chatelier/valve/pkg/valve"
	"github.com/stretchr/testify/require"
)

func TestParseByteSize(t *testing.T) {
	const KB = 1024
	const MB = 1024 * 1024

	tests := []struct {
		name      string
		input     string
		wantBytes int
		wantErr   bool
	}{
		// Valid cases
		{"Lowercase k", "4k", 4 * KB, false},
		{"Uppercase K", "4K", 4 * KB, false},
		{"Uppercase KB", "8KB", 8 * KB, false},
		{"Lowercase m", "1m", 1 * MB, false},
		{"Uppercase M", "1M", 1 * MB, false},
		{"Uppercase MB", "2MB", 2 * MB, false},
		{"Decimal M", "0.5M", 512 * KB, false},
		{"Whitespace", "  2 MB  ", 2 * MB, false},
		{"Max size allowed", "5MB", 5 * MB, false},
		{"Exactly min bytes (4KB)", "4KB", 4 * KB, false},
		{"Unit-less number (as bytes)", "8192", 8192, false},

		// Error cases
		{"Simple GB (Over limit)", "1GB", 0, true},
		{"Simple B (Below limit)", "512B", 0, true},
		{"Decimal KB (Below limit)", "1.5KB", 0, true},
		{"Over max size", "6MB", 0, true},
		{"Invalid format", "10 M B", 0, true},
		{"Invalid unit", "10TB", 0, true},
		{"No number", "MB", 0, true},
		{"Unit-less number (Below limit)", "1024", 0, true},
		{"Zero bytes", "0B", 0, true},
		{"Below min bytes (1B)", "1B", 0, true},
		{"Below min bytes (3KB)", "3KB", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBytes, err := valve.ParseByteSize(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantBytes, gotBytes)
			}
		})
	}
}