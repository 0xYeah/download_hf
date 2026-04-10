// Package download actions/download/download_state.go
package download

import (
	"encoding/json"
	"os"
	"sync"
)

type segmentState struct {
	Start int64 `json:"s"`
	End   int64 `json:"e"`
	Done  bool  `json:"d"`
}

type downloadState struct {
	TotalSize int64          `json:"size"`
	Segments  []segmentState `json:"segs"`
	mu        sync.Mutex
}

func statePath(dest string) string { return dest + ".hfdownload" }

// loadOrInitState loads a saved state if it matches totalSize, otherwise creates fresh segments.
func loadOrInitState(dest string, totalSize int64) *downloadState {
	if data, err := os.ReadFile(statePath(dest)); err == nil {
		var s downloadState
		if json.Unmarshal(data, &s) == nil && s.TotalSize == totalSize && len(s.Segments) == defaultSegments {
			return &s
		}
	}
	return initState(totalSize)
}

func initState(totalSize int64) *downloadState {
	segSize := totalSize / defaultSegments
	segs := make([]segmentState, defaultSegments)
	for i := range segs {
		start := int64(i) * segSize
		end := start + segSize - 1
		if i == defaultSegments-1 {
			end = totalSize - 1
		}
		segs[i] = segmentState{Start: start, End: end}
	}
	return &downloadState{TotalSize: totalSize, Segments: segs}
}

// alreadyDownloaded returns the total bytes in completed segments.
func (s *downloadState) alreadyDownloaded() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for _, seg := range s.Segments {
		if seg.Done {
			n += seg.End - seg.Start + 1
		}
	}
	return n
}

// markDone marks a segment as completed and persists state to disk.
func (s *downloadState) markDone(idx int, dest string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Segments[idx].Done = true
	data, _ := json.Marshal(s)
	os.WriteFile(statePath(dest), data, 0644)
}

func clearState(dest string) { os.Remove(statePath(dest)) }
