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
)

// TestValve_BufferingStrategies is now fully concurrent for all strategies.
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
			rate:       2, // Slow rate to ensure buffer fills
		},
		{
			name:       "DropOldest strategy (behaves as Block)",
			strategy:   valve.DropOldest,
			maxBuffer:  1,
			input:      []string{"a", "b", "c"},
			wantOutput: "a\nb\nc\n", // Expecting Block behavior
			rate:       2,
		},
		{
			name:       "DropNewest strategy",
			strategy:   valve.DropNewest,
			maxBuffer:  1,
			input:      []string{"a", "b", "c"},
			wantOutput: "a\n", // a gets in, b and c are dropped.
			rate:       1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputWriter := &bytes.Buffer{}
			inputReader := strings.NewReader(strings.Join(tt.input, "\n") + "\n")

			v := valve.New(context.Background(), tt.rate, 1, 0, false, tt.maxBuffer, tt.strategy, false, inputReader, outputWriter)

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				v.Read()
			}()

			go func() {
				defer wg.Done()
				v.Write()
			}()

			wg.Wait()

			if gotOutput := outputWriter.String(); gotOutput != tt.wantOutput {
				t.Errorf("got output %q, want %q", gotOutput, tt.wantOutput)
			}
		})
	}
}

func TestValve_ProgressIndicator(t *testing.T) {
	inputReader := &bytes.Buffer{}
	outputWriter := &bytes.Buffer{}
	progressWriter := &bytes.Buffer{}

	rateVal := 10.0 // 10 items per second
	burst := 1
	maxBuffer := 10
	numItems := 20

	v := valve.New(context.Background(), rateVal, burst, 0, true, maxBuffer, valve.Block, false, inputReader, outputWriter)
	v.SetProgressWriter(progressWriter) // Assuming a SetProgressWriter method exists

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		v.Write()
	}()

	go func() {
		defer wg.Done()
		v.Read()
	}()

	for i := 0; i < numItems; i++ {
		inputReader.WriteString(fmt.Sprintf("item%d\n", i))
	}

	wg.Wait()

	// Check if progress output was written
	if progressWriter.Len() == 0 {
		t.Error("expected progress output, but got none")
	}

	// Further checks could involve parsing the progress output and verifying its content and frequency
}

// dummyReader is a simple reader that produces a constant stream of zero bytes.
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
		numLines         int // for line-based tests
		expectedDuration time.Duration
		tolerance        float64 // e.g., 0.25 for 25%
	}{
		// Byte-based tests
		{
			name:             "20KB at 40KB/s",
			rateStr:          "40KB/s",
			dataSize:         20 * 1024,
			isBytes:          true,
			expectedDuration: 500 * time.Millisecond,
			tolerance:        0.25,
		},
		{
			name:             "1MB at 2MB/s",
			rateStr:          "2MB/s",
			dataSize:         1 * 1024 * 1024,
			isBytes:          true,
			expectedDuration: 500 * time.Millisecond,
			tolerance:        0.25,
		},
		{
			name:             "5MB at 10MB/s",
			rateStr:          "10MB/s",
			dataSize:         5 * 1024 * 1024,
			isBytes:          true,
			expectedDuration: 500 * time.Millisecond,
			tolerance:        0.25,
		},
		{
			name:             "10MB at 5MB/s",
			rateStr:          "5MB/s",
			dataSize:         10 * 1024 * 1024,
			isBytes:          true,
			expectedDuration: 2 * time.Second,
			tolerance:        0.20,
		},
		// Line-based tests
		{
			name:             "100 lines at 200 lines/s",
			rateStr:          "200/s",
			numLines:         100,
			isBytes:          false,
			expectedDuration: 500 * time.Millisecond,
			tolerance:        0.25,
		},
		{
			name:             "1000 lines at 200 lines/s",
			rateStr:          "200/s",
			numLines:         1000,
			isBytes:          false,
			expectedDuration: 5 * time.Second,
			tolerance:        0.20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate, isBytes, err := valve.ParseRate(tt.rateStr)
			if err != nil {
				t.Fatalf("ParseRate() error = %v", err)
			}

			var reader io.Reader
			if tt.isBytes {
				reader = &dummyReader{totalSize: tt.dataSize}
			} else {
				// Create a reader with N lines
				var sb strings.Builder
				for i := 0; i < tt.numLines; i++ {
					sb.WriteString("this is a test line\n")
				}
				reader = strings.NewReader(sb.String())
			}

			// Use io.Discard for the writer to avoid write overhead
			v := valve.New(context.Background(), rate, 1, 0, false, 1024*1024, valve.Block, isBytes, reader, io.Discard)

			var wg sync.WaitGroup
			wg.Add(2)

			startTime := time.Now()

			go func() {
				defer wg.Done()
				v.Read()
			}()

			go func() {
				defer wg.Done()
				v.Write()
			}()

			wg.Wait()
			elapsedTime := time.Since(startTime)

			minDuration := time.Duration(float64(tt.expectedDuration) * (1.0 - tt.tolerance))
			maxDuration := time.Duration(float64(tt.expectedDuration) * (1.0 + tt.tolerance))

			if elapsedTime < minDuration || elapsedTime > maxDuration {
				t.Errorf("elapsed time = %v, want between %v and %v", elapsedTime, minDuration, maxDuration)
			}
		})
	}
}

func TestValve_DropNewest_RaceCondition(t *testing.T) {
	output := &bytes.Buffer{}
	input := "1\n2\n3\n4\n5\n"
	reader := strings.NewReader(input)

	// A buffer of size 2, with drop-newest strategy.
	// Rate is 1 item/sec, burst is 1.
	v := valve.New(context.Background(), 1, 1, 0, false, 2, valve.DropNewest, false, reader, output)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); v.Read() }()
	go func() { defer wg.Done(); v.Write() }()
	wg.Wait()

	// With a buffer of 2, only items "1" and "2" should ever be in the buffer
	// and subsequently written. Item "3" should be dropped because the buffer
	// is full when it arrives. The old logic fails because the writer takes
	// item "1" before sleeping, making space for item "3".
	expected := "1\n2\n"
	if got := output.String(); got != expected {
		t.Errorf("DropNewest failed: got %q, want %q", got, expected)
	}
}
