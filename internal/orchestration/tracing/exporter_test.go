package tracing

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestNewFileExporter_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	exporter, err := NewFileExporter(tracePath)
	require.NoError(t, err)
	require.NotNil(t, exporter)

	// File should exist
	_, err = os.Stat(tracePath)
	require.NoError(t, err, "trace file should be created")

	err = exporter.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestNewFileExporter_CreatesParentDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "nested", "dir", "traces.jsonl")

	exporter, err := NewFileExporter(tracePath)
	require.NoError(t, err)
	require.NotNil(t, exporter)

	// File should exist
	_, err = os.Stat(tracePath)
	require.NoError(t, err, "trace file should be created with parent dirs")

	err = exporter.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestNewFileExporter_AppendsToExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	// Create file with initial content
	err := os.WriteFile(tracePath, []byte(`{"existing": "data"}`+"\n"), 0644)
	require.NoError(t, err)

	exporter, err := NewFileExporter(tracePath)
	require.NoError(t, err)

	// Write a span
	stub := tracetest.SpanStub{
		Name:      "test-span",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(100 * time.Millisecond),
	}
	err = exporter.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{stub.Snapshot()})
	require.NoError(t, err)

	err = exporter.Shutdown(context.Background())
	require.NoError(t, err)

	// Read file and verify both lines exist
	content, err := os.ReadFile(tracePath)
	require.NoError(t, err)

	lines := 0
	file, err := os.Open(tracePath)
	require.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines++
	}
	require.Equal(t, 2, lines, "file should have original line plus new span")

	// Verify original content is preserved
	require.Contains(t, string(content), `{"existing": "data"}`)
}

func TestFileExporter_WritesValidJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	exporter, err := NewFileExporter(tracePath)
	require.NoError(t, err)

	// Create test span with attributes and events
	stub := tracetest.SpanStub{
		Name:      "test-span",
		SpanKind:  trace.SpanKindInternal,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(100 * time.Millisecond),
		Status: sdktrace.Status{
			Code:        codes.Ok,
			Description: "",
		},
		Attributes: []attribute.KeyValue{
			attribute.String("command.id", "cmd-123"),
			attribute.String("command.type", "assign_task"),
			attribute.Int("command.priority", 1),
		},
		Events: []sdktrace.Event{
			{
				Name: "handler.started",
				Time: time.Now(),
				Attributes: []attribute.KeyValue{
					attribute.String("handler", "AssignTaskHandler"),
				},
			},
		},
	}

	err = exporter.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{stub.Snapshot()})
	require.NoError(t, err)

	err = exporter.Shutdown(context.Background())
	require.NoError(t, err)

	// Read and parse JSON
	file, err := os.Open(tracePath)
	require.NoError(t, err)
	defer file.Close()

	var record SpanRecord
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&record)
	require.NoError(t, err, "should be valid JSON")

	// Verify record fields
	require.Equal(t, "test-span", record.Name)
	require.Equal(t, "INTERNAL", record.Kind)
	require.Equal(t, "OK", record.Status)
	require.NotEmpty(t, record.StartTime)
	require.NotEmpty(t, record.EndTime)
	require.True(t, record.DurationMs > 0, "duration should be positive")

	// Verify attributes
	require.Equal(t, "cmd-123", record.Attributes["command.id"])
	require.Equal(t, "assign_task", record.Attributes["command.type"])
	require.EqualValues(t, 1, record.Attributes["command.priority"])

	// Verify events
	require.Len(t, record.Events, 1)
	require.Equal(t, "handler.started", record.Events[0].Name)
	require.Equal(t, "AssignTaskHandler", record.Events[0].Attributes["handler"])
}

