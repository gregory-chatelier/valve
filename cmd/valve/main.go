package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gregory-chatelier/valve/pkg/valve"
	"github.com/spf13/pflag"
)

var (
	rateStr     string
	burst       int
	jitter      int
	progress    bool
	maxBuffer   string
	onFull      string
	execCmd     string
	showVersion bool
	version     = "dev" // Default version, overridden by ldflags
)

var exitFunc = os.Exit

func init() {
	pflag.StringVarP(&rateStr, "rate", "r", "", "Flow rate (e.g., 10/s, 200/mn, 5MB/s, 2GB/h)")
	pflag.IntVarP(&burst, "burst", "b", 1, "Burst size in lines (for line-based transfers)")
	pflag.IntVarP(&jitter, "jitter", "j", 0, "Add \u00b1% random timing variation")
	pflag.BoolVarP(&progress, "progress", "p", false, "Show progress bar and live rate")
	pflag.StringVar(&maxBuffer, "max-buffer", "128KB", "Maximum internal buffer size (e.g., 64KB, 128KB, 512KB)")
	pflag.StringVar(&onFull, "on-full", "block", "On buffer full: block, drop-newest")
	pflag.StringVar(&execCmd, "exec", "", "Execute a command for each line of input (placeholder: {})")
	pflag.BoolVar(&showVersion, "version", false, "Show version info")
}

func main() {
	pflag.Parse()

	if showVersion {
		fmt.Printf("valve version %s\n", version)
		exitFunc(0)
	}

	if rateStr == "" {
		fmt.Println("Error: --rate is required")
		pflag.Usage()
		exitFunc(1)
	}

	rate, isBytes, err := valve.ParseRate(rateStr)
	if err != nil {
		fmt.Printf("Error parsing rate: %v\n", err)
		exitFunc(1)
	}

	if execCmd != "" && isBytes {
		fmt.Println("Error: --exec flag can only be used with line-based rates (e.g., 10/s), not byte-based rates (e.g., 5MB/s)")
		exitFunc(1)
	}

	var bufferSize int
	bufferSize, err = valve.ParseByteSize(maxBuffer)
	if err != nil {
		fmt.Printf("Error parsing buffer size: %v\n", err)
		exitFunc(1)
	}

	strategy, err := valve.ParseStrategy(onFull)
	if err != nil {
		fmt.Printf("Error parsing strategy: %v\n", err)
		exitFunc(1)
	}

	opts := valve.Options{
		Rate:           rate,
		Burst:          burst,
		Jitter:         jitter,
		ShowProgress:   progress,
		MaxBufferSize:  bufferSize,
		Strategy:       strategy,
		IsBytes:        isBytes,
		ExecCmd:        execCmd,
		Reader:         os.Stdin,
		Writer:         os.Stdout,
		ProgressWriter: os.Stderr,
	}

	v, err := valve.New(context.Background(), opts)
	if err != nil {
		fmt.Printf("Error creating valve: %v\n", err)
		exitFunc(1)
	}
	defer v.Close()

	if err := v.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}
}
