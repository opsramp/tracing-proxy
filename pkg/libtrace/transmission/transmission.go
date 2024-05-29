package transmission

// txClient handles the transmission of events to Opsramp.
//
// Overview
//
// Create a new instance of Client.
// Set any of the public fields for which you want to override the defaults.
// Call Start() to spin up the background goroutines necessary for transmission
// Call Add(Event) to queue an event for transmission
// Ensure Stop() is called to flush all in-flight messages.

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/opsramp/tracing-proxy/logger"
	"github.com/opsramp/tracing-proxy/pkg/retry"
	"github.com/opsramp/tracing-proxy/proxy"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	v1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/facebookgo/muster"
	"github.com/opsramp/tracing-proxy/pkg/libtrace/proto/proxypb"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	v11 "go.opentelemetry.io/proto/otlp/common/v1"
)

const (
	maxOverflowBatches int = 10
	// Default start-to-finish timeout for batch to send HTTP requests.
	defaultSendTimeout = time.Second * 60
	// Maximum BatchSize for a api payload
	maxApiSize int = 1000000
)

type TraceProxy struct {
	// How many events to collect into a batch before sending. A
	// batch could be sent before achieving this item limit if the
	// BatchTimeout has elapsed since the last batch is sent. If set
	// to zero, batches will only be sent upon reaching the
	// BatchTimeout. It is an error for both this and
	// the BatchTimeout to be zero.
	// Default: 50 (from Config.MaxBatchSize)
	MaxBatchSize uint

	// How often to send batches. Events queue up into a batch until
	// this time has elapsed or the batch item limit is reached
	// (MaxBatchSize), then the batch is sent to Honeycomb API.
	// If set to zero, batches will only be sent upon reaching the
	// MaxBatchSize item limit. It is an error for both this and
	// the MaxBatchSize to be zero.
	// Default: 100 milliseconds (from Config.SendFrequency)
	BatchTimeout time.Duration

	// The start-to-finish timeout for HTTP requests sending event
	// batches to the Honeycomb API. Transmission will retry once
	// when receiving a timeout, so total time spent attempting to
	// send events could be twice this value.
	// Default: 60 seconds.
	BatchSendTimeout time.Duration

	// number of batches that can be inflight simultaneously
	MaxConcurrentBatches uint

	// how many events to allow to pile up
	// if not specified, then the work channel becomes blocking
	// and attempting to add an event to the queue can fail
	PendingWorkCapacity uint

	// whether to block or drop events when the queue fills
	BlockOnSend bool

	// whether to block or drop responses when the queue fills
	BlockOnResponse bool

	UserAgentAddition string

	// toggles compression when sending batches of events
	DisableCompression bool

	// Deprecated, synonymous with DisableCompression
	DisableGzipCompression bool // nolint:all

	// set true to send events with msgpack encoding
	EnableMsgpackEncoding bool

	batchMaker func() muster.Batch
	responses  chan Response

	muster     *muster.Client
	musterLock sync.RWMutex

	Logger  logger.Logger
	Metrics Metrics

	UseTls         bool
	UseTlsInsecure bool

	IsPeer bool

	TraceEndpoint string
	LogsEndpoint  string
	SendEvents    bool
	Proxy         *proxy.Proxy

	TenantId          string
	Dataset           string
	AuthTokenEndpoint string
	AuthTokenKey      string
	AuthTokenSecret   string
	RetrySettings     *retry.Config

	defaultAuth *Auth
}

var (
	SendTraces bool
	SendEvents bool
)
var m sync.Mutex

