package valve_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gregory-chatelier/valve/pkg/valve"
	"github.com/stretchr/testify/require"
)

func TestValve_Exec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping exec test on Windows due to shell differences")
	}

	// Create a temporary file to act as a log
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "exec_test.log")

	// The command will append the line and a timestamp to the log file
	// We use `date +%s%N` for nanosecond precision
	command := fmt.Sprintf("echo \"'{}' $(date +%%s%%N)\" >> %s", logFile)

	input := "line1\nline2\nline3"
	rate := 10.0 // 10 lines/sec -> 100ms between tasks
	opts := valve.Options{
		Rate:           rate,
		ExecCmd:        command,
		Reader:         strings.NewReader(input),
		Writer:         &bytes.Buffer{},
		ProgressWriter: &bytes.Buffer{},
	}

	v, err := valve.New(context.Background(), opts)
	require.NoError(t, err)

	err = v.Run()
	require.NoError(t, err)

	// --- Verification ---
	output, err := os.ReadFile(logFile)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	require.Len(t, lines, 3, "Should have executed the command for each of the 3 lines")

	// Check placeholder replacement
	require.Contains(t, lines[0], "'line1'")
	require.Contains(t, lines[1], "'line2'")
	require.Contains(t, lines[2], "'line3'")

	// Check rate limiting
	var timestamps []int64
	for _, line := range lines {
		parts := strings.Fields(line)
		ts, err := time.Parse(time.RFC3339Nano, parts[len(parts)-1])
		require.NoError(t, err)
		timestamps = append(timestamps, ts.UnixNano())
	}

	delta1 := time.Duration(timestamps[1] - timestamps[0])
	delta2 := time.Duration(timestamps[2] - timestamps[1])
	expectedDelta := time.Second / time.Duration(rate)

	// Allow for some scheduler variance
	require.InDelta(t, expectedDelta, delta1, float64(expectedDelta/2), "Time between task 1 and 2 should be close to the rate")
	require.InDelta(t, expectedDelta, delta2, float64(expectedDelta/2), "Time between task 2 and 3 should be close to the rate")
}

func TestValve_Exec_WaitsForCompletion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping exec test on Windows due to shell differences")
	}

	// This command sleeps for 200ms
	command := "sleep 0.2 && echo '{}'"

	input := "line1\nline2"
	rate := 20.0 // Rate is 20/s (50ms), faster than the command's execution time
	opts := valve.Options{
		Rate:           rate,
		ExecCmd:        command,
		Reader:         strings.NewReader(input),
		Writer:         &bytes.Buffer{},
		ProgressWriter: &bytes.Buffer{},
	}

	v, err := valve.New(context.Background(), opts)
	require.NoError(t, err)

	startTime := time.Now()
	err = v.Run()
	elapsed := time.Since(startTime)
	require.NoError(t, err)

	// Each task takes 200ms. The rate limiter would allow the second task
	// to start after 50ms, but it must wait for the first to finish.
	// Total time should be at least 2 * 200ms = 400ms.
	require.GreaterOrEqual(t, elapsed.Milliseconds(), int64(400), "Valve should wait for each command to complete")
}
