package session

import (
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultBufferSize is the default ring buffer capacity (256 events).
	DefaultBufferSize = 256

	// DefaultFlushInterval is how often the background goroutine flushes to disk.
	DefaultFlushInterval = 100 * time.Millisecond

	// FlushThresholdPercent is the buffer fill percentage that triggers an immediate flush.
	FlushThresholdPercent = 75
)

// BufferedWriter provides buffered writes to a file with a ring buffer.
// It decouples pub/sub event reception from disk I/O using a background goroutine.
//
// Key features:
//   - 256-event ring buffer (configurable)
//   - Background flush every 100ms via time.Ticker
//   - Immediate flush when buffer is 75% full
//   - Thread-safe writes via mutex
//   - Error tracking via atomic counters
//   - Graceful degradation on write errors (no panics)
type BufferedWriter struct {
	// file is the underlying file handle for disk writes.
	file *os.File

	// buffer is the ring buffer holding pending writes.
	buffer [][]byte

	// bufferSize is the maximum number of events in the buffer.
	bufferSize int

	// flushThreshold is the number of events that triggers an immediate flush.
	flushThreshold int

	// flushInterval is the periodic flush interval.
	flushInterval time.Duration

	// mu protects buffer access.
	mu sync.Mutex

	// writeErrors tracks the total number of write errors.
	writeErrors atomic.Int64

	// lastError stores the most recent write error.
	lastError atomic.Value

	// done signals the background goroutine to stop.
	done chan struct{}

	// wg waits for the background goroutine to finish.
	wg sync.WaitGroup

	// closed indicates whether the writer has been closed.
	closed bool
}

// NewBufferedWriter creates a new BufferedWriter for the given file.
// It starts a background goroutine that flushes the buffer every 100ms.
func NewBufferedWriter(file *os.File) *BufferedWriter {
	return NewBufferedWriterWithConfig(file, DefaultBufferSize, DefaultFlushInterval)
}

// NewBufferedWriterWithConfig creates a BufferedWriter with custom buffer size and flush interval.
func NewBufferedWriterWithConfig(file *os.File, bufferSize int, flushInterval time.Duration) *BufferedWriter {
	if bufferSize <= 0 {
		bufferSize = DefaultBufferSize
	}
	if flushInterval <= 0 {
		flushInterval = DefaultFlushInterval
	}

	w := &BufferedWriter{
		file:           file,
		buffer:         make([][]byte, 0, bufferSize),
		bufferSize:     bufferSize,
		flushThreshold: (bufferSize * FlushThresholdPercent) / 100,
		flushInterval:  flushInterval,
		done:           make(chan struct{}),
	}

	// Start the background flush goroutine
	w.wg.Add(1)
	go w.flushLoop()

	return w
}

// Write appends data to the ring buffer.
// If the buffer reaches 75% capacity, an immediate flush is triggered.
// This method is thread-safe.
func (w *BufferedWriter) Write(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return os.ErrClosed
	}

	// Make a copy of the data to avoid issues with reused slices
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	w.buffer = append(w.buffer, dataCopy)

	// Check if we need to flush (75% full)
	if len(w.buffer) >= w.flushThreshold {
		return w.flushLocked()
	}

	return nil
}

// Flush writes all buffered events to disk.
// This method is thread-safe.
func (w *BufferedWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return os.ErrClosed
	}

	return w.flushLocked()
}

// flushLocked writes all buffered events to disk.
// Caller must hold w.mu.
func (w *BufferedWriter) flushLocked() error {
	if len(w.buffer) == 0 {
		return nil
	}

	// Write all buffered data to the file
	var writeErr error
	for _, data := range w.buffer {
		if _, err := w.file.Write(data); err != nil {
			writeErr = err
			w.writeErrors.Add(1)
			w.lastError.Store(err)
			// Continue writing remaining events; don't stop on first error
		}
	}

	// Clear the buffer (reuse the underlying slice)
	w.buffer = w.buffer[:0]

	return writeErr
}

// flushLoop is the background goroutine that periodically flushes the buffer.
func (w *BufferedWriter) flushLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			_ = w.Flush() // Errors are tracked via atomic counters
		}
	}
}

// Close stops the background goroutine, performs a final flush, and closes the file.
// After Close returns, no more writes are accepted.
func (w *BufferedWriter) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return os.ErrClosed
	}
	w.closed = true
	w.mu.Unlock()

	// Signal the background goroutine to stop
	close(w.done)

	// Wait for the background goroutine to finish
	w.wg.Wait()

	// Perform final flush
	w.mu.Lock()
	flushErr := w.flushLocked()
	w.mu.Unlock()

	// Close the file
	closeErr := w.file.Close()

	// Return the first error encountered
	if flushErr != nil {
		return flushErr
	}
	return closeErr
}

// ErrorCount returns the total number of write errors encountered.
func (w *BufferedWriter) ErrorCount() int64 {
	return w.writeErrors.Load()
}

// LastError returns the most recent write error, or nil if no errors have occurred.
func (w *BufferedWriter) LastError() error {
	if err := w.lastError.Load(); err != nil {
		return err.(error)
	}
	return nil
}

// Len returns the current number of buffered events.
// This method is thread-safe.
func (w *BufferedWriter) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.buffer)
}