func (h *TraceProxy) Start() error {
	if h.Logger == nil {
		h.Logger = &logger.NullLogger{}
	}
	if h.TenantId == "" {
		return fmt.Errorf("tenantId cant be empty")
	}

	// Set Events and Traces send flags
	m.Lock() //nolint:all
	SendEvents = h.SendEvents
	SendTraces = true
	m.Unlock()

	// populate auth token
	if h.defaultAuth == nil {
		auth, err := CreateNewAuth(
			h.AuthTokenEndpoint,
			h.AuthTokenKey,
			h.AuthTokenSecret,
			time.Minute*4,
			h.RetrySettings,
			time.Minute*5,
			h.Proxy,
		)
		if err != nil {
			return err
		}
		h.defaultAuth = auth
		_ = h.defaultAuth.Start()
		_, _ = h.defaultAuth.Renew()
	}

	// establish initial connection
	var opts []grpc.DialOption
	if h.UseTls {
		tlsCfg := &tls.Config{InsecureSkipVerify: h.UseTlsInsecure}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	opts = append(opts, grpc.WithUnaryInterceptor(h.defaultAuth.UnaryClientInterceptor))

	apiHostURL, err := url.Parse(h.TraceEndpoint)
	if err != nil {
		return err
	}
	apiPort := apiHostURL.Port()
	if apiPort == "" {
		apiPort = "443"
	}
	tAddr := fmt.Sprintf("%s:%s", apiHostURL.Hostname(), apiPort)

	logsApiHostUrl, err := url.Parse(h.LogsEndpoint)
	if err != nil {
		return err
	}
	logsApiPort := logsApiHostUrl.Port()
	if logsApiPort == "" {
		logsApiPort = "443"
	}
	lAddr := fmt.Sprintf("%s:%s", logsApiHostUrl.Hostname(), logsApiPort)

	connObj, err := NewConnection(ConnConfig{
		Proxy: h.Proxy,
		TAddr: tAddr,
		TOpts: opts,
		LAddr: lAddr,
		LOpts: opts,
	})
	if err != nil {
		return err
	}
	h.defaultAuth.conn = connObj

	h.responses = make(chan Response, h.PendingWorkCapacity*2)
	if h.Metrics == nil {
		h.Metrics = &nullMetrics{}
	}
	if h.BatchSendTimeout == 0 {
		h.BatchSendTimeout = defaultSendTimeout
	}
	if h.batchMaker == nil {
		h.batchMaker = func() muster.Batch {
			return &batchAgg{
				userAgentAddition:     h.UserAgentAddition,
				batches:               map[string][]*Event{},
				conn:                  h.defaultAuth.conn,
				blockOnResponse:       h.BlockOnResponse,
				responses:             h.responses,
				metrics:               h.Metrics,
				disableCompression:    h.DisableGzipCompression || h.DisableCompression,
				enableMsgpackEncoding: h.EnableMsgpackEncoding,
				logger:                h.Logger,
				tenantId:              h.TenantId,
				dataset:               h.Dataset,
				isPeer:                h.IsPeer,
			}
		}
	}
	m := h.createMuster()
	h.muster = m
	return h.muster.Start()
}

func (h *TraceProxy) createMuster() *muster.Client {
	m := new(muster.Client)
	m.MaxBatchSize = h.MaxBatchSize
	m.BatchTimeout = h.BatchTimeout
	m.MaxConcurrentBatches = h.MaxConcurrentBatches
	m.PendingWorkCapacity = h.PendingWorkCapacity
	m.BatchMaker = h.batchMaker
	return m
}

func (h *TraceProxy) Stop() error {
	h.Logger.Info().Logf("TraceProxy transmission stopping")
	err := h.muster.Stop()
	if h.responses != nil {
		close(h.responses)
	}
	if h.defaultAuth != nil {
		h.defaultAuth.Stop()
	}

	return err
}

func (h *TraceProxy) Flush() (err error) {
	// There isn't a way to flush a muster.Client directly, so we have to stop
	// the old one (which has a side effect of flushing the data) and make a new
	// one. We start the new one and swap it with the old one so that we minimize
	// the time we hold the musterLock for.
	newMuster := h.createMuster()
	err = newMuster.Start()
	if err != nil {
		return err
	}
	h.musterLock.Lock() //nolint:all
	m := h.muster
	h.muster = newMuster
	h.musterLock.Unlock()
	return m.Stop()
}

// Add enqueues ev to be sent. If a Flush is in-progress, this will block until
// it completes. Similarly, if BlockOnSend is set and the pending work is more
// than the PendingWorkCapacity, this will block a Flush until more pending
// work can be enqueued.
func (h *TraceProxy) Add(ev *Event) {
	if h.tryAdd(ev) {
		h.Metrics.Increment("messages_queued")
		return
	}
	h.Metrics.Increment("queue_overflow")
	r := Response{
		Err:      errors.New("queue overflow"),
		Metadata: ev.Metadata,
	}
	h.Logger.Debug().Logf("got response code %d, error %s, and body %s",
		r.StatusCode, r.Err, string(r.Body))
	writeToResponse(h.responses, r, h.BlockOnResponse)
}

// tryAdd attempts to add ev to the underlying muster. It returns false if this
// was unsuccessful because the muster queue (muster.Work) is full.
func (h *TraceProxy) tryAdd(ev *Event) bool {
	h.musterLock.RLock()
	defer h.musterLock.RUnlock()

	// Even though this queue is locked against changing h.Muster, the Work queue length
	// could change due to actions on the worker side, so make sure we only measure it once.
	qLen := len(h.muster.Work)
	h.Logger.Debug().Logf("adding event to transmission; queue length %d", qLen)
	h.Metrics.Gauge("queue_length", qLen)

	if h.BlockOnSend {
		h.muster.Work <- ev
		return true
	} else {
		select {
		case h.muster.Work <- ev:
			return true
		default:
			return false
		}
	}
}

func (h *TraceProxy) TxResponses() chan Response {
	return h.responses
}

func (h *TraceProxy) SendResponse(r Response) bool {
	if h.BlockOnResponse {
		h.responses <- r
	} else {
		select {
		case h.responses <- r:
		default:
			return true
		}
	}
	return false
}

// batchAgg is a batch aggregator - it's actually collecting what will
// eventually be one or more batches sent to the /1/batch/dataset endpoint.
type batchAgg struct {
	// map of batch keys to a list of events destined for that batch
	batches map[string][]*Event
	// Used to re-enqueue events when an initial batch is too large
	overflowBatches       map[string][]*Event
	blockOnResponse       bool
	userAgentAddition     string
	disableCompression    bool
	enableMsgpackEncoding bool

	responses chan Response
	// numEncoded int

	metrics Metrics

	testBlocker *sync.WaitGroup

	logger logger.Logger

	dataset string
	isPeer  bool

	tenantId string
	conn     *Connection
}

// batch is a collection of events that will all be POSTed as one HTTP call
// type batch []*Event

func (b *batchAgg) Add(ev interface{}) {
	// from muster godoc: "The Batch does not need to be safe for concurrent
	// access; the Client will handle synchronization."
	if b.batches == nil {
		b.batches = map[string][]*Event{}
	}
	e, _ := ev.(*Event)
	// collect separate buckets of events to send based on apiHost and dataset
	key := fmt.Sprintf("%s_%s", e.APIHost, e.Dataset)
	b.batches[key] = append(b.batches[key], e)
}

func (b *batchAgg) enqueueResponse(resp Response) {
	if writeToResponse(b.responses, resp, b.blockOnResponse) {
		if b.testBlocker != nil {
			b.testBlocker.Done()
		}
	}
}

func (b *batchAgg) reEnqueueEvents(events []*Event) {
	if b.overflowBatches == nil {
		b.overflowBatches = make(map[string][]*Event)
	}
	for _, e := range events {
		key := fmt.Sprintf("%s_%s", e.APIHost, e.Dataset)
		b.overflowBatches[key] = append(b.overflowBatches[key], e)
	}
}

func (b *batchAgg) Fire(notifier muster.Notifier) {
	defer notifier.Done()

	// send each batchKey's collection of events as a POST to /1/batch/<dataset>
	// we don't need the batch key anymore; it's done its sorting job
	for _, events := range b.batches {
		// b.fireBatch(events)
		// b.exportBatch(events)
		b.exportProtoMsgBatch(events)
	}
	// The initial batches could have had payloads that were greater than 5MB.
	// The remaining events will have overflowed into overflowBatches
	// Process these until complete. Overflow batches can also overflow, so we
	// have to prepare to process it multiple times
	overflowCount := 0
	if b.overflowBatches != nil {
		for len(b.overflowBatches) > 0 {
			// We really shouldn't get here but defensively avoid an endless
			// loop of re-enqueued events
			if overflowCount > maxOverflowBatches {
				break
			}
			overflowCount++
			// fetch the keys in this map - we can't range over the map
			// because it's possible that fireBatch will re-enqueue more overflow
			// events
			keys := make([]string, len(b.overflowBatches))
			i := 0
			for k := range b.overflowBatches {
				keys[i] = k
				i++
			}

			for _, k := range keys {
				events := b.overflowBatches[k]
				// fireBatch may append more overflow events,
				// so we want to clear this key before firing the batch
				delete(b.overflowBatches, k)
				// b.fireBatch(events)
				// b.exportBatch(events)
				b.exportProtoMsgBatch(events)
			}
		}
	}
}

func (b *batchAgg) exportProtoMsgBatch(events []*Event) {
	if len(events) == 0 {
		// we managed to create a batch with no events. 🤔️ that's odd, let's move on.
		return
	}

	logTraceReq := proxypb.ExportLogTraceProxyServiceRequest{
		TenantId: b.tenantId,
	}

	traceReq := &proxypb.ExportTraceProxyServiceRequest{
		TenantId: b.tenantId,
	}

	var apiHost, appName string
	var resourceLogs []*v1.ResourceLogs
	var LogRecords []*v1.LogRecord
	for _, ev := range events {
		if ev == nil {
			continue
		}
		apiHost = ev.APIHost

		traceData := proxypb.ProxySpan{
			Data:      &proxypb.Data{},
			Timestamp: ev.Timestamp.Format(time.RFC3339Nano),
		}

		traceData.Data.TraceTraceID, _ = ev.Data["traceTraceID"].(string)
		traceData.Data.TraceParentID, _ = ev.Data["traceParentID"].(string)
		traceData.Data.TraceSpanID, _ = ev.Data["traceSpanID"].(string)
		traceData.Data.TraceLinkTraceID, _ = ev.Data["traceLinkTraceID"].(string)
		traceData.Data.TraceLinkSpanID, _ = ev.Data["traceLinkSpanID"].(string)
		traceData.Data.Type, _ = ev.Data["type"].(string)
		traceData.Data.MetaType, _ = ev.Data["metaType"].(string)
		traceData.Data.SpanName, _ = ev.Data["spanName"].(string)
		traceData.Data.SpanKind, _ = ev.Data["spanKind"].(string)
		traceData.Data.SpanNumEvents, _ = ev.Data["spanNumEvents"].(int64)
		traceData.Data.SpanNumLinks, _ = ev.Data["spanNumLinks"].(int64)
		traceData.Data.StatusCode, _ = ev.Data["statusCode"].(int64)
		traceData.Data.StatusMessage, _ = ev.Data["statusMessage"].(string)
		traceData.Data.Time, _ = ev.Data["time"].(int64)
		traceData.Data.DurationMs, _ = ev.Data["durationMs"].(float64)
		traceData.Data.StartTime, _ = ev.Data["startTime"].(int64)
		traceData.Data.EndTime, _ = ev.Data["endTime"].(int64)
		traceData.Data.Error, _ = ev.Data["error"].(bool)
		traceData.Data.FromProxy, _ = ev.Data["fromProxy"].(bool)
		traceData.Data.ParentName, _ = ev.Data["parentName"].(string)

		resourceAttr, _ := ev.Data["resourceAttributes"].(map[string]interface{})

		for key, val := range resourceAttr {
			var resourceAttrKeyVal proxypb.KeyValue
			resourceAttrKeyVal.Key = key

			switch v := val.(type) {
			case nil:
				b.logger.Debug().Logf("resource attribute value is nil for key: %v", key) // here v has type interface{}
			case string:
				resourceAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_StringValue{StringValue: v}} // here v has type int
			case bool:
				resourceAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_BoolValue{BoolValue: v}} // here v has type interface{}
			case int64:
				resourceAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_IntValue{IntValue: v}} // here v has type interface{}
			default:
				b.logger.Debug().Logf("resource attribute type unknown: %v", v) // here v has type interface{}
			}

			if key == "app" {
				appName = fmt.Sprintf("%s", val)
			}
			traceData.Data.ResourceAttributes = append(traceData.Data.ResourceAttributes, &resourceAttrKeyVal)
		}
		spanAttr, _ := ev.Data["spanAttributes"].(map[string]interface{})
		for key, val := range spanAttr {
			var spanAttrKeyVal proxypb.KeyValue
			spanAttrKeyVal.Key = key

			switch v := val.(type) {
			case nil:
				b.logger.Debug().Logf("span attribute value is nil for key: %v", key) // here v has type interface{}
			case string:
				spanAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_StringValue{StringValue: v}} // here v has type int
			case bool:
				spanAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_BoolValue{BoolValue: v}} // here v has type interface{}
			case int64:
				spanAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_IntValue{IntValue: v}} // here v has type interface{}
			default:
				b.logger.Debug().Logf("span attribute type unknown: %v", v) // here v has type interface{}
			}

			traceData.Data.SpanAttributes = append(traceData.Data.SpanAttributes, &spanAttrKeyVal)
		}
		traceData.Data.SpanAttributes = append(traceData.Data.SpanAttributes, &proxypb.KeyValue{
			Key:   "kind",
			Value: &proxypb.AnyValue{Value: &proxypb.AnyValue_StringValue{StringValue: traceData.Data.SpanKind}},
		})

		eventAttr, _ := ev.Data["eventAttributes"].(map[string]interface{})
		for key, val := range eventAttr {
			var eventAttrKeyVal proxypb.KeyValue
			eventAttrKeyVal.Key = key

			switch v := val.(type) {
			case nil:
				b.logger.Debug().Logf("event attribute value is nil for key: %v", key) // here v has type interface{}
			case string:
				eventAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_StringValue{StringValue: v}} // here v has type int
			case bool:
				eventAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_BoolValue{BoolValue: v}} // here v has type interface{}
			case int64:
				eventAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_IntValue{IntValue: v}} // here v has type interface{}
			default:
				b.logger.Debug().Logf("event attribute type unknown: %v", v) // here v has type interface{}
			}

			traceData.Data.EventAttributes = append(traceData.Data.EventAttributes, &eventAttrKeyVal)
		}

		traceReq.Items = append(traceReq.Items, &traceData)

		for _, event := range ev.SpanEvents {
			var recordAttributes []*v11.KeyValue
			for key, val := range event.Attributes {
				spanEventAttrKeyVal := &v11.KeyValue{}
				spanEventAttrKeyVal.Key = key
				switch v := val.(type) {
				case nil:
					b.logger.Debug().Logf("event attribute value is nil for key: %v", key) // here v has type interface{}
				case string:
					spanEventAttrKeyVal.Value = &v11.AnyValue{Value: &v11.AnyValue_StringValue{StringValue: v}} // here v has type string
				case bool:
					spanEventAttrKeyVal.Value = &v11.AnyValue{Value: &v11.AnyValue_BoolValue{BoolValue: v}} // here v has type interface{}
				case int64:
					spanEventAttrKeyVal.Value = &v11.AnyValue{Value: &v11.AnyValue_IntValue{IntValue: v}} // here v has type interface{}
				default:
					b.logger.Debug().Logf("event attribute type unknown: %v", v) // here v has type interface{}
				}
				recordAttributes = append(recordAttributes, spanEventAttrKeyVal)
			}
			appNameRes := &v11.KeyValue{
				Key:   "trace_app",
				Value: &v11.AnyValue{Value: &v11.AnyValue_StringValue{StringValue: appName}},
			}
			recordAttributes = append(recordAttributes, appNameRes)
			LogRecord := &v1.LogRecord{
				TimeUnixNano:           event.Timestamp,
				ObservedTimeUnixNano:   0,
				SeverityNumber:         0,
				SeverityText:           "",
				Body:                   &v11.AnyValue{Value: &v11.AnyValue_StringValue{StringValue: event.Name}},
				Attributes:             recordAttributes,
				DroppedAttributesCount: 0,
				Flags:                  0,
				TraceId:                nil,
				SpanId:                 nil,
			}
			LogRecords = append(LogRecords, LogRecord)
		}
		logTraceData := proxypb.ProxyLogSpan{
			Data:      &proxypb.LogTraceData{},
			Timestamp: ev.Timestamp.Format(time.RFC3339Nano),
		}

		logTraceData.Data.TraceTraceID, _ = ev.Data["traceTraceID"].(string)
		logTraceData.Data.TraceParentID, _ = ev.Data["traceParentID"].(string)
		logTraceData.Data.TraceSpanID, _ = ev.Data["traceSpanID"].(string)
		logTraceData.Data.TraceLinkTraceID, _ = ev.Data["traceLinkTraceID"].(string)
		logTraceData.Data.TraceLinkSpanID, _ = ev.Data["traceLinkSpanID"].(string)
		logTraceData.Data.Type, _ = ev.Data["type"].(string)
		logTraceData.Data.MetaType, _ = ev.Data["metaType"].(string)
		logTraceData.Data.SpanName, _ = ev.Data["spanName"].(string)
		logTraceData.Data.SpanKind, _ = ev.Data["spanKind"].(string)
		logTraceData.Data.SpanNumEvents, _ = ev.Data["spanNumEvents"].(int64)
		logTraceData.Data.SpanNumLinks, _ = ev.Data["spanNumLinks"].(int64)
		logTraceData.Data.StatusCode, _ = ev.Data["statusCode"].(int64)
		logTraceData.Data.StatusMessage, _ = ev.Data["statusMessage"].(string)
		logTraceData.Data.Time, _ = ev.Data["time"].(int64)
		logTraceData.Data.DurationMs, _ = ev.Data["durationMs"].(float64)
		logTraceData.Data.StartTime, _ = ev.Data["startTime"].(int64)
		logTraceData.Data.EndTime, _ = ev.Data["endTime"].(int64)
		logTraceData.Data.Error, _ = ev.Data["error"].(bool)
		logTraceData.Data.FromProxy, _ = ev.Data["fromProxy"].(bool)
		logTraceData.Data.ParentName, _ = ev.Data["parentName"].(string)

		logTraceResourceAttr, _ := ev.Data["resourceAttributes"].(map[string]interface{})

		for key, val := range logTraceResourceAttr {
			var resourceAttrKeyVal proxypb.KeyValue
			resourceAttrKeyVal.Key = key

			switch v := val.(type) {
			case nil:
				b.logger.Debug().Logf("resource attribute value is nil for key: %v", key) // here v has type interface{}
			case string:
				resourceAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_StringValue{StringValue: v}} // here v has type int
			case bool:
				resourceAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_BoolValue{BoolValue: v}} // here v has type interface{}
			case int64:
				resourceAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_IntValue{IntValue: v}} // here v has type interface{}
			default:
				b.logger.Debug().Logf("resource attribute type unknown: %v", v) // here v has type interface{}
			}
			logTraceData.Data.ResourceAttributes = append(logTraceData.Data.ResourceAttributes, &resourceAttrKeyVal)
		}
		logTraceSpanAttr, _ := ev.Data["spanAttributes"].(map[string]interface{})
		for key, val := range logTraceSpanAttr {
			var spanAttrKeyVal proxypb.KeyValue
			spanAttrKeyVal.Key = key

			switch v := val.(type) {
			case nil:
				b.logger.Debug().Logf("span attribute value is nil for key: %v", key) // here v has type interface{}
			case string:
				spanAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_StringValue{StringValue: v}} // here v has type int
			case bool:
				spanAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_BoolValue{BoolValue: v}} // here v has type interface{}
			case int64:
				spanAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_IntValue{IntValue: v}} // here v has type interface{}
			default:
				b.logger.Debug().Logf("span attribute type unknown: %v", v) // here v has type interface{}
			}

			logTraceData.Data.SpanAttributes = append(logTraceData.Data.SpanAttributes, &spanAttrKeyVal)
		}
		logTraceData.Data.SpanAttributes = append(logTraceData.Data.SpanAttributes, &proxypb.KeyValue{
			Key:   "kind",
			Value: &proxypb.AnyValue{Value: &proxypb.AnyValue_StringValue{StringValue: logTraceData.Data.SpanKind}},
		})

		logTraceEventAttr, _ := ev.Data["eventAttributes"].(map[string]interface{})
		for key, val := range logTraceEventAttr {
			var eventAttrKeyVal proxypb.KeyValue
			eventAttrKeyVal.Key = key

			switch v := val.(type) {
			case nil:
				b.logger.Debug().Logf("event attribute value is nil for key: %v", key) // here v has type interface{}
			case string:
				eventAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_StringValue{StringValue: v}} // here v has type int
			case bool:
				eventAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_BoolValue{BoolValue: v}} // here v has type interface{}
			case int64:
				eventAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_IntValue{IntValue: v}} // here v has type interface{}
			default:
				b.logger.Debug().Logf("event attribute type unknown: %v", v) // here v has type interface{}
			}

			logTraceData.Data.EventAttributes = append(logTraceData.Data.EventAttributes, &eventAttrKeyVal)
		}

		for _, spanEvent := range ev.SpanEvents {
			var proxySpanEvent proxypb.SpanEvent
			proxySpanEvent.Name = spanEvent.Name
			proxySpanEvent.TimeStamp = spanEvent.Timestamp
			for key, val := range spanEvent.Attributes {
				spanEventAttrKeyVal := &proxypb.KeyValue{}
				spanEventAttrKeyVal.Key = key
				switch v := val.(type) {
				case nil:
					b.logger.Debug().Logf("event attribute value is nil for key: %v", key) // here v has type interface{}
				case string:
					spanEventAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_StringValue{StringValue: v}} // here v has type string
				case bool:
					spanEventAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_BoolValue{BoolValue: v}} // here v has type interface{}
				case int64:
					spanEventAttrKeyVal.Value = &proxypb.AnyValue{Value: &proxypb.AnyValue_IntValue{IntValue: v}} // here v has type interface{}
				default:
					b.logger.Debug().Logf("event attribute type unknown: %v", v) // here v has type interface{}
				}
				proxySpanEvent.SpanEventAttributes = append(proxySpanEvent.SpanEventAttributes, spanEventAttrKeyVal)
			}
			logTraceData.Data.SpanEvents = append(logTraceData.Data.SpanEvents, &proxySpanEvent)
		}
		logTraceReq.Items = append(logTraceReq.Items, &logTraceData)
	}

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.New(map[string]string{
		"tenantId": b.tenantId,
		"dataset":  b.dataset,
	}))

	var sendDirect bool

	if b.isPeer {
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}

		apiHostURL, err := url.Parse(apiHost)
		if err != nil {
			sendDirect = true
			b.logger.Error().Logf("sending directly, unable to parse peer url: %v", err)
		}

		apiPort := apiHostURL.Port()
		if apiPort == "" {
			apiPort = "80"
		}
		apiHost = fmt.Sprintf("%s:%s", apiHostURL.Hostname(), apiPort)

		peerConn, err := grpc.NewClient(apiHost, opts...)
		if err != nil {
			sendDirect = true
			b.logger.Error().Logf("sending directly, unable to establish connection to %s error: %v", apiHost, err)
		}

		if !sendDirect {
			peerClient := proxypb.NewTraceProxyServiceClient(peerConn)
			logTraceBatches := b.SendlogTraceBatches(&logTraceReq)
			for _, record := range logTraceBatches {
				logTraceReq := &proxypb.ExportLogTraceProxyServiceRequest{
					Items:    record,
					TenantId: b.tenantId,
				}
				r, err := peerClient.ExportLogTraceProxy(ctx, logTraceReq)
				if st, ok := status.FromError(err); ok {
					if st.Code() != codes.OK {
						b.logger.Error().Logf("sending failed. error: %s", st.String())
						b.metrics.Increment("send_errors")
					} else {
						b.metrics.Increment("batches_sent")
					}
				}

				b.logger.Debug().Logf("trace proxy response msg from peer: %s", r.GetMessage())
				b.logger.Debug().Logf("trace proxy response status from peer: %s", r.GetStatus())
			}

			return
		}
	}

	if sendDirect || !b.isPeer {
		if SendTraces {
			traceBatches := b.SendTraceBatches(traceReq)

			for _, batch := range traceBatches {
				req := &proxypb.ExportTraceProxyServiceRequest{
					Items:    batch,
					TenantId: b.tenantId,
				}
				b.ExportTraces(req, ctx)
			}
		}

		if len(LogRecords) > 0 {
			logBatches := b.sendLogBatches(LogRecords)

			for _, batch := range logBatches {

				var scopeLogs []*v1.ScopeLogs
				scopeLog := &v1.ScopeLogs{
					Scope:      nil,
					LogRecords: batch,
					SchemaUrl:  "",
				}
				scopeLogs = append(scopeLogs, scopeLog)
				resourceAttributes := []*commonpb.KeyValue{
					{
						Key:   "source",
						Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "trace"}},
					},
					{
						Key:   "type",
						Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "event"}},
					},
				}

				resourceLog := &v1.ResourceLogs{
					Resource: &resourcepb.Resource{
						Attributes:             resourceAttributes,
						DroppedAttributesCount: 0,
					},
					ScopeLogs: scopeLogs,
				}
				resourceLogs = append(resourceLogs, resourceLog)

				if SendEvents && len(resourceLogs) > 0 {
					eventsReq := collogspb.ExportLogsServiceRequest{
						ResourceLogs: resourceLogs,
					}

					hostName, err := os.Hostname()
					if err != nil || hostName == "" {
						b.logger.Error().Logf("error Getting Hostname: %v", err)
						hostName = "ErrorHostname"
					}

					logsCtx := metadata.NewOutgoingContext(context.Background(), metadata.New(map[string]string{
						"tenantId": b.tenantId,
						"hostname": hostName,
					}))
					logsResponse, logsError := b.conn.GetLogClient().Export(logsCtx, &eventsReq)

					if st, ok := status.FromError(logsError); ok {
						if st.Code() != codes.OK {
							b.logger.Error().Logf("sending event log failed. error: %s", st.String())
							if strings.Contains(logsError.Error(), "LOG MANAGEMENT WAS NOT ENABLED") {
								b.logger.Error().Logf("Enable Log Management For Tenant and Restart Tracing Proxy")
								m.Lock()
								SendEvents = false
								m.Unlock()
							}
						}
					}
					b.logger.Debug().Logf("Sending Event Log response: %v", logsResponse.String())
				} else {
					b.logger.Debug().Logf("Unable to Process the logs exporting because SendEvents was: %v or len(resourcelogs) %v", SendEvents, len(resourceLogs))
				}

			}

		}

	}
}

