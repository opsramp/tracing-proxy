package types

import (
	"context"
	"time"
)

const (
	APIKeyHeader = "X-OpsRamp-Team"

	SampleRateHeader = "X-OpsRamp-Samplerate"
	TimestampHeader  = "X-OpsRamp-Event-Time"
	QueryTokenHeader = "X-OpsRamp-Refinery-Query"
)

// used to put a request ID into the request context for logging
type RequestIDContextKey struct{}

// event is not part of a trace - it's an event that showed up with no trace ID
type Event struct {
	Context     context.Context
	APIHost     string
	APIKey      string
	APIToken    string
	APITenantId string
	Dataset     string
	Environment string
	SampleRate  uint
	Timestamp   time.Time
	Data        map[string]interface{}
}

// Trace isn't something that shows up on the wire; it gets created within
// tracing-proxy. Traces are not thread-safe; only one goroutine should be working
// with a trace object at a time.
type Trace struct {
	APIHost string
	APIKey  string
	Dataset string
	TraceID string

	// SampleRate should only be changed if the changer holds the SendSampleLock
	SampleRate uint
	// KeepSample should only be changed if the changer holds the SendSampleLock
	KeepSample bool
	// Sent should only be changed if the changer holds the SendSampleLock
	Sent bool

	SendBy time.Time

	// StartTime is the server time when the first span arrived for this trace.
	// Used to calculate how long traces spend sitting in tracing-proxy
	StartTime time.Time

	// ArrivalTime is the server time when the first span arrived for this trace.
	// Used to calculate how long traces spend sitting in Refinery
	ArrivalTime time.Time

	RootSpan *Span

	// DataSize is the sum of the DataSize of spans that are added.
	// It's used to help expire the most expensive traces.
	DataSize int

	// spans is the list of spans in this trace
	spans []*Span

	// totalImpact is the sum of the trace's cacheImpact; if this value is 0
	// it is recalculated during CacheImpact(), otherwise this value is
	// returned. We reset it to 0 when adding spans so it gets recalculated.
	// This is used to memoize the impact calculation so that it doesn't get
	// calculated over and over during a sort.
	totalImpact int
}

// AddSpan adds a span to this trace
func (t *Trace) AddSpan(sp *Span) {
	// We've done all the work to know this is a trace we are putting in our cache, so
	// now is when we can calculate the size of it so that our cache size management
	// code works properly.
	sp.ArrivalTime = time.Now()
	sp.DataSize = sp.GetDataSize()
	t.DataSize += sp.DataSize
	t.spans = append(t.spans, sp)
	t.totalImpact = 0
}

// CacheImpact calculates an abstract value for something we're calling cache impact, which is
// the sum of the CacheImpact of all of the spans in a trace. We use it to order traces
// so we can eject the ones that having the most impact on the cache size, but balancing that
// against preferring to keep newer spans.
func (t *Trace) CacheImpact(traceTimeout time.Duration) int {
	if t.totalImpact == 0 {
		for _, sp := range t.GetSpans() {
			t.totalImpact += sp.CacheImpact(traceTimeout)
		}
	}
	return t.totalImpact
}

// GetSpans returns the list of descendants in this trace
func (t *Trace) GetSpans() []*Span {
	return t.spans
}

// DescendantCount gets the number of descendants of all kinds currently in this trace
func (t *Trace) DescendantCount() uint {
	return uint(len(t.spans))
}

func (t *Trace) GetSamplerKey() (string, bool) {
	if IsLegacyAPIKey(t.APIKey) {
		return t.Dataset, true
	}

	env := ""
	for _, sp := range t.GetSpans() {
		if sp.Event.Environment != "" {
			env = sp.Event.Environment
			break
		}
	}

	return env, false
}

// Span is an event that shows up with a trace ID, so will be part of a Trace
type Span struct {
	Event
	TraceID     string
	DataSize    int
	ArrivalTime time.Time
}

// GetDataSize computes the size of the Data element of the Span.
// Note that it's not the full size of the span, but we're mainly using this for
// relative ordering, not absolute calculations.
func (sp *Span) GetDataSize() int {
	total := 0
	// the data types we should be getting from JSON are:
	// float64, int64, bool, string
	for _, v := range sp.Data {
		switch v.(type) {
		case bool:
			total += 1
		case float64, int64, int:
			total += 8
		case string, []byte:
			total += len(v.(string))
		default:
			total += 8 // catchall
		}
	}
	return total
}

// cacheImpactFactor controls how much more we weigh older spans compared to newer ones;
// setting this to 1 means they're not weighted by duration
const cacheImpactFactor = 4

// CacheImpact calculates an abstract value for something we're calling cache impact, which is
// the product of the size of the span and a factor related to the amount of time the span
// has been stored in the cache, based on the TraceTimeout value.
func (sp *Span) CacheImpact(traceTimeout time.Duration) int {
	// multiplier will be a value from 1-cacheImpactFactor, depending on how long the
	// span has been in the cache compared to the traceTimeout. It might go higher
	// during the brief period between traceTimeout and the time when the span is sent.
	multiplier := int(cacheImpactFactor*time.Since(sp.ArrivalTime)/traceTimeout) + 1
	// We can assume DataSize was set when the span was added.
	return multiplier * sp.DataSize
}

func IsLegacyAPIKey(apiKey string) bool {
	return len(apiKey) == 32
}