func TestFileExporter_ThreadSafe(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	exporter, err := NewFileExporter(tracePath)
	require.NoError(t, err)

	// Concurrently write spans
	var wg sync.WaitGroup
	numGoroutines := 10
	spansPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < spansPerGoroutine; j++ {
				stub := tracetest.SpanStub{
					Name:      "concurrent-span",
					StartTime: time.Now(),
					EndTime:   time.Now().Add(time.Millisecond),
					Attributes: []attribute.KeyValue{
						attribute.Int("worker", workerID),
						attribute.Int("iteration", j),
					},
				}
				err := exporter.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{stub.Snapshot()})
				require.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	err = exporter.Shutdown(context.Background())
	require.NoError(t, err)

	// Verify all spans were written
	file, err := os.Open(tracePath)
	require.NoError(t, err)
	defer file.Close()

	var count int
	decoder := json.NewDecoder(file)
	for {
		var record SpanRecord
		if err := decoder.Decode(&record); err != nil {
			break
		}
		count++
		// Verify each record is valid JSON (would fail if writes were corrupted)
		require.NotEmpty(t, record.Name)
	}

	expectedCount := numGoroutines * spansPerGoroutine
	require.Equal(t, expectedCount, count, "all spans should be written")
}

func TestFileExporter_Shutdown_ClosesFile(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	exporter, err := NewFileExporter(tracePath)
	require.NoError(t, err)

	// First shutdown should succeed
	err = exporter.Shutdown(context.Background())
	require.NoError(t, err)

	// Second shutdown should also succeed (idempotent)
	err = exporter.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestFileExporter_ExportEmptySpans(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	exporter, err := NewFileExporter(tracePath)
	require.NoError(t, err)

	// Exporting empty slice should succeed
	err = exporter.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{})
	require.NoError(t, err)

	err = exporter.Shutdown(context.Background())
	require.NoError(t, err)

	// File should be empty
	info, err := os.Stat(tracePath)
	require.NoError(t, err)
	require.Zero(t, info.Size(), "file should be empty after exporting no spans")
}

func TestFileExporter_MultipleSpanBatch(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	exporter, err := NewFileExporter(tracePath)
	require.NoError(t, err)

	// Export multiple spans in one batch
	spans := make([]sdktrace.ReadOnlySpan, 5)
	for i := 0; i < 5; i++ {
		stub := tracetest.SpanStub{
			Name:      "batch-span",
			StartTime: time.Now(),
			EndTime:   time.Now().Add(time.Millisecond),
			Attributes: []attribute.KeyValue{
				attribute.Int("index", i),
			},
		}
		spans[i] = stub.Snapshot()
	}

	err = exporter.ExportSpans(context.Background(), spans)
	require.NoError(t, err)

	err = exporter.Shutdown(context.Background())
	require.NoError(t, err)

	// Verify all 5 spans were written
	file, err := os.Open(tracePath)
	require.NoError(t, err)
	defer file.Close()

	var count int
	decoder := json.NewDecoder(file)
	for {
		var record SpanRecord
		if err := decoder.Decode(&record); err != nil {
			break
		}
		count++
	}
	require.Equal(t, 5, count)
}

func TestSpanKindToString(t *testing.T) {
	tests := []struct {
		kind     trace.SpanKind
		expected string
	}{
		{trace.SpanKindInternal, "INTERNAL"},
		{trace.SpanKindServer, "SERVER"},
		{trace.SpanKindClient, "CLIENT"},
		{trace.SpanKindProducer, "PRODUCER"},
		{trace.SpanKindConsumer, "CONSUMER"},
		{trace.SpanKindUnspecified, "UNSPECIFIED"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := spanKindToString(tt.kind)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestSpanRecord_ErrorStatus(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	exporter, err := NewFileExporter(tracePath)
	require.NoError(t, err)

	stub := tracetest.SpanStub{
		Name:      "error-span",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(100 * time.Millisecond),
		Status: sdktrace.Status{
			Code:        codes.Error,
			Description: "something went wrong",
		},
	}

	err = exporter.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{stub.Snapshot()})
	require.NoError(t, err)

	err = exporter.Shutdown(context.Background())
	require.NoError(t, err)

	// Read and verify
	file, err := os.Open(tracePath)
	require.NoError(t, err)
	defer file.Close()

	var record SpanRecord
	err = json.NewDecoder(file).Decode(&record)
	require.NoError(t, err)

	require.Equal(t, "ERROR", record.Status)
	require.Equal(t, "something went wrong", record.StatusMsg)
}