func (b *batchAgg) SendTraceBatches(traceReq *proxypb.ExportTraceProxyServiceRequest) [][]*proxypb.ProxySpan {
	splittingTraces := traceReq.Items
	traceReq.Items = []*proxypb.ProxySpan{}
	var batchTraces [][]*proxypb.ProxySpan
	batchSize := 0
	for i, span := range splittingTraces {
		spanSize := proto.Size(span)
		if spanSize > maxApiSize {
			// OOPS! your larger than my tummy(1mb), so cant handle you
			b.logger.Info().Logf("Span size is greater than 1mb, so dropping")
			continue
		}
		if spanSize+batchSize > maxApiSize {
			batchTraces = append(batchTraces, traceReq.Items)
			batchSize = 0
			traceReq.Items = []*proxypb.ProxySpan{}
		}
		traceReq.Items = append(traceReq.Items, span)
		batchSize += spanSize
		if i == len(splittingTraces)-1 {
			batchTraces = append(batchTraces, traceReq.Items)
			batchSize = 0
			traceReq.Items = []*proxypb.ProxySpan{}
		}
	}
	return batchTraces
}

func (b *batchAgg) ExportTraces(traceReq *proxypb.ExportTraceProxyServiceRequest, ctx context.Context) {
	r, err := b.conn.GetTraceClient().ExportTraceProxy(ctx, traceReq)
	if st, ok := status.FromError(err); ok {
		if st.Code() != codes.OK {
			b.logger.Error().Logf("sending failed. error: %s", st.String())
			b.metrics.Increment("send_errors")
			if strings.Contains(strings.ToUpper(err.Error()), "TRACE MANAGEMENT WAS NOT ENABLED") {
				b.logger.Error().Logf("Enable Trace Management For Tenant and Restart Tracing Proxy")
				m.Lock()
				SendTraces = false
				m.Unlock()
			}
		} else {
			b.metrics.Increment("batches_sent")
		}
	}
	b.logger.Debug().Logf("trace proxy response msg: %s", r.GetMessage())
	b.logger.Debug().Logf("trace proxy response status: %s", r.GetStatus())
}

