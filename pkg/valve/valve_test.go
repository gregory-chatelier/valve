package valve_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gregory-chatelier/valve/pkg/valve"
)

func TestValve_BufferingStrategies(t *testing.T) {
	tests := []struct {
		name       string
		strategy   valve.Strategy
		maxBuffer  int
		input      []string
		wantOutput string
	}{
		{
			name:       "Block strategy - buffer not full",
			strategy:   valve.Block,
			maxBuffer:  3,
			input:      []string{"a", "b", "c"},
			wantOutput: "a\nb\nc\n",
		},
		{
			name:       "Block strategy - buffer full",
			strategy:   valve.Block,
			maxBuffer:  2,
			input:      []string{"a", "b", "c"}, // 'c' should block until 'a' is read
			wantOutput: "a\nb\nc\n",
		},
		{
			name:       "DropOldest strategy",
			strategy:   valve.DropOldest,
			maxBuffer:  2,
			input:      []string{"a", "b", "c"}, // 'a' should be dropped, buffer contains [b, c]
			wantOutput: "b\nc\n",
		},
		{
			name:       "DropNewest strategy",
			strategy:   valve.DropNewest,
			maxBuffer:  2,
			input:      []string{"a", "b", "c"}, // 'c' should be dropped, buffer contains [a, b]
			wantOutput: "a\nb\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputWriter := &bytes.Buffer{}
			v := valve.New(1000, 1, 0, false, tt.maxBuffer, tt.strategy, false, nil, outputWriter)

			var wg sync.WaitGroup
			if tt.strategy == valve.Block {
				wg.Add(1)
				go func() {
					defer wg.Done()
					v.Write()
				}()
			}

			// Simulate input by directly pushing to the buffer
			for _, item := range tt.input {
				dataCopy := make([]byte, len(item))
				copy(dataCopy, []byte(item))
				// This is where the buffering strategy logic should be applied
				switch v.OnFull {
				case valve.Block:
					v.Buffer() <- dataCopy
				case valve.DropOldest:
					select {
					case v.Buffer() <- dataCopy:
						// Successfully wrote to buffer
					default:
						// Buffer is full, drop oldest by reading one item and then writing new one
						<-v.Buffer()
						v.Buffer() <- dataCopy
					}
				case valve.DropNewest:
					select {
					case v.Buffer() <- dataCopy:
						// Successfully wrote to buffer
					default:
						// Buffer is full, drop newest (i.e., this dataCopy)
						// Do nothing, effectively dropping dataCopy
					}
				}
			}
			close(v.Buffer()) // Signal end of input to the writer

			// If writer was not started in a goroutine, run it now
			if tt.strategy != valve.Block {
				v.Write()
			} else {
				wg.Wait() // Wait for the writer goroutine to finish
			}

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

	v := valve.New(rateVal, burst, 0, true, maxBuffer, valve.Block, false, inputReader, outputWriter)
	v.SetProgressWriter(progressWriter) // Assuming a SetProgressWriter method exists

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		v.Write()
	}()

	wg.Add(1)
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
			v := valve.New(rate, 16, 0, false, 1024*1024, valve.Block, isBytes, reader, io.Discard)

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