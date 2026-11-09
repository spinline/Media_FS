package downloader

import (
	"fmt"
	"sync"
)

// WindowState is the fetch state of a cached window.
type WindowState string

const (
	WindowFetching WindowState = "fetching"
	WindowReady    WindowState = "ready"
	WindowError    WindowState = "error"
)

// WindowStatus is a JSON-serialisable snapshot of one window (for monitoring).
type WindowStatus struct {
	Start         int64       `json:"start"`
	End           int64       `json:"end"`
	State         WindowState `json:"state"`
	Error         string      `json:"error,omitempty"`
	LastAccessAgo int64       `json:"last_access_ago_sec"`
	ExpiresIn     int64       `json:"expires_in_sec"`
}

// FileStatus is a JSON-serialisable snapshot of one open file (for monitoring).
type FileStatus struct {
	FH         uint64         `json:"fh"`
	Path       string         `json:"path"`
	FileSize   int64          `json:"file_size"`
	WindowSize int64          `json:"window_size"`
	Windows    []WindowStatus `json:"windows"`
}

// ReadAheadReader coordinates prefetch triggers for one open file.
// It does NOT own the window data — that lives in WindowCache and survives file close.
type ReadAheadReader struct {
	url        string
	headers    map[string]string
	fileSize   int64
	dl         *Downloader
	windowSize int64
	maxActive  int // max prefetch windows to track per reader

	mu     sync.Mutex
	active []*CachedWindow // windows currently tracked by this reader
}

// NewReadAheadReader creates a reader. Window fetching is performed on-demand
// during ReadAt, avoiding heavy memory consumption when files are only probed
// by scanners, thumbnailers, or file managers.
func NewReadAheadReader(url string, headers map[string]string, fileSize int64, dl *Downloader, windowSizeMB, maxActive int) *ReadAheadReader {
	if windowSizeMB <= 0 {
		windowSizeMB = 16
	}
	if maxActive <= 0 {
		maxActive = 2
	}
	return &ReadAheadReader{
		url:        url,
		headers:    headers,
		fileSize:   fileSize,
		dl:         dl,
		windowSize: int64(windowSizeMB) * 1024 * 1024,
		maxActive:  maxActive,
	}
}

// WindowSize returns the configured window size in bytes.
func (r *ReadAheadReader) WindowSize() int64 { return r.windowSize }

// ReadAt fills buf starting at offset.
// Returns bytes written (may be less than len(buf) at a window boundary).
func (r *ReadAheadReader) ReadAt(buf []byte, offset int64) (int, error) {
	if offset >= r.fileSize {
		return 0, nil
	}
	length := int64(len(buf))
	if offset+length > r.fileSize {
		length = r.fileSize - offset
	}

	alignedStart := (offset / r.windowSize) * r.windowSize

	r.mu.Lock()
	w := r.findActiveLocked(alignedStart)
	if w == nil {
		w = r.dl.WindowCache.GetOrCreate(r.url, r.headers, alignedStart, r.windowSize, r.fileSize)
		r.addActiveLocked(w)
	}
	r.mu.Unlock()

	<-w.done // wait for fetch (data/err written before close)

	if w.err != nil {
		// Fall back to direct fetch on cache miss/error
		data, err := r.dl.fetchChunk(r.url, offset, offset+length-1, r.headers)
		if err != nil {
			return 0, err
		}
		// Reseed next window
		r.mu.Lock()
		r.ensureWindowLocked(alignedStart + r.windowSize)
		r.mu.Unlock()
		return copy(buf[:length], data), nil
	}

	winOffset := offset - w.Start
	avail := int64(len(w.data)) - winOffset
	if avail <= 0 {
		return 0, fmt.Errorf("readahead: no data at offset %d", offset)
	}
	if avail < length {
		length = avail
	}
	n := copy(buf[:length], w.data[winOffset:])

	// At 50% of window: prefetch up to maxActive upcoming windows to keep playback seamless.
	if offset >= w.Start+(w.Size/2) {
		r.mu.Lock()
		for step := int64(1); step <= int64(r.maxActive); step++ {
			nextWin := alignedStart + step*r.windowSize
			if nextWin < r.fileSize {
				r.ensureWindowLocked(nextWin)
			}
		}
		r.mu.Unlock()
	}
	return n, nil
}

func (r *ReadAheadReader) findActiveLocked(alignedStart int64) *CachedWindow {
	for _, w := range r.active {
		if w.Start == alignedStart {
			return w
		}
	}
	return nil
}

func (r *ReadAheadReader) addActiveLocked(w *CachedWindow) {
	// Drop oldest from active list when at capacity (data stays in WindowCache)
	for len(r.active) >= r.maxActive {
		r.active = r.active[1:]
	}
	r.active = append(r.active, w)
}

func (r *ReadAheadReader) ensureWindowLocked(windowStart int64) {
	if windowStart >= r.fileSize || r.findActiveLocked(windowStart) != nil {
		return
	}
	w := r.dl.WindowCache.GetOrCreate(r.url, r.headers, windowStart, r.windowSize, r.fileSize)
	r.addActiveLocked(w)
}

// Status returns the cache state of all windows for this file's URL.
func (r *ReadAheadReader) Status() []WindowStatus {
	return r.dl.WindowCache.StatusForURL(r.url)
}

// PurgeAll drops all cached windows for this file.
func (r *ReadAheadReader) PurgeAll() {
	r.dl.WindowCache.PurgeURL(r.url)
	r.mu.Lock()
	r.active = nil
	r.mu.Unlock()
}

// PurgeWindow drops one cached window.
func (r *ReadAheadReader) PurgeWindow(start int64) bool {
	r.dl.WindowCache.Remove(r.url, start)
	r.mu.Lock()
	for i, w := range r.active {
		if w.Start == start {
			r.active = append(r.active[:i], r.active[i+1:]...)
			break
		}
	}
	r.mu.Unlock()
	return true
}

// Close releases this reader's active window references.
// Window data remains in WindowCache for future readers.
func (r *ReadAheadReader) Close() {
	r.mu.Lock()
	r.active = nil
	r.mu.Unlock()
}
