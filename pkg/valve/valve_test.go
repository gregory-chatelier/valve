package valve_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gregory-chatelier/valve/pkg/valve"
	"github.com/stretchr/testify/require"
)

func TestValve_BufferingStrategies(t *testing.T) {
	tests := []struct {
		name       string
		strategy   valve.Strategy
		maxBuffer  int
		input      []string
		wantOutput string
		rate       float64
	}{
		{
			name:       "Block strategy - buffer not full",
			strategy:   valve.Block,
			maxBuffer:  3,
			input:      []string{"a", "b", "c"},
			wantOutput: "a\nb\nc\n",
			rate:       1000,
		},
		{
			name:       "Block strategy - buffer full",
			strategy:   valve.Block,
			maxBuffer:  1,
			input:      []string{"a", "b", "c"},
			wantOutput: "a\nb\nc\n",
			rate:       1000,
		},
		{
			name:       "DropNewest strategy",
			strategy:   valve.DropNewest,
			maxBuffer:  1,
			input:      []string{"a", "b", "c"},
			wantOutput: "a\n",
			rate:       1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputWriter := &bytes.Buffer{}
			inputReader := strings.NewReader(strings.Join(tt.input, "\n") + "\n")

			opts := valve.Options{
				Rate:          tt.rate,
				Burst:         1,
				MaxBufferSize: tt.maxBuffer,
				Strategy:      tt.strategy,
				Reader:        inputReader,
				Writer:        outputWriter,
			}
			v, err := valve.New(context.Background(), opts)
			require.NoError(t, err)

			err = v.Run()
			require.NoError(t, err)

			require.Equal(t, tt.wantOutput, outputWriter.String())
		})
	}
}

func TestValve_ProgressIndicator(t *testing.T) {
	inputReader := &bytes.Buffer{}
	outputWriter := &bytes.Buffer{}
	progressWriter := &bytes.Buffer{}

	opts := valve.Options{
		Rate:           10.0,
		Burst:          1,
		ShowProgress:   true,
		MaxBufferSize:  10,
		Strategy:       valve.Block,
		Reader:         inputReader,
		Writer:         outputWriter,
		ProgressWriter: progressWriter,
	}
	v, err := valve.New(context.Background(), opts)
	require.NoError(t, err)

	// We need to run the valve in a goroutine to be able to write to its input
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		v.Run()
	}()

	for i := 0; i < 20; i++ {
		fmt.Fprintf(inputReader, "item%d\n", i)
		time.Sleep(10 * time.Millisecond) // Give some time for processing
	}
	// Closing the reader is not feasible here, so we just wait a bit
	time.Sleep(3 * time.Second)
	v.Close() // Manually close the valve
	wg.Wait()

	require.NotEmpty(t, progressWriter.String(), "expected progress output, but got none")
}

type dummyReader struct {
	totalSize int64
	readPos   int64
}

func (r *dummyReader) Read(p []byte) (n int, err error) {
	if r.readPos >= r.totalSize {
		return 0, io.EOF
	}
	remaining := r.totalSize - r.readPos
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	for i := range p {
		p[i] = 0
	}
	n = len(p)
	r.readPos += int64(n)
	return n, nil
}

func TestValve_RateLimiting(t *testing.T) {
	tests := []struct {
		name             string
		rateStr          string
		dataSize         int64
		isBytes          bool
		numLines         int
		expectedDuration time.Duration
		tolerance        float64
	}{
		{"20KB at 40KB/s", "40KB/s", 20 * 1024, true, 0, 500 * time.Millisecond, 0.25},
		{"1MB at 2MB/s", "2MB/s", 1 * 1024 * 1024, true, 0, 500 * time.Millisecond, 0.25},
		{"5MB at 10MB/s", "10MB/s", 5 * 1024 * 1024, true, 0, 500 * time.Millisecond, 0.25},
		{"10MB at 5MB/s", "5MB/s", 10 * 1024 * 1024, true, 0, 2 * time.Second, 0.20},
		{"100 lines at 200 lines/s", "200/s", 0, false, 100, 500 * time.Millisecond, 0.25},
		{"1000 lines at 200 lines/s", "200/s", 0, false, 1000, 5 * time.Second, 0.20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate, isBytes, err := valve.ParseRate(tt.rateStr)
			require.NoError(t, err)

			var reader io.Reader
			if tt.isBytes {
				reader = &dummyReader{totalSize: tt.dataSize}
			} else {
				var sb strings.Builder
				for i := 0; i < tt.numLines; i++ {
					sb.WriteString("this is a test line\n")
				}
				reader = strings.NewReader(sb.String())
			}

			opts := valve.Options{
				Rate:          rate,
				Burst:         1,
				MaxBufferSize: 1024 * 1024,
				Strategy:      valve.Block,
				IsBytes:       isBytes,
				Reader:        reader,
				Writer:        io.Discard,
			}
			v, err := valve.New(context.Background(), opts)
			require.NoError(t, err)

			startTime := time.Now()
			err = v.Run()
			elapsedTime := time.Since(startTime)
			require.NoError(t, err)

			minDuration := time.Duration(float64(tt.expectedDuration) * (1.0 - tt.tolerance))
			maxDuration := time.Duration(float64(tt.expectedDuration) * (1.0 + tt.tolerance))

			require.True(t, elapsedTime >= minDuration && elapsedTime <= maxDuration,
				"elapsed time = %v, want between %v and %v", elapsedTime, minDuration, maxDuration)
		})
	}
}

func TestValve_DropNewest_RaceCondition(t *testing.T) {
	out := &bytes.Buffer{}
	input := "1\n2\n3\n4\n5\n"
	reader := strings.NewReader(input)

	opts := valve.Options{
		Rate:          1,
		Burst:         1,
		MaxBufferSize: 2,
		Strategy:      valve.DropNewest,
		Reader:        reader,
		Writer:        out,
	}
	v, err := valve.New(context.Background(), opts)
	require.NoError(t, err)

	err = v.Run()
	require.NoError(t, err)

	expected := "1\n2\n"
	require.Equal(t, expected, out.String(), "DropNewest failed")
}

type errorReader struct {
	reader io.Reader
	err    error
	count  int
}

func (er *errorReader) Read(p []byte) (n int, err error) {
	if er.count == 0 {
		er.count++
		return er.reader.Read(p)
	}
	return 0, er.err
}
