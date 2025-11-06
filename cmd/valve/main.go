package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/gregory-chatelier/valve/pkg/valve"
	"github.com/spf13/pflag"
)

var (
	rateStr     string
	burst       int
	jitter      int
	progress    bool
	maxBuffer   int
	onFull      string
	showVersion bool
	version     = "dev" // Default version, overridden by ldflags
)

var exitFunc = os.Exit

func init() {
	pflag.StringVarP(&rateStr, "rate", "r", "", "Flow rate (e.g., 10/s, 200/mn, 5MB/s, 2GB/h)")
	pflag.IntVarP(&burst, "burst", "b", 1, "Burst size in lines (for line-based transfers)")
	pflag.IntVarP(&jitter, "jitter", "j", 0, "Add \u00b1% random timing variation")
	pflag.BoolVarP(&progress, "progress", "p", false, "Show progress bar and live rate")
	pflag.IntVar(&maxBuffer, "max-buffer", 1024*1024, "Maximum internal buffer in bytes")
	pflag.StringVar(&onFull, "on-full", "block", "On buffer full: block, drop-oldest, drop-newest")
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

	strategy, err := valve.ParseStrategy(onFull)
	if err != nil {
		fmt.Printf("Error parsing strategy: %v\n", err)
		exitFunc(1)
	}

	v := valve.New(context.Background(), rate, burst, jitter, progress, maxBuffer, strategy, isBytes, os.Stdin, os.Stdout)
	defer v.Close() // Ensure context is cancelled and goroutines are cleaned up

	if progress {
		v.SetProgressWriter(os.Stderr)
	}

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
}
