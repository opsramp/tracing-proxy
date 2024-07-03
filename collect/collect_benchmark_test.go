package collect

import (
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/opsramp/tracing-proxy/collect/cache"
	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/logger"
	"github.com/opsramp/tracing-proxy/metrics"
	"github.com/opsramp/tracing-proxy/sample"
	"github.com/opsramp/tracing-proxy/transmit"
	"github.com/opsramp/tracing-proxy/types"
)

func BenchmarkCollect(b *testing.B) {
	transmission := &transmit.MockTransmission{}
	err := transmission.Start()
	if err != nil {
		b.Error(err)
	}
	conf := &config.MockConfig{
		GetSendDelayVal:    0,
		GetTraceTimeoutVal: 60 * time.Second,
		GetSamplerTypeVal:  &config.DeterministicSamplerConfig{},
		SendTickerVal:      2 * time.Millisecond,
	}

	log := &logger.LogrusLogger{}
	err = log.SetLevel("warn")
	if err != nil {
		b.Error(err)
	}
	err = log.Start()
	if err != nil {
		b.Error(err)
	}

	metric := &metrics.MockMetrics{}
	metric.Start()

	stc, err := cache.NewLegacySentCache(15)
	assert.NoError(b, err, "lru cache should start")

	coll := &InMemCollector{
		Config:       conf,
		Logger:       log,
		Transmission: transmission,
		Metrics:      metric,
		SamplerFactory: &sample.SamplerFactory{
			Config: conf,
			Logger: log,
		},
		BlockOnAddSpan:   true,
		cache:            cache.NewInMemCache(3, metric, log),
		incoming:         make(chan *types.Span, 500), // nolint:all
		fromPeer:         make(chan *types.Span, 500), // nolint:all
		datasetSamplers:  make(map[string]sample.Sampler),
		sampleTraceCache: stc,
	}
	go coll.collect()

	// wait until we get n number of spans out the other side
	wait := func(n int) {
		for {
			transmission.Mux.RLock()
			count := len(transmission.Events)
			transmission.Mux.RUnlock()

			if count >= n {
				break
			}
			time.Sleep(100 * time.Microsecond)
		}
	}

	b.Run("AddSpan", func(b *testing.B) {
		transmission.Flush()
		for n := 0; n < b.N; n++ {
			span := &types.Span{
				TraceID: strconv.Itoa(rand.Int()), // #nosec G404
				Event: types.Event{
					Dataset: "aoeu",
				},
			}
			err := coll.AddSpan(span)
			if err != nil {
				b.Error(err)
			}
		}
		wait(b.N)
	})

	b.Run("AddSpanFromPeer", func(b *testing.B) {
		transmission.Flush()
		for n := 0; n < b.N; n++ {
			span := &types.Span{
				TraceID: strconv.Itoa(rand.Int()), // #nosec 404
				Event: types.Event{
					Dataset: "aoeu",
				},
			}
			err := coll.AddSpanFromPeer(span)
			if err != nil {
				b.Error(err)
			}
		}
		wait(b.N)
	})
}
