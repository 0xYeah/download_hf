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
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	if err := f.Truncate(totalSize); err != nil {
		return fmt.Errorf("预分配文件失败: %w", err)
	}

	var downloaded int64
	ctx, cancel := context.WithCancel(context.Background())

	var printerWg sync.WaitGroup
	printerWg.Add(1)
	go func() {
		defer printerWg.Done()
		runProgressPrinter(ctx, totalSize, &downloaded)
	}()

	segSize := totalSize / defaultSegments
	errCh := make(chan error, defaultSegments)
	var wg sync.WaitGroup

	for i := 0; i < defaultSegments; i++ {
		start := int64(i) * segSize
		end := start + segSize - 1
		if i == defaultSegments-1 {
			end = totalSize - 1
		}
		wg.Add(1)
		go func(start, end int64) {
			defer wg.Done()
			if err := downloadSegment(url, f, start, end, &downloaded); err != nil {
				errCh <- fmt.Errorf("分段 %d-%d: %w", start, end, err)
			}
		}(start, end)
	}

	wg.Wait()
	cancel()
	printerWg.Wait()
	close(errCh)

	for e := range errCh {
		if e != nil {
			f.Close()
			os.Remove(dest)
			return e
		}
	}
	return nil
}

// downloadSegment retries up to segmentRetries times on failure.
// The atomic downloaded counter is only incremented on success to keep progress accurate.
func downloadSegment(url string, f *os.File, start, end int64, downloaded *int64) error {
	var lastErr error
	for attempt := 0; attempt < segmentRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		written, err := tryDownloadSegment(url, f, start, end)
		if err == nil {
			atomic.AddInt64(downloaded, written)
			return nil
		}
		lastErr = err
	}
	return lastErr
}

func tryDownloadSegment(url string, f *os.File, start, end int64) (written int64, err error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
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
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return written, fmt.Errorf("读取失败: %w", readErr)
		}
	}
	return written, nil
}