func (b *batchAgg) SendlogTraceBatches(logTraceReq *proxypb.ExportLogTraceProxyServiceRequest) [][]*proxypb.ProxyLogSpan {

	splittingLogTraces := logTraceReq.Items
	logTraceReq.Items = []*proxypb.ProxyLogSpan{}
	var batchLogTraces [][]*proxypb.ProxyLogSpan
	batchSize := 0
	for i, span := range splittingLogTraces {
		spanSize := proto.Size(span)
		if spanSize > maxApiSize {
			// OOPS! your larger than my tummy(1mb), so cant handle you
			b.logger.Info().Logf("Span with Events size is greater than 1mb, so dropping")
			continue
		}
		if spanSize+batchSize > maxApiSize {
			batchLogTraces = append(batchLogTraces, logTraceReq.Items)
			batchSize = 0
			logTraceReq.Items = []*proxypb.ProxyLogSpan{}
		}
		logTraceReq.Items = append(logTraceReq.Items, span)
		batchSize += spanSize
		if i == len(splittingLogTraces)-1 {
			batchLogTraces = append(batchLogTraces, logTraceReq.Items)
			batchSize = 0
			logTraceReq.Items = []*proxypb.ProxyLogSpan{}
		}
	}
	return batchLogTraces
}

func (b batchAgg) sendLogBatches(logRecords []*v1.LogRecord) [][]*v1.LogRecord {

	var logs []*v1.LogRecord
	var batchLogs [][]*v1.LogRecord
	batchSize := 0
	for i, log := range logRecords {
		logSize := proto.Size(log)
		if logSize > maxApiSize {
			// OOPS! your larger than my tummy(1mb), so cant handle you
			b.logger.Info().Logf("Log size is greater than 1mb, so dropping")
			continue
		}
		if logSize+batchSize > maxApiSize {
			batchLogs = append(batchLogs, logs)
			batchSize = 0
			logs = []*v1.LogRecord{}
		}
		logs = append(logs, log)
		batchSize += logSize
		if i == len(logRecords)-1 {
			batchLogs = append(batchLogs, logs)
			batchSize = 0
			logs = []*v1.LogRecord{}
		}
	}
	return batchLogs
}
