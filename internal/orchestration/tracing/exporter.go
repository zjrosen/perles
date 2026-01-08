package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// FileExporter exports spans to a JSONL file for local development and debugging.
// It implements the sdktrace.SpanExporter interface.
type FileExporter struct {
	file *os.File
	mu   sync.Mutex
}

// NewFileExporter creates a new file exporter that writes spans to the given path.
// The file is created if it doesn't exist, and appended to if it does.
// Parent directories are created automatically.
func NewFileExporter(path string) (*FileExporter, error) {
	// Clean the path to prevent path traversal attacks
	cleanPath := filepath.Clean(path)

	// Ensure parent directory exists
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create trace directory: %w", err)
	}

	file, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600) // #nosec G304 -- path is cleaned above
	if err != nil {
		return nil, fmt.Errorf("open trace file: %w", err)
	}
	return &FileExporter{file: file}, nil
}

// ExportSpans writes spans to the file in JSONL format.
// Each span is written as a single JSON object on its own line.
func (e *FileExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if len(spans) == 0 {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	encoder := json.NewEncoder(e.file)
	for _, span := range spans {
		record := spanToRecord(span)
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("encode span: %w", err)
		}
	}
	return nil
}

// Shutdown closes the file and releases resources.
func (e *FileExporter) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.file != nil {
		err := e.file.Close()
		e.file = nil
		return err
	}
	return nil
}

// SpanRecord is the JSON structure for exported spans.
// This format is designed for easy parsing with jq and other JSON tools.
type SpanRecord struct {
	TraceID      string         `json:"trace_id"`
	SpanID       string         `json:"span_id"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Name         string         `json:"name"`
	Kind         string         `json:"kind"`
	StartTime    string         `json:"start_time"`
	EndTime      string         `json:"end_time"`
	DurationMs   float64        `json:"duration_ms"`
	Status       string         `json:"status"`
	StatusMsg    string         `json:"status_message,omitempty"`
	Attributes   map[string]any `json:"attributes,omitempty"`
	Events       []EventRecord  `json:"events,omitempty"`
}

// EventRecord is the JSON structure for span events.
type EventRecord struct {
	Name       string         `json:"name"`
	Timestamp  string         `json:"timestamp"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// spanToRecord converts an OpenTelemetry span to our JSON format.
func spanToRecord(span sdktrace.ReadOnlySpan) SpanRecord {
	sc := span.SpanContext()

	// Build parent span ID
	parentSpanID := ""
	if span.Parent().IsValid() {
		parentSpanID = span.Parent().SpanID().String()
	}

	// Convert span kind to string
	kindStr := spanKindToString(span.SpanKind())

	// Convert status
	status := span.Status()
	statusStr := "UNSET"
	switch status.Code {
	case codes.Ok:
		statusStr = "OK"
	case codes.Error:
		statusStr = "ERROR"
	}

	// Calculate duration
	duration := span.EndTime().Sub(span.StartTime())

	// Convert attributes
	attrs := make(map[string]any)
	for _, kv := range span.Attributes() {
		attrs[string(kv.Key)] = kv.Value.AsInterface()
	}

	// Convert events
	var events []EventRecord
	for _, evt := range span.Events() {
		evtAttrs := make(map[string]any)
		for _, kv := range evt.Attributes {
			evtAttrs[string(kv.Key)] = kv.Value.AsInterface()
		}
		events = append(events, EventRecord{
			Name:       evt.Name,
			Timestamp:  evt.Time.Format(time.RFC3339Nano),
			Attributes: evtAttrs,
		})
	}

	return SpanRecord{
		TraceID:      sc.TraceID().String(),
		SpanID:       sc.SpanID().String(),
		ParentSpanID: parentSpanID,
		Name:         span.Name(),
		Kind:         kindStr,
		StartTime:    span.StartTime().Format(time.RFC3339Nano),
		EndTime:      span.EndTime().Format(time.RFC3339Nano),
		DurationMs:   float64(duration.Microseconds()) / 1000.0,
		Status:       statusStr,
		StatusMsg:    status.Description,
		Attributes:   attrs,
		Events:       events,
	}
}

// spanKindToString converts a trace.SpanKind to a string.
func spanKindToString(kind trace.SpanKind) string {
	switch kind {
	case trace.SpanKindInternal:
		return "INTERNAL"
	case trace.SpanKindServer:
		return "SERVER"
	case trace.SpanKindClient:
		return "CLIENT"
	case trace.SpanKindProducer:
		return "PRODUCER"
	case trace.SpanKindConsumer:
		return "CONSUMER"
	default:
		return "UNSPECIFIED"
	}
}
