// Package download actions/download/download_multi.go
package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func downloadMulti(url, dest string, totalSize int64) error {
	state := loadOrInitState(dest, totalSize)

	// Pre-allocate file only when needed (preserve existing data for resume)
	existingSize := int64(0)
	if info, err := os.Stat(dest); err == nil {
		existingSize = info.Size()
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	if existingSize != totalSize {
		if err := f.Truncate(totalSize); err != nil {
			f.Close()
			os.Remove(dest)
			return fmt.Errorf("预分配文件失败: %w", err)
		}
	}

	downloaded := state.alreadyDownloaded()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var bgWg sync.WaitGroup
	bgWg.Add(2)
	go func() { defer bgWg.Done(); runProgressPrinter(ctx, totalSize, &downloaded) }()
	go func() { defer bgWg.Done(); runWatchdog(ctx, cancel, &downloaded) }()

	errCh := make(chan error, defaultSegments)
	var wg sync.WaitGroup

	for i, seg := range state.Segments {
		if seg.Done {
			continue
		}
		wg.Add(1)
		go func(idx int, seg segmentState) {
			defer wg.Done()
			if err := downloadSegment(ctx, url, f, seg.Start, seg.End, &downloaded); err != nil {
				cancel() // abort all other segments immediately
				errCh <- fmt.Errorf("分段 %d-%d: %w", seg.Start, seg.End, err)
				return
			}
			state.markDone(idx, dest) // persist progress for resume
		}(i, seg)
	}

	wg.Wait()
	cancel()
	bgWg.Wait()
	close(errCh)

	for e := range errCh {
		if e != nil {
			return e // state file kept on disk for next resume
		}
	}

	clearState(dest)
	return nil
}

// downloadSegment retries up to segmentRetries times, rolling back the progress
// counter before each retry so the display stays accurate.
func downloadSegment(ctx context.Context, url string, f *os.File, start, end int64, downloaded *int64) error {
	var lastErr error
	for attempt := 0; attempt < segmentRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		written, err := tryDownloadSegment(ctx, url, f, start, end, downloaded)
		if err == nil {
			return nil
		}
		atomic.AddInt64(downloaded, -written) // roll back partial count before retry
		lastErr = err
	}
	return lastErr
}

func tryDownloadSegment(ctx context.Context, url string, f *os.File, start, end int64, downloaded *int64) (written int64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("服务器不支持分段，状态码：%d", resp.StatusCode)
	}

	buf := make([]byte, bufSize)
	offset := start
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.WriteAt(buf[:n], offset); writeErr != nil {
				return written, fmt.Errorf("写入失败: %w", writeErr)
			}
			offset += int64(n)
			written += int64(n)
			atomic.AddInt64(downloaded, int64(n))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return written, readErr
		}
	}
	return written, nil
}
