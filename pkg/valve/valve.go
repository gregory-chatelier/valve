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
	limiter    *rate.Limiter
	burst      int
	jitter     time.Duration
	progress   bool
	maxBuffer  int
	OnFull     Strategy
	isBytes    bool
	rate       float64

	reader io.Reader
	writer io.Writer
	buffer chan []byte
	progressWriter io.Writer

	itemsProcessed int64
	bytesProcessed int64
	startTime      time.Time
	rng            *rand.Rand
}

// SetProgressWriter sets the writer for progress output.
func (v *Valve) SetProgressWriter(w io.Writer) {
	v.progressWriter = w
}

// New creates a new Valve with the given configuration.
func New(rateVal float64, burst int, jitterPercent int, progress bool, maxBuffer int, onFull Strategy, isBytes bool, reader io.Reader, writer io.Writer) *Valve {
	limiter := rate.NewLimiter(rate.Limit(rateVal), burst)

	jitter := time.Duration(0)
	if jitterPercent > 0 {
		// Jitter will be calculated based on the rate later
	}

	return &Valve{
		limiter:   limiter,
		burst:     burst,
		jitter:    jitter,
		progress:  progress,
		maxBuffer: maxBuffer,
		OnFull:    onFull,
		isBytes:   isBytes,
		rate:      rateVal,
		reader:    reader,
		writer:    writer,
		buffer:    make(chan []byte, maxBuffer),
		startTime: time.Now(),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
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
	for data := range v.buffer {
		// Calculate jittered delay
		delay := v.limiter.Reserve().Delay()
		if v.jitter > 0 && delay > 0 {
			randomFactor := 1.0 - (rand.Float64()*2-1)*(float64(v.jitter)/float64(time.Second))
			delay = time.Duration(float64(delay) * randomFactor)
		}

		// Wait for the limiter
		if delay > 0 {
			time.Sleep(delay)
		}

		// Write the data
		_, err := v.writer.Write(data)
		if err != nil {
			// Handle error
			return
		}

		// Write a newline if not in bytes mode
		if !v.isBytes {
			_, err = v.writer.Write([]byte("\n"))
			if err != nil {
				// Handle error
				return
			}
		}

		v.itemsProcessed++
		v.bytesProcessed += int64(len(data))

		// Write progress indicator
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

func (v *Valve) Buffer() chan []byte {
	return v.buffer
}
