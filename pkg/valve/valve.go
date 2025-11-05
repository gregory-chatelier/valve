package valve

import (
	"bufio"
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
	limiter          *rate.Limiter
	burst            int
	jitter           time.Duration
	progress         bool
	maxBuffer        int
	OnFull           Strategy
	isBytes          bool
	rate             float64
	reader           io.Reader
	writer           io.Writer
	buffer           chan []byte
	progressWriter   io.Writer
	itemsProcessed   int64
	bytesProcessed   int64
	startTime        time.Time
	targetBatchBytes int // For adaptive batching
	rng            *rand.Rand
}

// SetProgressWriter sets the writer for progress output.
func (v *Valve) SetProgressWriter(w io.Writer) {
	v.progressWriter = w
}

// New creates a new Valve with the given configuration.
func New(rateVal float64, burst int, jitterPercent int, progress bool, maxBuffer int, onFull Strategy, isBytes bool, reader io.Reader, writer io.Writer) *Valve {
	var targetBatchBytes int
	limiterBurst := burst // Default for line-mode

	if isBytes {
		// For byte-mode, we use adaptive batching. Target ~50ms worth of data
		// per batch for sleep accuracy.
		targetBatchBytes = int(rateVal / 20.0)
		if targetBatchBytes < 1024 {
			targetBatchBytes = 1024
		}

		// The limiter's burst size MUST be at least the batch size. We add a margin
		// of one chunk size because the batching loop may create a batch that is
		// slightly larger than the target.
		const assumedChunkSize = 1024
		limiterBurst = targetBatchBytes + assumedChunkSize
	}

	limiter := rate.NewLimiter(rate.Limit(rateVal), limiterBurst)

	jitter := time.Duration(0)
	if jitterPercent > 0 {
		// Jitter will be calculated based on the rate later
	}

	return &Valve{
		limiter:          limiter,
		burst:            burst,
		jitter:           jitter,
		progress:         progress,
		maxBuffer:        maxBuffer,
		OnFull:           onFull,
		isBytes:          isBytes,
		rate:             rateVal,
		reader:           reader,
		writer:           writer,
		buffer:           make(chan []byte, maxBuffer),
		startTime:        time.Now(),
		targetBatchBytes: targetBatchBytes,
	}
}
func (v *Valve) Read() {
	defer close(v.buffer)

	if v.isBytes {
		// Read by chunk size
		buf := make([]byte, 1024) // 1KB chunk size, can be made configurable
		for {
			n, err := v.reader.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				// Handle error
				return
			}
			data := buf[:n]
			dataCopy := make([]byte, len(data))
			copy(dataCopy, data)
			v.buffer <- dataCopy
		}
	} else {
		// Read by line
		scanner := bufio.NewScanner(v.reader)
		for scanner.Scan() {
			data := scanner.Bytes()
			// Make a copy of the data, as the scanner may reuse the buffer
			dataCopy := make([]byte, len(data))
			copy(dataCopy, data)

			switch v.OnFull {
			case Block:
				v.buffer <- dataCopy
			case DropOldest:
				select {
				case v.buffer <- dataCopy:
				default:
					<-v.buffer
					v.buffer <- dataCopy
				}
			case DropNewest:
				select {
				case v.buffer <- dataCopy:
				default:
				}
			}
		}
	}
}

func (v *Valve) Write() {
	if v.isBytes {
		// Byte-based transfers use adaptive batching for accuracy.
		for {
			firstData, ok := <-v.buffer
			if !ok {
				return // Channel is closed and empty.
			}

			batch := [][]byte{firstData}
			currentBatchBytes := len(firstData)

			// Greedily pull more items from the buffer to fill the batch.
		DrainLoop:
			for {
				if currentBatchBytes >= v.targetBatchBytes {
					break DrainLoop
				}

				select {
				case data, ok := <-v.buffer:
					if !ok {
						break DrainLoop // Channel closed.
					}
					batch = append(batch, data)
					currentBatchBytes += len(data)
				default:
					break DrainLoop // Buffer is empty.
				}
			}

			// Reserve tokens for the entire batch and sleep for the calculated delay.
			if currentBatchBytes > 0 {
				r := v.limiter.ReserveN(time.Now(), currentBatchBytes)
				delay := r.Delay()

				if v.jitter > 0 && delay > 0 {
					randomFactor := 1.0 - (rand.Float64()*2-1)*(float64(v.jitter)/float64(time.Second))
					delay = time.Duration(float64(delay) * randomFactor)
				}

				if delay > 0 {
					time.Sleep(delay)
				}
			}

			// Write the entire batch to the output and update progress stats.
			for _, data := range batch {
				_, err := v.writer.Write(data)
				if err != nil {
					return // Or handle error
				}
			}

			v.itemsProcessed += int64(len(batch))
			v.bytesProcessed += int64(currentBatchBytes)

			// Update progress display once per batch.
			if v.progress && v.progressWriter != nil {
				elapsed := time.Since(v.startTime).Seconds()
				if elapsed > 0 {
					bytesPerSecond := float64(v.bytesProcessed) / elapsed
					progressInfo := fmt.Sprintf("\rTransferred: %s, Rate: %s/s", formatBytes(float64(v.bytesProcessed)), formatBytes(bytesPerSecond))
					v.progressWriter.Write([]byte(progressInfo))
				}
			}
		}
	} else {
		// Line-based transfers process one line at a time for classic token-bucket behavior.
		for data := range v.buffer {
			// Reserve 1 token for this line and sleep.
			r := v.limiter.ReserveN(time.Now(), 1)
			delay := r.Delay()

			if v.jitter > 0 && delay > 0 {
				randomFactor := 1.0 - (rand.Float64()*2-1)*(float64(v.jitter)/float64(time.Second))
				delay = time.Duration(float64(delay) * randomFactor)
			}

			if delay > 0 {
				time.Sleep(delay)
			}

			// Write the single line to the output.
			_, err := v.writer.Write(data)
			if err != nil {
				return // Or handle error
			}
			_, err = v.writer.Write([]byte("\n"))
			if err != nil {
				return // Or handle error
			}

			v.itemsProcessed++
			v.bytesProcessed += int64(len(data))

			// Update progress display.
			if v.progress && v.progressWriter != nil {
				elapsed := time.Since(v.startTime).Seconds()
				if elapsed > 0 {
					bytesPerSecond := float64(v.bytesProcessed) / elapsed
					linesPerSecond := float64(v.itemsProcessed) / elapsed
					progressInfo := fmt.Sprintf("\rProcessed Lines: %d, Rate: %.2f lines/s, Data Rate: %s/s", v.itemsProcessed, linesPerSecond, formatBytes(bytesPerSecond))
					v.progressWriter.Write([]byte(progressInfo))
				}
			}
		}
	}
}
func (v *Valve) Buffer() chan []byte {
	return v.buffer
}
