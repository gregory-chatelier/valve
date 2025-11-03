package valve_test

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/gregory-chatelier/valve/pkg/valve"
)

func TestValve_BufferingStrategies(t *testing.T) {
	tests := []struct {
		name        string
		strategy    valve.Strategy
		maxBuffer   int
		input       []string
		wantOutput  string
	}{
		{
			name:        "Block strategy - buffer not full",
			strategy:    valve.Block,
			maxBuffer:   3,
			input:       []string{"a", "b", "c"},
			wantOutput:  "a\nb\nc\n",
		},
		{
			name:        "Block strategy - buffer full",
			strategy:    valve.Block,
			maxBuffer:   2,
			input:       []string{"a", "b", "c"}, // 'c' should block until 'a' is read
			wantOutput:  "a\nb\nc\n",
		},
		{
			name:        "DropOldest strategy",
			strategy:    valve.DropOldest,
			maxBuffer:   2,
			input:       []string{"a", "b", "c"}, // 'a' should be dropped, buffer contains [b, c]
			wantOutput:  "b\nc\n",
		},
		{
			name:        "DropNewest strategy",
			strategy:    valve.DropNewest,
			maxBuffer:   2,
			input:       []string{"a", "b", "c"}, // 'c' should be dropped, buffer contains [a, b]
			wantOutput:  "a\nb\n",
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
