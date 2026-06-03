package lsp

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/posthog/posthog-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.28.0"
	"go.opentelemetry.io/otel/trace"
)

type Telemetry struct {
	mu             sync.RWMutex
	PHClient       posthog.Client
	TracerProvider *sdktrace.TracerProvider
	Tracer         trace.Tracer
	Enabled        bool
}

var globalTelemetry *Telemetry

// InitTelemetry initializes the OTLP exporter and PostHog client.
func InitTelemetry(version string) (*Telemetry, error) {
	token := os.Getenv("POSTHOG_PROJECT_TOKEN")
	if token == "" {
		globalTelemetry = &Telemetry{Enabled: false}
		return globalTelemetry, nil
	}

	host := os.Getenv("POSTHOG_HOST")
	if host == "" {
		host = "us.i.posthog.com" // default
	}

	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("lugo-lsp"),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(host),
		otlptracehttp.WithURLPath("/i/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + token,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	phClient, err := posthog.NewWithConfig(
		token,
		posthog.Config{
			Endpoint: "https://" + host,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create posthog client: %w", err)
	}

	globalTelemetry = &Telemetry{
		PHClient:       phClient,
		TracerProvider: tp,
		Tracer:         tp.Tracer("lugo-lsp"),
		Enabled:        true,
	}

	return globalTelemetry, nil
}

// SetTelemetryEnabled dynamically updates the telemetry preference.
func SetTelemetryEnabled(enabled bool) {
	if globalTelemetry == nil {
		return
	}
	globalTelemetry.mu.Lock()
	defer globalTelemetry.mu.Unlock()
	
	// If it was already false due to missing token, don't enable it
	if globalTelemetry.PHClient == nil {
		globalTelemetry.Enabled = false
		return
	}
	
	globalTelemetry.Enabled = enabled
}

// Close gracefully shuts down telemetry.
func (t *Telemetry) Close() {
	t.mu.RLock()
	enabled := t.Enabled
	t.mu.RUnlock()
	
	if !enabled {
		return
	}
	ctx := context.Background()
	_ = t.TracerProvider.Shutdown(ctx)
	_ = t.PHClient.Close()
}

// CapturePanic sends a panic event to PostHog as an exception.
func CapturePanic(r any, source string) {
	if globalTelemetry == nil {
		return
	}
	
	globalTelemetry.mu.RLock()
	enabled := globalTelemetry.Enabled
	globalTelemetry.mu.RUnlock()
	
	if !enabled {
		return
	}

	errStr := fmt.Sprintf("panic: %v\n%s", r, debug.Stack())
	
	exception := posthog.NewDefaultException(
		time.Now(),
		"system", // we don't have user IDs in an LSP usually
		"PanicError",
		fmt.Sprintf("[%s] %v", source, r),
	)

	// Inject stack trace into properties
	exception.Properties = posthog.NewProperties().Set("stack", errStr).Set("source", source)

	globalTelemetry.PHClient.Enqueue(exception)
}

// Flush ensures queued events are sent (e.g., before exiting).
func FlushTelemetry() {
	if globalTelemetry == nil {
		return
	}
	
	globalTelemetry.mu.RLock()
	enabled := globalTelemetry.Enabled
	globalTelemetry.mu.RUnlock()
	
	if !enabled {
		return
	}
	_ = globalTelemetry.TracerProvider.ForceFlush(context.Background())
}
