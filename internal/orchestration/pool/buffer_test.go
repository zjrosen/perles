package pool

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutputBuffer_Basic(t *testing.T) {
	buf := NewOutputBuffer(3)

	require.Equal(t, 0, buf.Len())
	require.Equal(t, 3, buf.Capacity())
	require.Empty(t, buf.Lines())
	require.Empty(t, buf.String())
}

func TestOutputBuffer_WriteAndRead(t *testing.T) {
	buf := NewOutputBuffer(5)

	buf.Write("line 1")
	buf.Write("line 2")
	buf.Write("line 3")

	require.Equal(t, 3, buf.Len())
	require.Equal(t, []string{"line 1", "line 2", "line 3"}, buf.Lines())
	require.Equal(t, "line 1\nline 2\nline 3", buf.String())
}

func TestOutputBuffer_RingBehavior(t *testing.T) {
	buf := NewOutputBuffer(3)

	// Fill the buffer
	buf.Write("line 1")
	buf.Write("line 2")
	buf.Write("line 3")

	// Buffer is full, now write more
	buf.Write("line 4") // Should overwrite "line 1"
	require.Equal(t, 3, buf.Len())
	require.Equal(t, []string{"line 2", "line 3", "line 4"}, buf.Lines())

	buf.Write("line 5") // Should overwrite "line 2"
	require.Equal(t, []string{"line 3", "line 4", "line 5"}, buf.Lines())

	buf.Write("line 6") // Should overwrite "line 3"
	require.Equal(t, []string{"line 4", "line 5", "line 6"}, buf.Lines())
}

func TestOutputBuffer_LastN(t *testing.T) {
	buf := NewOutputBuffer(5)

	buf.Write("line 1")
	buf.Write("line 2")
	buf.Write("line 3")
	buf.Write("line 4")
	buf.Write("line 5")

	// Last 3 lines
	require.Equal(t, []string{"line 3", "line 4", "line 5"}, buf.LastN(3))

	// Last 1 line
	require.Equal(t, []string{"line 5"}, buf.LastN(1))

	// More than available
	require.Equal(t, []string{"line 1", "line 2", "line 3", "line 4", "line 5"}, buf.LastN(10))

	// Zero or negative
	require.Nil(t, buf.LastN(0))
	require.Nil(t, buf.LastN(-1))
}

func TestOutputBuffer_LastN_AfterWrap(t *testing.T) {
	buf := NewOutputBuffer(3)

	// Fill and wrap
	buf.Write("line 1")
	buf.Write("line 2")
	buf.Write("line 3")
	buf.Write("line 4") // Wraps, buffer now: ["line 2", "line 3", "line 4"]

	// LastN should still work correctly after wrap
	require.Equal(t, []string{"line 3", "line 4"}, buf.LastN(2))
	require.Equal(t, []string{"line 4"}, buf.LastN(1))
	require.Equal(t, []string{"line 2", "line 3", "line 4"}, buf.LastN(5))
}

func TestOutputBuffer_Clear(t *testing.T) {
	buf := NewOutputBuffer(5)

	buf.Write("line 1")
	buf.Write("line 2")
	buf.Write("line 3")

	require.Equal(t, 3, buf.Len())

	buf.Clear()

	require.Equal(t, 0, buf.Len())
	require.Empty(t, buf.Lines())
}

func TestOutputBuffer_MinCapacity(t *testing.T) {
	// Capacity less than 1 should be clamped to 1
	buf := NewOutputBuffer(0)
	require.Equal(t, 1, buf.Capacity())

	buf = NewOutputBuffer(-5)
	require.Equal(t, 1, buf.Capacity())
}

func TestOutputBuffer_Concurrent(t *testing.T) {
	buf := NewOutputBuffer(100)
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				buf.Write("line")
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = buf.Lines()
				_ = buf.LastN(10)
				_ = buf.Len()
				_ = buf.String()
			}
		}()
	}

	wg.Wait()

	// Should not panic and should have data
	require.LessOrEqual(t, buf.Len(), 100)
}

func TestOutputBuffer_LinesReturnsCopy(t *testing.T) {
	buf := NewOutputBuffer(5)
	buf.Write("line 1")
	buf.Write("line 2")

	lines := buf.Lines()
	// Modify the returned slice
	lines[0] = "modified"

	// Original buffer should be unchanged
	require.Equal(t, []string{"line 1", "line 2"}, buf.Lines())
}
