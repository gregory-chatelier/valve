package valve

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var bytePool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 1024) // Default buffer size
	},
}

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

const (
	defaultChunkSize            = 1024
	adaptiveBatchTargetInterval = 20.0 // Target ~50ms (1/20th of a second) for adaptive batching
	jitterPercentageDivisor     = 100.0
)

// Valve controls the rate of data flow.
type Valve struct {
	limiter        *rate.Limiter
	burst          int
	jitter         time.Duration
	progress       bool
	maxBuffer      int
	onFull         Strategy // Unexported
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
	rng            *rand.Rand // Random number generator for jitter

	ctx    context.Context
	cancel context.CancelFunc
}

// SetProgressWriter sets the writer for progress output.
func (v *Valve) SetProgressWriter(w io.Writer) {
	v.progressWriter = w
}

// New creates a new Valve and starts its internal goroutines.
func New(parentCtx context.Context, rateVal float64, burst int, jitterPercent int, progress bool, maxBuffer int, onFull Strategy, isBytes bool, reader io.Reader, writer io.Writer) *Valve {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)

	var limiterCapacity int
	if isBytes {
		adaptiveBatchSize := int(rateVal / adaptiveBatchTargetInterval)
		if adaptiveBatchSize < defaultChunkSize {
			adaptiveBatchSize = defaultChunkSize
		}
		const assumedChunkSize = defaultChunkSize
		limiterCapacity = adaptiveBatchSize + assumedChunkSize
	} else {
		limiterCapacity = burst
	}

	limiter := rate.NewLimiter(rate.Limit(rateVal), limiterCapacity)

	// When a dropping strategy is used, or if the user wants no initial burst,
	// we must consume the initial tokens from the bucket.
	if onFull == DropOldest || onFull == DropNewest || burst <= 1 {
		limiter.WaitN(ctx, limiterCapacity)
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
		onFull:         onFull,
		isBytes:        isBytes,
		rate:           rateVal,
		reader:         reader,
		writer:         writer,
		buffer:         make(chan []byte, maxBuffer),
		sem:            sem,
		startTime:      time.Now(),
		rng:            rand.New(rand.NewSource(time.Now().UnixNano())),
		ctx:            ctx,
		cancel:         cancel,
	}
	if jitterPercent > 0 && v.rate > 0 {
		delayPerUnit := float64(time.Second) / v.rate
		v.jitter = time.Duration(delayPerUnit * (float64(jitterPercent) / jitterPercentageDivisor))
	}

	return v
}

// Read fills the buffer from the reader.
func (v *Valve) Read() {
	defer close(v.buffer)

	if v.isBytes {
		for {
			select {
			case <-v.ctx.Done():
				return
			default:
				// Continue reading
			}
			buf := bytePool.Get().([]byte)
			n, err := v.reader.Read(buf)
			if err != nil {
				if err == io.EOF {
					bytePool.Put(buf)
					break
				}
				bytePool.Put(buf)
				return
			}
			dataCopy := make([]byte, n)
			copy(dataCopy, buf[:n])
			v.sendToBuffer(dataCopy)
			bytePool.Put(buf)
		}
	} else {
		scanner := bufio.NewScanner(v.reader)
		for scanner.Scan() {
			select {
			case <-v.ctx.Done():
				return
			default:
				// Continue reading
			}
			data := scanner.Bytes()
			dataCopy := make([]byte, len(data)+1) // +1 for newline
			copy(dataCopy, data)
			dataCopy[len(data)] = '\n'
			v.sendToBuffer(dataCopy)
		}
	}
}

// sendToBuffer acquires a semaphore token before adding data to the buffer.
func (v *Valve) sendToBuffer(data []byte) {
	switch v.onFull {
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
	for {
		select {
		case <-v.ctx.Done():
			return
		case data, ok := <-v.buffer:
			if !ok {
				return // Channel closed
			}
			// Rate limit first.
			n := 1
			if v.isBytes {
				n = len(data)
			}
			reservation := v.limiter.ReserveN(time.Now(), n)
			delay := reservation.Delay()

			if v.jitter > 0 {
				randomJitter := time.Duration(v.rng.Int63n(int64(v.jitter*2))) - v.jitter // Random value between -v.jitter and +v.jitter
				delay += randomJitter
				if delay < 0 {
					delay = 0
				}
			}

			if delay > 0 {
				time.Sleep(delay)
			}

			// Now write.
			_, err := v.writer.Write(data)
			if err != nil {
				return
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
}// Buffer is a convenience method for direct channel access in tests.
func (v *Valve) Buffer() chan []byte {
	return v.buffer
}

// Close cancels the internal context, signaling all goroutines to shut down.
func (v *Valve) Close() {
	v.cancel()
}
