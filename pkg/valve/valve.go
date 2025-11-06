package valve

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"time"

	"golang.org/x/time/rate"
)

// Strategy defines the behavior when the buffer is full.
type Strategy string

const (
	// Block strategy pauses reads until space frees up in the buffer.
	Block Strategy = "block"
	// DropOldest strategy discards the oldest data in the buffer to make space.
	DropOldest Strategy = "drop-oldest"
	// DropNewest strategy discards the newest incoming data when the buffer is full.
	DropNewest Strategy = "drop-newest"
)

// Valve controls the rate of data flow.
type Valve struct {
	limiter        *rate.Limiter
	burst          int
	jitter         time.Duration
	progress       bool
	maxBuffer      int
	OnFull         Strategy
	isBytes        bool
	rate           float64
	reader         io.Reader
	writer         io.Writer
	progressWriter io.Writer

	buffer chan []byte
	sem    chan struct{} // Semaphore to control access to the buffer's capacity

	// Internal state for progress calculation
	itemsProcessed int64
	bytesProcessed int64
	startTime      time.Time
}

// SetProgressWriter sets the writer for progress output.
func (v *Valve) SetProgressWriter(w io.Writer) {
	v.progressWriter = w
}

// New creates a new Valve and starts its internal goroutines.
func New(rateVal float64, burst int, jitterPercent int, progress bool, maxBuffer int, onFull Strategy, isBytes bool, reader io.Reader, writer io.Writer) *Valve {
	limiterBurst := burst
	if isBytes {
		adaptiveBatchSize := int(rateVal / 20.0)
		if adaptiveBatchSize < 1024 {
			adaptiveBatchSize = 1024
		}
		const assumedChunkSize = 1024
		limiterBurst = adaptiveBatchSize + assumedChunkSize
	}

	limiter := rate.NewLimiter(rate.Limit(rateVal), limiterBurst)

	// When a dropping strategy is used, or if the user wants no initial burst,
	// we must consume the initial tokens from the bucket.
	if onFull == DropOldest || onFull == DropNewest || burst <= 1 {
		limiter.WaitN(context.Background(), limiterBurst)
	}

	// The semaphore is filled with tokens representing the buffer capacity.
	sem := make(chan struct{}, maxBuffer)
	for i := 0; i < maxBuffer; i++ {
		sem <- struct{}{}
	}

	v := &Valve{
		limiter:        limiter,
		burst:          burst,
		jitter:         time.Duration(0),
		progress:       progress,
		maxBuffer:      maxBuffer,
		OnFull:         onFull,
		isBytes:        isBytes,
		rate:           rateVal,
		reader:         reader,
		writer:         writer,
		buffer:         make(chan []byte, maxBuffer),
		sem:            sem,
		startTime:      time.Now(),
	}

	if jitterPercent > 0 && v.rate > 0 {
		delayPerUnit := float64(time.Second) / v.rate
		v.jitter = time.Duration(delayPerUnit * (float64(jitterPercent) / 100.0))
	}

	return v
}

// Read fills the buffer from the reader.
func (v *Valve) Read() {
	defer close(v.buffer)

	if v.isBytes {
		buf := make([]byte, 1024)
		for {
			n, err := v.reader.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				return
			}
			dataCopy := make([]byte, n)
			copy(dataCopy, buf[:n])
			v.sendToBuffer(dataCopy)
		}
	} else {
		scanner := bufio.NewScanner(v.reader)
		for scanner.Scan() {
			dataCopy := make([]byte, len(scanner.Bytes()))
			copy(dataCopy, scanner.Bytes())
			v.sendToBuffer(dataCopy)
		}
	}
}

// sendToBuffer acquires a semaphore token before adding data to the buffer.
func (v *Valve) sendToBuffer(data []byte) {
	switch v.OnFull {
	case Block:
		<-v.sem
		v.buffer <- data
	case DropOldest:
		// For DropOldest, the producer must wait for a slot to become free.
		// The writer is responsible for dropping the oldest items.
		<-v.sem
		v.buffer <- data
	case DropNewest:
		select {
		case <-v.sem:
			v.buffer <- data
		default:
			// Drop
		}
	}
}

// Write drains the buffer and writes to the final writer.
func (v *Valve) Write() {
	for data := range v.buffer {
		// Rate limit first.
		n := 1
		if v.isBytes {
			n = len(data)
		}
		err := v.limiter.WaitN(context.Background(), n)
		if err != nil {
			return
		}

		if v.jitter > 0 {
			jitterAmount := time.Duration(rand.Int63n(int64(v.jitter)*2)) - v.jitter
			time.Sleep(jitterAmount)
		}

		_, err = v.writer.Write(data)
		if err != nil {
			return
		}
		if !v.isBytes {
			_, err = v.writer.Write([]byte("\n"))
			if err != nil {
				return
			}
		}

		// Only after the write is complete, release the semaphore token.
		v.sem <- struct{}{}

		// Update progress stats.
		v.itemsProcessed++
		v.bytesProcessed += int64(len(data))
		if v.progress && v.progressWriter != nil {
			elapsed := time.Since(v.startTime).Seconds()
			if elapsed > 0 {
				var progressInfo string
				bytesPerSecond := float64(v.bytesProcessed) / elapsed
				if v.isBytes {
					progressInfo = fmt.Sprintf("\rTransferred: %s, Rate: %s/s", formatBytes(float64(v.bytesProcessed)), formatBytes(bytesPerSecond))
				} else {
					linesPerSecond := float64(v.itemsProcessed) / elapsed
					progressInfo = fmt.Sprintf("\rProcessed Lines: %d, Rate: %.2f lines/s, Data Rate: %s/s", v.itemsProcessed, linesPerSecond, formatBytes(bytesPerSecond))
				}
			v.progressWriter.Write([]byte(progressInfo))
			}
		}
	}
}

// Buffer is a convenience method for direct channel access in tests.
func (v *Valve) Buffer() chan []byte {
	return v.buffer
}
