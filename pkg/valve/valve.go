package valve

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os/exec"
	"runtime"
	"strings"
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
	Block      Strategy = "block"       // Block when buffer is full
	DropNewest Strategy = "drop-newest" // Drop the newest item when buffer is full
)

const (
	defaultChunkSize            = 1024
	adaptiveBatchTargetInterval = 20.0 // Target ~50ms (1/20th of a second) for adaptive batching
	jitterPercentageDivisor     = 100.0
)

// Options holds the configuration for a new Valve.
type Options struct {
	Rate           float64
	Burst          int
	Jitter         int
	ShowProgress   bool
	MaxBufferSize  int
	Strategy       Strategy
	IsBytes        bool
	ExecCmd        string
	Reader         io.Reader
	Writer         io.Writer
	ProgressWriter io.Writer
}

// Valve controls the rate of data flow.
type Valve struct {
	opts    Options
	limiter *rate.Limiter
	jitter  time.Duration

	buffer chan []byte
	sem    chan struct{} // Semaphore to control access to the buffer's capacity

	// Internal state for progress calculation
	itemsProcessed int64
	bytesProcessed int64
	startTime      time.Time
	rng            *rand.Rand // Random number generator for jitter

	ctx    context.Context
	cancel context.CancelFunc
	errCh  chan error
}

// New creates a new Valve.
func New(parentCtx context.Context, opts Options) (*Valve, error) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)

	var limiterCapacity int
	if opts.IsBytes {
		adaptiveBatchSize := int(opts.Rate / adaptiveBatchTargetInterval)
		if adaptiveBatchSize < defaultChunkSize {
			adaptiveBatchSize = defaultChunkSize
		}
		const assumedChunkSize = defaultChunkSize
		limiterCapacity = adaptiveBatchSize + assumedChunkSize
	} else {
		limiterCapacity = opts.Burst
	}

	limiter := rate.NewLimiter(rate.Limit(opts.Rate), limiterCapacity)

	// When a dropping strategy is used, or if the user wants no initial burst,
	// we must consume the initial tokens from the bucket.
	if opts.Strategy == DropNewest || opts.Burst <= 1 {
		if err := limiter.WaitN(ctx, limiterCapacity); err != nil {
			cancel()
			return nil, err
		}
	}

	// The semaphore is filled with tokens representing the buffer capacity.
	sem := make(chan struct{}, opts.MaxBufferSize)
	for i := 0; i < opts.MaxBufferSize; i++ {
		sem <- struct{}{}
	}

	v := &Valve{
		opts:      opts,
		limiter:   limiter,
		jitter:    time.Duration(0),
		buffer:    make(chan []byte, opts.MaxBufferSize),
		sem:       sem,
		startTime: time.Now(),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		ctx:       ctx,
		cancel:    cancel,
		errCh:     make(chan error, 2), // Increased buffer to avoid blocking
	}
	if opts.Jitter > 0 && opts.Rate > 0 {
		delayPerUnit := float64(time.Second) / opts.Rate
		v.jitter = time.Duration(delayPerUnit * (float64(opts.Jitter) / jitterPercentageDivisor))
	}

	return v, nil
}

// Run starts the valve's operation. It's a blocking call.
func (v *Valve) Run() error {
	if v.opts.ExecCmd != "" {
		return v.runExec()
	}
	return v.runPipe()
}

// runPipe runs the standard stdin -> buffer -> stdout pipeline.
func (v *Valve) runPipe() error {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		v.read()
	}()

	go func() {
		defer wg.Done()
		v.write()
	}()

	wg.Wait()
	close(v.errCh)

	// Return the first error encountered, if any.
	for err := range v.errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// runExec runs the command execution pipeline.
func (v *Valve) runExec() error {
	scanner := bufio.NewScanner(v.opts.Reader)
	for scanner.Scan() {
		if err := v.limiter.Wait(v.ctx); err != nil {
			return err
		}

		line := scanner.Text()
		cmdStr := strings.ReplaceAll(v.opts.ExecCmd, "{}", line)

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("powershell.exe", "-Command", cmdStr)
		} else {
			cmd = exec.Command("sh", "-c", cmdStr)
		}

		cmd.Stdout = v.opts.Writer
		cmd.Stderr = v.opts.ProgressWriter // Often stderr is used for progress/errors

		if err := cmd.Run(); err != nil {
			// Log the error but don't stop processing other lines
			fmt.Fprintf(v.opts.ProgressWriter, "command failed for line '%s': %v\n", line, err)
		}
		v.updateProgress(len(line) + 1)
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

// read fills the buffer from the reader.
func (v *Valve) read() {
	defer close(v.buffer)

	if v.opts.IsBytes {
		for {
			select {
			case <-v.ctx.Done():
				return
			default:
			}
			buf := bytePool.Get().([]byte)
			n, err := v.opts.Reader.Read(buf)
			if n > 0 {
				dataCopy := make([]byte, n)
				copy(dataCopy, buf[:n])
				v.sendToBuffer(dataCopy)
			}
			bytePool.Put(buf)
			if err != nil {
				if err != io.EOF {
					v.errCh <- err
				}
				return
			}
		}
	} else {
		scanner := bufio.NewScanner(v.opts.Reader)
		for scanner.Scan() {
			select {
			case <-v.ctx.Done():
				return
			default:
			}
			data := scanner.Bytes()
			dataCopy := make([]byte, len(data)+1) // +1 for newline
			copy(dataCopy, data)
			dataCopy[len(data)] = '\n'
			v.sendToBuffer(dataCopy)
		}
		if err := scanner.Err(); err != nil {
			v.errCh <- err
		}
	}
}

// sendToBuffer acquires a semaphore token before adding data to the buffer.
func (v *Valve) sendToBuffer(data []byte) {
	switch v.opts.Strategy {
	case Block:
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

// write drains the buffer and writes to the final writer.
func (v *Valve) write() {
	for {
		select {
		case <-v.ctx.Done():
			return
		case data, ok := <-v.buffer:
			if !ok {
				return // Channel closed
			}
			n := 1
			if v.opts.IsBytes {
				n = len(data)
			}
			if err := v.limiter.WaitN(v.ctx, n); err != nil {
				v.errCh <- err
				return
			}

			_, err := v.opts.Writer.Write(data)
			if err != nil {
				v.errCh <- err
				return
			}

			v.sem <- struct{}{}

			v.updateProgress(len(data))
		}
	}
}

func (v *Valve) updateProgress(bytesWritten int) {
	v.itemsProcessed++
	v.bytesProcessed += int64(bytesWritten)
	if v.opts.ShowProgress && v.opts.ProgressWriter != nil {
		elapsed := time.Since(v.startTime).Seconds()
		if elapsed > 0 {
			var progressInfo string
			bytesPerSecond := float64(v.bytesProcessed) / elapsed
			if v.opts.IsBytes {
				progressInfo = fmt.Sprintf("\rTransferred: %s, Rate: %s/s", formatBytes(float64(v.bytesProcessed)), formatBytes(bytesPerSecond))
			} else {
				linesPerSecond := float64(v.itemsProcessed) / elapsed
				progressInfo = fmt.Sprintf("\rProcessed Lines: %d, Rate: %.2f lines/s, Data Rate: %s/s", v.itemsProcessed, linesPerSecond, formatBytes(bytesPerSecond))
			}
			v.opts.ProgressWriter.Write([]byte(progressInfo))
		}
	}
}

// Close cancels the internal context, signaling all goroutines to shut down.
func (v *Valve) Close() {
	v.cancel()
}
