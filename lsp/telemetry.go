package lsp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/posthog/posthog-go"
)

const (
	defaultJournalBytes      = 1 << 20
	maxMetadataTextBytes     = 512
	maxCrashStackBytes       = 16 << 10
	maxRecentTraceActivities = 16
	posthogProjectToken      = "phc_AtCceYjFoZzdnFgfKNMGArJGbLMyFzzqvjBx7SQCou6k"
	posthogIngestionHost     = "https://eu.i.posthog.com"
)

type telemetryContextKey struct{}

// TraceContext is a small local correlation context for journaled traces.
type TraceContext struct{ TraceID, SpanID string }
type journalEntry struct{ Data []byte }
type traceJournal struct {
	mu         sync.Mutex
	file       *os.File
	path       string
	max, bytes int
	entries    []journalEntry
	err        error
}

func (j *traceJournal) append(v any) {
	if j == nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	data = append(data, '\n')
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(data) > j.max {
		data = data[len(data)-j.max:]
	}
	for j.bytes+len(data) > j.max && len(j.entries) > 0 {
		j.bytes -= len(j.entries[0].Data)
		j.entries = j.entries[1:]
	}
	j.entries = append(j.entries, journalEntry{Data: append([]byte(nil), data...)})
	j.bytes += len(data)
	if j.file != nil {
		if err := j.rewriteLocked(); err != nil {
			j.err = err
		}
	}
}

func (j *traceJournal) rewriteLocked() error {
	if _, err := j.file.Seek(0, 0); err != nil {
		return err
	}
	if err := j.file.Truncate(0); err != nil {
		return err
	}
	for _, entry := range j.entries {
		if _, err := j.file.Write(entry.Data); err != nil {
			return err
		}
	}
	return nil
}

// purge removes the in-memory and on-disk local trace history. It cannot retract
// events already accepted by PostHog before the user opted out.
func (j *traceJournal) purge() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.entries = nil
	j.bytes = 0
	var err error
	if j.file != nil {
		err = j.file.Close()
		j.file = nil
	}
	if j.path != "" {
		if removeErr := os.Remove(j.path); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
			err = removeErr
		}
	}
	j.err = err
	return err
}

func (j *traceJournal) close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.file == nil {
		return j.err
	}
	err := j.file.Close()
	j.file = nil
	if j.err != nil {
		return j.err
	}
	return err
}

// Telemetry owns only PostHog analytics and a bounded local journal. It never creates a remote trace exporter.
type Telemetry struct {
	mu               sync.RWMutex
	PHClient         posthog.Client
	Enabled          bool
	journal          *traceJournal
	closed, optedOut bool
	build            map[string]string
	recent           []traceActivity
	enqueue          func(posthog.Message) error // test seam; production uses PHClient.Enqueue.
	deliveryFailures atomic.Uint64
}

type traceActivity struct {
	Event   string    `json:"event"`
	TraceID string    `json:"trace_id,omitempty"`
	SpanID  string    `json:"span_id,omitempty"`
	Time    time.Time `json:"time"`
}

var globalTelemetry *Telemetry
var redactionPattern = regexp.MustCompile(`(?i)(password|passwd|token|secret|authorization|api[_-]?key)\s*[:=]\s*[^\s,;]+`)
var pathPattern = regexp.MustCompile(`(?i)(?:file://)?(?:[a-z]:[\\/]|\\\\)[^\s"']+|(?:^|[\s(])(?:file://)?/(?:[^/\s"']+/)+[^/\s"']*`)

func panicStack() string {
	buf := make([]byte, 64*1024)
	n := runtime.Stack(buf, true)
	if n > len(buf) {
		n = len(buf)
	}
	return string(buf[:n])
}

