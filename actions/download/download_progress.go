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
	idleCheckInterval = 30 * time.Second
	idleTimeout       = 2 * time.Minute
)

func runProgressPrinter(ctx context.Context, totalSize int64, downloaded *int64) {
	if isTerminal() {
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
	} else {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				d := atomic.LoadInt64(downloaded)
				fmt.Printf("\r⏳ %.1f MB / %.1f MB (%d%%)   \n",
					float64(d)/1024/1024, float64(totalSize)/1024/1024, pctOf(d, totalSize))
				return
			case <-ticker.C:
				d := atomic.LoadInt64(downloaded)
				fmt.Printf("\r⏳ %.1f MB / %.1f MB (%d%%)   ",
					float64(d)/1024/1024, float64(totalSize)/1024/1024, pctOf(d, totalSize))
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

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
