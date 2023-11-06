package sample

import (
	"testing"

	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/logger"
	"github.com/opsramp/tracing-proxy/metrics"
	"github.com/opsramp/tracing-proxy/types"

	"github.com/stretchr/testify/assert"
)

func TestDynamicAddSampleRateKeyToTrace(t *testing.T) {
	const spanCount = 5

	var mockMetrics metrics.MockMetrics
	mockMetrics.Start()

	sampler := &DynamicSampler{
		Config: &config.DynamicSamplerConfig{
			FieldList:                    []string{"http.status_code"},
			AddSampleRateKeyToTrace:      true,
			AddSampleRateKeyToTraceField: "meta.key",
		},
		Logger:  &logger.NullLogger{},
		Metrics: &mockMetrics,
	}

	trace := &types.Trace{}
	for i := 0; i < spanCount; i++ {
		trace.AddSpan(&types.Span{
			Event: types.Event{
				Data: map[string]interface{}{
					"http.status_code": "200",
				},
			},
		})
	}
	if err := sampler.Start(); err != nil {
		t.Error(err)
	}
	sampler.GetSampleRate(trace) // nolint:all

	spans := trace.GetSpans()
	assert.Len(t, spans, spanCount, "should have the same number of spans as input")
	for _, span := range spans {
		assert.Equal(t, span.Event.Data, map[string]interface{}{
			"http.status_code": "200",
			"meta.key":         "200•,",
		}, "should add the sampling key to all spans in the trace")
	}
}