func envEnabled() bool {
	v, ok := os.LookupEnv("LUGO_TELEMETRY")
	if !ok {
		return true
	}
	b, err := strconv.ParseBool(v)
	return err == nil && b
}
func envEnabledWithDefault(name string, fallback bool) bool {
	v, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	return err == nil && b
}
func envInt(name string, fallback int) int {
	n, err := strconv.Atoi(os.Getenv(name))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func traceJournalPath() string {
	if path := os.Getenv("LUGO_TRACE_JOURNAL"); path != "" {
		return path
	}
	if cache, err := os.UserCacheDir(); err == nil {
		return filepath.Join(cache, "lugo", "trace.ndjson")
	}
	return ""
}

func processBuildMetadata(version string) map[string]string {
	metadata := map[string]string{
		"version":    redactMetadataText(version),
		"go_version": runtime.Version(),
		"go_os":      runtime.GOOS,
		"go_arch":    runtime.GOARCH,
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision", "vcs.modified":
				metadata[setting.Key] = redactMetadataText(setting.Value)
			}
		}
	}
	return metadata
}

// InitTelemetry configures anonymous PostHog analytics for the EU project.
func InitTelemetry(version string) (*Telemetry, error) {
	localOnly := envEnabledWithDefault("LUGO_TELEMETRY_LOCAL_ONLY", false)
	enabled := envEnabled()
	t := &Telemetry{Enabled: enabled && !localOnly, build: processBuildMetadata(version)}
	globalTelemetry = t
	journalPath := traceJournalPath()
	// An explicit global opt-out must not open, retain, or recreate a local
	// journal. Local-only mode deliberately keeps the bounded journal while
	// disabling all PostHog delivery, for offline debugging and tests.
	if !enabled {
		t.optedOut = true
		if journalPath != "" {
			if err := os.Remove(journalPath); err != nil && !os.IsNotExist(err) {
				return t, fmt.Errorf("purge trace journal: %w", err)
			}
		}
		return t, nil
	}
	t.journal = &traceJournal{max: envInt("LUGO_TRACE_MAX_BYTES", defaultJournalBytes)}
	if journalPath != "" {
		if err := os.MkdirAll(filepath.Dir(journalPath), 0700); err != nil {
			return t, fmt.Errorf("create trace journal directory: %w", err)
		}
		f, err := os.OpenFile(journalPath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return t, fmt.Errorf("open trace journal: %w", err)
		}
		t.journal.file, t.journal.path = f, journalPath
		if err := loadTraceJournal(t.journal); err != nil {
			_ = f.Close()
			return t, fmt.Errorf("load trace journal: %w", err)
		}
	}
	if localOnly {
		return t, nil
	}
	client, err := posthog.NewWithConfig(posthogProjectToken, posthog.Config{Endpoint: posthogIngestionHost})
	if err != nil {
		t.Enabled = false
		return t, fmt.Errorf("create posthog client: %w", err)
	}
	t.PHClient = client
	t.enqueue = client.Enqueue
	return t, nil
}

func loadTraceJournal(j *traceJournal) error {
	if j == nil || j.file == nil {
		return nil
	}
	info, err := j.file.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return nil
	}
	size := info.Size()
	if size > int64(j.max) {
		size = int64(j.max)
	}
	data := make([]byte, size)
	if _, err := j.file.ReadAt(data, info.Size()-size); err != nil {
		return err
	}
	for index, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 || (index == 0 && line[0] != '{') {
			continue
		}
		line = append(line, '\n')
		j.entries = append(j.entries, journalEntry{Data: append([]byte(nil), line...)})
		j.bytes += len(line)
	}
	return j.rewriteLocked()
}

// SetTelemetryEnabled applies the editor preference. Disabling is terminal for
// this process: queued delivery is closed and the local trace file is purged.
func SetTelemetryEnabled(enabled bool) {
	if globalTelemetry == nil {
		return
	}
	if !enabled {
		globalTelemetry.disableAndPurge()
		return
	}
	globalTelemetry.mu.Lock()
	defer globalTelemetry.mu.Unlock()
	if globalTelemetry.PHClient != nil && !globalTelemetry.closed && !globalTelemetry.optedOut {
		globalTelemetry.Enabled = true
	}
}

