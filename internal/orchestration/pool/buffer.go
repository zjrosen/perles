// Package pool provides worker pool management for orchestrating multiple Claude processes.
package pool

import (
	"strings"
	"sync"
)

// OutputBuffer is a thread-safe ring buffer for storing recent output lines.
// It maintains a bounded memory footprint by discarding older lines when capacity is reached.
type OutputBuffer struct {
	lines    []string
	capacity int
	start    int // Index of oldest line
	count    int // Number of lines stored
	mu       sync.RWMutex
}

// NewOutputBuffer creates a new OutputBuffer with the specified capacity.
// Capacity must be at least 1.
func NewOutputBuffer(capacity int) *OutputBuffer {
	if capacity < 1 {
		capacity = 1
	}
	return &OutputBuffer{
		lines:    make([]string, capacity),
		capacity: capacity,
	}
}

// Write appends a line to the buffer.
// If the buffer is full, the oldest line is overwritten.
func (b *OutputBuffer) Write(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.count < b.capacity {
		// Buffer not full, append
		b.lines[b.count] = line
		b.count++
	} else {
		// Buffer full, overwrite oldest
		b.lines[b.start] = line
		b.start = (b.start + 1) % b.capacity
	}
}

// Lines returns all lines in the buffer in chronological order.
// Returns a copy to prevent races.
func (b *OutputBuffer) Lines() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]string, b.count)
	for i := 0; i < b.count; i++ {
		idx := (b.start + i) % b.capacity
		result[i] = b.lines[idx]
	}
	return result
}

// LastN returns the last n lines from the buffer.
// If n exceeds the number of stored lines, all lines are returned.
func (b *OutputBuffer) LastN(n int) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if n > b.count {
		n = b.count
	}
	if n <= 0 {
		return nil
	}

	result := make([]string, n)
	startIdx := b.count - n
	for i := 0; i < n; i++ {
		idx := (b.start + startIdx + i) % b.capacity
		result[i] = b.lines[idx]
	}
	return result
}

// Len returns the number of lines currently stored.
func (b *OutputBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}

// Capacity returns the maximum number of lines the buffer can hold.
func (b *OutputBuffer) Capacity() int {
	return b.capacity
}

// String returns all lines joined with newlines.
func (b *OutputBuffer) String() string {
	lines := b.Lines()
	return strings.Join(lines, "\n")
}

// Clear removes all lines from the buffer.
func (b *OutputBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.start = 0
	b.count = 0
}
