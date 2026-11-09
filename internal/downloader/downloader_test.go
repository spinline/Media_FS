package downloader

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newMockRangeServer creates an HTTP test server that mimics a streaming media server (like Jellyfin)
// supporting RFC 7233 HTTP Range requests.
func newMockRangeServer(totalSize int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHdr := r.Header.Get("Range")
		if rangeHdr == "" {
			w.Header().Set("Content-Length", strconv.FormatInt(totalSize, 10))
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusOK)
			// write dummy data
			chunk := make([]byte, 32768)
			for written := int64(0); written < totalSize; {
				n := int64(len(chunk))
				if written+n > totalSize {
					n = totalSize - written
				}
				for i := int64(0); i < n; i++ {
					chunk[i] = byte((written + i) % 256)
				}
				w.Write(chunk[:n])
				written += n
			}
			return
		}

		if !strings.HasPrefix(rangeHdr, "bytes=") {
			http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
			return
		}

		parts := strings.Split(strings.TrimPrefix(rangeHdr, "bytes="), "-")
		if len(parts) != 2 {
			http.Error(w, "invalid range format", http.StatusBadRequest)
			return
		}

		start, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || start < 0 || start >= totalSize {
			http.Error(w, "invalid start range", http.StatusRequestedRangeNotSatisfiable)
			return
		}

		end := totalSize - 1
		if parts[1] != "" {
			parsedEnd, err := strconv.ParseInt(parts[1], 10, 64)
			if err == nil && parsedEnd >= start {
				if parsedEnd < totalSize {
					end = parsedEnd
				}
			}
		}

		length := end - start + 1
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)

		data := make([]byte, length)
		for i := int64(0); i < length; i++ {
			data[i] = byte((start + i) % 256)
		}
		w.Write(data)
	}))
}

func TestDownloader_HTTPRange(t *testing.T) {
	const totalSize = 10 * 1024 * 1024 // 10 MB simulated media file
	ts := newMockRangeServer(totalSize)
	defer ts.Close()

	dl := &Downloader{
		ParallelChunks: 2,
		ChunkSizeMB:    1,
		BufferSizeKB:   64,
		MaxCacheMB:     16,
		CacheTTLMin:    5,
		HTTPClient:     ts.Client(),
	}
	dl.Init()
	defer dl.WindowCache.Close()

	// Test 1: Fetch from beginning
	data, err := dl.ReadAt(ts.URL+"/media.mkv", nil, 0, 1024)
	if err != nil {
		t.Fatalf("ReadAt beginning failed: %v", err)
	}
	if len(data) != 1024 {
		t.Fatalf("expected 1024 bytes, got %d", len(data))
	}
	for i, b := range data {
		if b != byte(i%256) {
			t.Fatalf("byte mismatch at %d: expected %d, got %d", i, byte(i%256), b)
		}
	}

	// Test 2: Fetch from middle (simulate seeking)
	const seekOffset = 5 * 1024 * 1024 // 5 MB
	const readLen = 4096
	data, err = dl.ReadAt(ts.URL+"/media.mkv", nil, seekOffset, readLen)
	if err != nil {
		t.Fatalf("ReadAt middle seek failed: %v", err)
	}
	if len(data) != readLen {
		t.Fatalf("expected %d bytes, got %d", readLen, len(data))
	}
	for i, b := range data {
		expected := byte((seekOffset + int64(i)) % 256)
		if b != expected {
			t.Fatalf("byte mismatch at seek offset %d+%d: expected %d, got %d", seekOffset, i, expected, b)
		}
	}

	// Test 3: Fetch end of file (index/cues read)
	const endOffset = totalSize - 2048
	data, err = dl.ReadAt(ts.URL+"/media.mkv", nil, endOffset, 2048)
	if err != nil {
		t.Fatalf("ReadAt end of file failed: %v", err)
	}
	if len(data) != 2048 {
		t.Fatalf("expected 2048 bytes, got %d", len(data))
	}
	for i, b := range data {
		expected := byte((endOffset + int64(i)) % 256)
		if b != expected {
			t.Fatalf("byte mismatch at end offset %d+%d: expected %d, got %d", endOffset, i, expected, b)
		}
	}
}

func TestReadAheadReader_VideoSeeking(t *testing.T) {
	const totalSize = 20 * 1024 * 1024 // 20 MB simulated video file
	ts := newMockRangeServer(totalSize)
	defer ts.Close()

	dl := &Downloader{
		ParallelChunks: 2,
		ChunkSizeMB:    1,
		BufferSizeKB:   64,
		MaxCacheMB:     32,
		CacheTTLMin:    10,
		HTTPClient:     ts.Client(),
	}
	dl.Init()
	defer dl.WindowCache.Close()

	// 1 MB window size, 2 active windows
	reader := NewReadAheadReader(ts.URL+"/movie.mp4", nil, totalSize, dl, 1, 2)
	defer reader.Close()

	// Wait briefly for initial async prefetch to settle
	time.Sleep(50 * time.Millisecond)

	buf := make([]byte, 4096)

	// Step 1: Video player opens file -> reads header at offset 0
	n, err := reader.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("read header failed: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("expected %d bytes, got %d", len(buf), n)
	}
	for i := 0; i < n; i++ {
		if buf[i] != byte(i%256) {
			t.Fatalf("header byte mismatch at %d", i)
		}
	}

	// Step 2: Video player seeks to tail (moov atom / MKV cues at end of file)
	lastOffset := totalSize - int64(len(buf))
	n, err = reader.ReadAt(buf, lastOffset)
	if err != nil {
		t.Fatalf("read tail index failed: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("expected %d bytes, got %d", len(buf), n)
	}
	for i := 0; i < n; i++ {
		expected := byte((lastOffset + int64(i)) % 256)
		if buf[i] != expected {
			t.Fatalf("tail index byte mismatch at %d", i)
		}
	}

	// Step 3: Forward seek to middle of the video (e.g. 10 MB)
	seekForwardOffset := int64(10 * 1024 * 1024)
	n, err = reader.ReadAt(buf, seekForwardOffset)
	if err != nil {
		t.Fatalf("seek forward failed: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("expected %d bytes, got %d", len(buf), n)
	}
	for i := 0; i < n; i++ {
		expected := byte((seekForwardOffset + int64(i)) % 256)
		if buf[i] != expected {
			t.Fatalf("forward seek byte mismatch at %d", i)
		}
	}

	// Step 4: Play continuous video chunks around seek position
	for block := 1; block <= 5; block++ {
		curOffset := seekForwardOffset + int64(block*len(buf))
		n, err = reader.ReadAt(buf, curOffset)
		if err != nil {
			t.Fatalf("playback block %d failed: %v", block, err)
		}
		for i := 0; i < n; i++ {
			expected := byte((curOffset + int64(i)) % 256)
			if buf[i] != expected {
				t.Fatalf("playback block %d mismatch at %d", block, i)
			}
		}
	}

	// Step 5: Backward seek (user seeks back to 3 MB)
	seekBackOffset := int64(3 * 1024 * 1024)
	n, err = reader.ReadAt(buf, seekBackOffset)
	if err != nil {
		t.Fatalf("backward seek failed: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("expected %d bytes, got %d", len(buf), n)
	}
	for i := 0; i < n; i++ {
		expected := byte((seekBackOffset + int64(i)) % 256)
		if buf[i] != expected {
			t.Fatalf("backward seek byte mismatch at %d", i)
		}
	}
}
