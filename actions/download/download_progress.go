// Package download actions/download/download_progress.go
package download

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/cheggaaa/pb/v3"
)

const (
	idleCheckInterval   = 30 * time.Second
	idleTimeout         = 2 * time.Minute
	logProgressInterval = 5 * time.Second
)

// stdoutMode returns "terminal", "logfile", or "plain" (pipe/unknown).
func stdoutMode() string {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return "plain"
	}
	if fi.Mode()&os.ModeCharDevice != 0 {
		return "terminal"
	}
	if fi.Mode().IsRegular() {
		return "logfile"
	}
	return "plain"
}

func runProgressPrinter(ctx context.Context, totalSize int64, downloaded *int64) {
	switch stdoutMode() {
	case "terminal":
		bar := pb.Full.Start64(totalSize)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		defer bar.Finish()
		for {
			select {
			case <-ctx.Done():
				bar.SetCurrent(atomic.LoadInt64(downloaded))
				return
			case <-ticker.C:
				bar.SetCurrent(atomic.LoadInt64(downloaded))
			}
		}
	case "logfile":
		// Daemon mode: overwrite the same line via \r so `tail -f` shows in-place updates.
		// Final line commits with \n, showing elapsed time.
		ticker := time.NewTicker(logProgressInterval)
		defer ticker.Stop()
		prev := atomic.LoadInt64(downloaded)
		start := time.Now()
		for {
			select {
			case <-ctx.Done():
				d := atomic.LoadInt64(downloaded)
				elapsed := time.Since(start).Round(time.Second)
				if totalSize > 0 && d >= totalSize {
					fmt.Printf("\r⏳ %.1f MB / %.1f MB (100%%) 用时: %s\n",
						float64(totalSize)/1024/1024, float64(totalSize)/1024/1024, elapsed)
				} else {
					fmt.Printf("\r⏳ %.1f MB / %.1f MB (%d%%) 中断 用时: %s\n",
						float64(d)/1024/1024, float64(totalSize)/1024/1024, pctOf(d, totalSize), elapsed)
				}
				return
			case <-ticker.C:
				d := atomic.LoadInt64(downloaded)
				speed := float64(d-prev) / logProgressInterval.Seconds() / 1024 / 1024
				prev = d
				fmt.Printf("\r⏳ %.1f MB / %.1f MB (%d%%) %.2f MB/s   ",
					float64(d)/1024/1024, float64(totalSize)/1024/1024, pctOf(d, totalSize), speed)
			}
		}
	default: // pipe or unknown — carriage-return in-place updates
		const plainInterval = 200 * time.Millisecond
		ticker := time.NewTicker(plainInterval)
		defer ticker.Stop()
		prev := atomic.LoadInt64(downloaded)
		for {
			select {
			case <-ctx.Done():
				d := atomic.LoadInt64(downloaded)
				fmt.Printf("\r⏳ %.1f MB / %.1f MB (%d%%)             \n",
					float64(d)/1024/1024, float64(totalSize)/1024/1024, pctOf(d, totalSize))
				return
			case <-ticker.C:
				d := atomic.LoadInt64(downloaded)
				speed := float64(d-prev) / plainInterval.Seconds() / 1024 / 1024
				prev = d
				fmt.Printf("\r⏳ %.1f MB / %.1f MB (%d%%) %.2f MB/s   ",
					float64(d)/1024/1024, float64(totalSize)/1024/1024, pctOf(d, totalSize), speed)
			}
		}
	}
}

// runWatchdog cancels ctx if no download progress is observed for idleTimeout.
func runWatchdog(ctx context.Context, cancel context.CancelFunc, downloaded *int64) {
	ticker := time.NewTicker(idleCheckInterval)
	defer ticker.Stop()

	last := atomic.LoadInt64(downloaded)
	idle := time.Duration(0)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := atomic.LoadInt64(downloaded)
			if current == last {
				idle += idleCheckInterval
				if idle >= idleTimeout {
					cancel()
					return
				}
			} else {
				idle = 0
				last = current
			}
		}
	}
}

func pctOf(d, total int64) int64 {
	if total <= 0 {
		return 0
	}
	return d * 100 / total
}