func (t *Telemetry) disableAndPurge() {
	t.mu.Lock()
	if t.optedOut {
		t.mu.Unlock()
		return
	}
	t.Enabled, t.optedOut = false, true
	client, journal := t.PHClient, t.journal
	t.mu.Unlock()
	// Do not report cleanup/delivery failures: that could recurse or make an
	// opt-out perform another network operation. Close only drops the client queue.
	if client != nil {
		_ = client.Close()
	}
	_ = journal.purge()
}

func (t *Telemetry) Close() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	client, journal := t.PHClient, t.journal
	t.mu.Unlock()
	if journal != nil {
		_ = journal.close()
	}
	if client != nil {
		_ = client.Close()
	}
}

// Record stores a bounded local event and, when enabled, queues only redacted
// metadata for PostHog. Enqueue is asynchronous; failures are counted locally
// and never generate telemetry events themselves.
func (t *Telemetry) Record(ctx context.Context, event string, properties map[string]any) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.optedOut {
		return
	}
	tc := TraceContextFromContext(ctx)
	redacted := make(map[string]any, len(properties)+5)
	for key, value := range properties {
		// Crash stacks have their own redaction and larger cap. Recent trace
		// identifiers and build metadata are pre-sanitized structured fields,
		// not ordinary free-form metadata.
		if event == "lugo.panic" {
			switch key {
			case "stack":
				redacted[key] = redactCrashStack(fmt.Sprint(value))
				continue
			case "recent_trace_context", "process":
				redacted[key] = value
				continue
			}
		}
		redacted[key] = redactMetadataText(fmt.Sprint(value))
	}
	redacted["trace_id"] = redactMetadataText(tc.TraceID)
	redacted["span_id"] = redactMetadataText(tc.SpanID)
	redacted["time"] = time.Now().UTC()
	redacted["delivery_failures"] = t.deliveryFailures.Load()
	payload := map[string]any{"event": event, "properties": redacted}
	if t.journal != nil {
		t.journal.append(payload)
	}
	t.addRecentLocked(event, tc)
	if t.Enabled && t.enqueue != nil {
		props := posthog.NewProperties()
		for key, value := range redacted {
			props.Set(key, value)
		}
		if err := t.enqueue(posthog.Capture{DistinctId: "lugo", Event: event, Properties: props}); err != nil {
			t.deliveryFailures.Add(1)
		}
	}
}

func (t *Telemetry) addRecentLocked(event string, tc TraceContext) {
	a := traceActivity{Event: redactMetadataText(event), TraceID: redactMetadataText(tc.TraceID), SpanID: redactMetadataText(tc.SpanID), Time: time.Now().UTC()}
	if len(t.recent) == maxRecentTraceActivities {
		copy(t.recent, t.recent[1:])
		t.recent[len(t.recent)-1] = a
		return
	}
	t.recent = append(t.recent, a)
}

func (t *Telemetry) recentTraceContext() []traceActivity {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return append([]traceActivity(nil), t.recent...)
}

// DeliveryFailures reports locally observed queue failures. It has no network side effects.
func (t *Telemetry) DeliveryFailures() uint64 {
	if t == nil {
		return 0
	}
	return t.deliveryFailures.Load()
}

func (t *Telemetry) Flush() error {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	journal := t.journal
	t.mu.RUnlock()
	if journal == nil {
		return nil
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.file != nil {
		if err := journal.file.Sync(); err != nil {
			journal.err = err
			return err
		}
	}
	return journal.err
}
func FlushTelemetry() {
	if globalTelemetry != nil {
		_ = globalTelemetry.Flush()
	}
}

// RedactTelemetryText bounds ordinary metadata before it is retained or sent.
func RedactTelemetryText(s string) string { return redactMetadataText(s) }

func redactMetadataText(s string) string {
	s = redactionPattern.ReplaceAllString(s, "$1=[REDACTED]")
	s = pathPattern.ReplaceAllString(s, "[REDACTED]")
	if len(s) > maxMetadataTextBytes {
		s = s[:maxMetadataTextBytes] + "…"
	}
	return s
}

// redactCrashStack uses a separate, larger bound from normal event metadata.
func redactCrashStack(s string) string {
	s = redactionPattern.ReplaceAllString(s, "$1=[REDACTED]")
	s = pathPattern.ReplaceAllString(s, "[REDACTED]")
	if len(s) > maxCrashStackBytes {
		s = s[:maxCrashStackBytes] + "…"
	}
	return s
}

func CapturePanic(r any, source string) { CapturePanicContext(context.Background(), r, source) }

// CapturePanicContext emits a privacy-bounded crash envelope with recent trace
// identifiers and process/build metadata, never arbitrary prior event payloads.
func CapturePanicContext(ctx context.Context, r any, source string) {
	t := globalTelemetry
	if t == nil {
		return
	}
	t.mu.RLock()
	closed, optedOut := t.closed, t.optedOut
	build := make(map[string]string, len(t.build))
	for key, value := range t.build {
		build[key] = value
	}
	t.mu.RUnlock()
	if closed || optedOut {
		return
	}
	t.Record(ctx, "lugo.panic", map[string]any{
		"source":               source,
		"message":              fmt.Sprint(r),
		"stack":                redactCrashStack(panicStack()),
		"recent_trace_context": t.recentTraceContext(),
		"process":              build,
	})
}

func NewTraceContext() TraceContext {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return TraceContext{TraceID: "0", SpanID: "0"}
	}
	return TraceContext{TraceID: hex.EncodeToString(b[:16]), SpanID: hex.EncodeToString(b[16:])}
}
func WithTraceContext(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, telemetryContextKey{}, tc)
}
func TraceContextFromContext(ctx context.Context) TraceContext {
	if ctx != nil {
		if tc, ok := ctx.Value(telemetryContextKey{}).(TraceContext); ok {
			return tc
		}
	}
	return TraceContext{}
}
func TraceIDFromContext(ctx context.Context) string { return TraceContextFromContext(ctx).TraceID }
func SpanIDFromContext(ctx context.Context) string  { return TraceContextFromContext(ctx).SpanID }
func StartTrace(ctx context.Context) (context.Context, TraceContext) {
	tc := NewTraceContext()
	return WithTraceContext(ctx, tc), tc
}

// StartSpan creates a child span while preserving the current trace ID.
func StartSpan(ctx context.Context) (context.Context, TraceContext) {
	parent := TraceContextFromContext(ctx)
	child := NewTraceContext()
	if parent.TraceID != "" {
		child.TraceID = parent.TraceID
	}
	return WithTraceContext(ctx, child), child
}

// RecordTelemetry records an event through the process-wide telemetry sink.
func RecordTelemetry(ctx context.Context, event string, properties map[string]any) {
	if globalTelemetry != nil {
		globalTelemetry.Record(ctx, event, properties)
	}
}

// RecordLocalTelemetry records an event in the bounded local journal only.
func RecordLocalTelemetry(ctx context.Context, event string, properties map[string]any) {
	t := globalTelemetry
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.optedOut {
		return
	}
	tc := TraceContextFromContext(ctx)
	redacted := make(map[string]any, len(properties)+5)
	for key, value := range properties {
		redacted[key] = redactMetadataText(fmt.Sprint(value))
	}
	redacted["trace_id"] = redactMetadataText(tc.TraceID)
	redacted["span_id"] = redactMetadataText(tc.SpanID)
	redacted["time"] = time.Now().UTC()
	redacted["delivery_failures"] = t.deliveryFailures.Load()
	if t.journal != nil {
		t.journal.append(map[string]any{"event": event, "properties": redacted})
	}
	t.addRecentLocked(event, tc)
}
