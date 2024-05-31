package convert

import (
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/martian/log"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

const (
	traceIDShortLength = 8
	traceIDLongLength  = 16
	defaultSampleRate  = int32(1)
)

var (
	possibleServiceNames  = []string{"service_name", "service.name"}
	possibleInstanceNames = []string{"instance", "k8s.pod.name", "host.name"}
)

// TranslateTraceRequestResult represents an OTLP trace request translated into Opsramp-friendly structure
// RequestSize is total byte size of the entire OTLP request
// Batches represent events grouped by their target dataset
type TranslateTraceRequestResult struct {
	RequestSize int
	Batches     []Batch
}

// TranslateTraceRequestFromReader translates an OTLP/HTTP request into Honeycomb-friendly structure
// RequestInfo is the parsed information from the HTTP headers
func TranslateTraceRequestFromReader(body io.ReadCloser, ri RequestInfo, additionalAttr map[string]string, sendEvents bool) (*TranslateOTLPRequestResult, error) {
	request := &coltracepb.ExportTraceServiceRequest{}
	if err := parseOtlpRequestBody(body, ri.ContentType, ri.ContentEncoding, request); err != nil {
		return nil, fmt.Errorf("%s: %s", ErrFailedParseBody, err)
	}
	return TranslateTraceRequest(request, ri, additionalAttr, sendEvents)
}

// TranslateTraceRequest translates an OTLP/gRPC request into OpsRamp-friendly structure
// RequestInfo is the parsed information from the gRPC metadata
func TranslateTraceRequest(request *coltracepb.ExportTraceServiceRequest, ri RequestInfo, additionalResAttr map[string]string, sendEvents bool) (*TranslateOTLPRequestResult, error) {
	var batches []Batch

	for _, resourceSpan := range request.ResourceSpans {
		var events []Event
		var spanEvents []SpanEvent
		resourceAttrs := getResourceAttributes(resourceSpan.Resource)
		dataset := getDataset(ri, resourceAttrs)
		traceAttributes := make(map[string]map[string]interface{})
		traceAttributes["resourceAttributes"] = make(map[string]interface{})
		// filling all the additional attributes here
		for k, v := range additionalResAttr {
			traceAttributes["resourceAttributes"][k] = v
		}

		// trying to classify the spans based on resource attributes
		_classificationAttributes := map[string]string{}

		if resourceSpan.Resource != nil {
			addAttributesToMap(traceAttributes["resourceAttributes"], resourceSpan.Resource.Attributes)
			_classificationAttributes = DetermineClassification(resourceSpan.GetResource().GetAttributes())
		}

		// normalizing service_name of trace
		isUnknownService := true
		for _, key := range possibleServiceNames {
			if val, ok := traceAttributes["resourceAttributes"][key]; ok {
				isUnknownService = false
				delete(traceAttributes["resourceAttributes"], key)
				traceAttributes["resourceAttributes"]["service_name"] = val
				break
			}
		}
		if isUnknownService {
			traceAttributes["resourceAttributes"]["service_name"] = _unknown
		}
		// normalizing instance name
		isUnknownInstance := true
		for _, key := range possibleInstanceNames {
			if val, ok := traceAttributes["resourceAttributes"][key]; ok {
				isUnknownInstance = false
				traceAttributes["resourceAttributes"]["instance"] = val
				break
			}
		}
		if isUnknownInstance {
			traceAttributes["resourceAttributes"]["instance"] = _unknown
		}

		for _, librarySpan := range resourceSpan.ScopeSpans {
			scopeAttrs := getScopeAttributes(librarySpan.Scope)

			// update classification attrs with scope attributes
			_scopeClassificationAttrs := _classificationAttributes
			if librarySpan.GetScope() != nil {
				_scopeClassificationAttrs = NormalizeClassification(_classificationAttributes, librarySpan.GetScope().GetAttributes())
			}

			for _, span := range librarySpan.GetSpans() {
				traceAttributes["spanAttributes"] = make(map[string]interface{})
				traceAttributes["eventAttributes"] = make(map[string]interface{})

				traceID := BytesToTraceID(span.TraceId)
				spanID := hex.EncodeToString(span.SpanId)

				spanKind := getSpanKind(span.Kind)

				var spanStatusCode int64
				var isError bool

				for key, attributeValue := range span.Attributes {
					if attributeValue.GetKey() == "http.status_code" || attributeValue.GetKey() == "http.response.status_code" {
						statusCodeType := fmt.Sprintf("%v", span.Attributes[key].Value)
						if strings.Contains(statusCodeType, "int") {
							spanStatusCode = span.Attributes[key].Value.GetIntValue()
						} else {
							var err error
							spanStatusCode, err = strconv.ParseInt(span.Attributes[key].Value.GetStringValue(), 10, 64)
							if err != nil {
								log.Errorf("Unable to Check span status Code: ", err, " statusCodeType was: ", statusCodeType)
							}
						}
						if spanStatusCode >= 400 || spanStatusCode == 0 {
							isError = true
							break
						}
					} else if attributeValue.GetKey() == "rpc.grpc.status_code" {
						statusCodeType := fmt.Sprintf("%v", span.Attributes[key].Value)
						if strings.Contains(statusCodeType, "int") {
							spanStatusCode = span.Attributes[key].Value.GetIntValue()
						} else {
							var err error
							spanStatusCode, err = strconv.ParseInt(span.Attributes[key].Value.GetStringValue(), 10, 64)
							if err != nil {
								log.Errorf("Unable to Check span status Code: ", err)
							}
						}

						if spanStatusCode != 0 {
							isError = true
							break
						}
					}
				}

				if !isError {
					spanStatusCode = int64(tracepb.Status_STATUS_CODE_UNSET)
				} else {
					traceAttributes["spanAttributes"]["error"] = isError
				}

				eventAttrs := map[string]interface{}{
					"traceTraceID":     traceID,
					"traceSpanID":      spanID,
					"type":             spanKind,
					"spanKind":         spanKind,
					"spanName":         span.Name,
					"durationMs":       float64(span.EndTimeUnixNano-span.StartTimeUnixNano) / float64(time.Millisecond),
					"startTime":        int64(span.StartTimeUnixNano),
					"endTime":          int64(span.EndTimeUnixNano),
					"statusCode":       spanStatusCode,
					"spanNumLinks":     len(span.Links),
					"spanNumEvents":    len(span.Events),
					"meta.signal_type": "trace",
				}
				if span.ParentSpanId != nil {
					eventAttrs["traceParentID"] = hex.EncodeToString(span.ParentSpanId)
				}

				if isError {
					eventAttrs["error"] = isError
				}

				if span.Status != nil && len(span.Status.Message) > 0 {
					eventAttrs["statusMessage"] = span.Status.Message
				}

				// copy resource & scope attributes then span attributes
				for k, v := range resourceAttrs {
					eventAttrs[k] = v
				}
				for k, v := range scopeAttrs {
					eventAttrs[k] = v
				}

				// update scope classification attrs with span attributes
				if span.Attributes != nil {
					addAttributesToMap(traceAttributes["spanAttributes"], span.Attributes)
					_spanClassificationAttrs := NormalizeClassification(_scopeClassificationAttrs, span.GetAttributes())

					for k, v := range _spanClassificationAttrs {
						traceAttributes["spanAttributes"][k] = v
					}
				}

				// get sample rate after resource and scope attributes have been added
				sampleRate := getSampleRate(eventAttrs)

				// Copy resource attributes
				eventAttrs["resourceAttributes"] = traceAttributes["resourceAttributes"]

				// normalizing instance name
				if isUnknownInstance {
					stillUnknownInstance := true
					for _, key := range possibleInstanceNames {
						if val, ok := traceAttributes["spanAttributes"][key]; ok {
							traceAttributes["spanAttributes"]["instance"] = val
							stillUnknownInstance = false
							break
						}
					}
					if stillUnknownInstance {
						traceAttributes["spanAttributes"]["instance"] = _unknown
					}
				}

				for key, value := range scopeAttrs {
					traceAttributes["spanAttributes"][key] = value
				}

				// Copy span attributes
				eventAttrs["spanAttributes"] = traceAttributes["spanAttributes"]

				// Check for event attributes and add them

				if sendEvents {
					spanEvents = []SpanEvent{}
					for _, sevent := range span.Events {
						traceAttributes["spanEventAttributes"] = make(map[string]interface{})
						if sevent.Attributes != nil {
							addAttributesToMap(traceAttributes["eventAttributes"], sevent.Attributes)
						}
						addAttributesToMap(traceAttributes["spanEventAttributes"], sevent.Attributes)
						traceAttributes["spanEventAttributes"]["traceId"] = traceID
						traceAttributes["spanEventAttributes"]["spanId"] = spanID
						traceAttributes["spanEventAttributes"]["trace_operation"] = span.Name
						if traceAttributes["spanAttributes"]["instance"] == nil {
							traceAttributes["spanAttributes"]["instance"] = traceAttributes["resourceAttributes"]["instance"]
						}
						traceAttributes["spanEventAttributes"]["trace_instance"] = traceAttributes["spanAttributes"]["instance"]
						traceAttributes["spanEventAttributes"]["trace_service"] = traceAttributes["resourceAttributes"]["service_name"]
						spanEvents = append(spanEvents, SpanEvent{
							Name:       sevent.GetName(),
							Timestamp:  sevent.TimeUnixNano,
							Attributes: traceAttributes["spanEventAttributes"],
						})
					}
				}

				eventAttrs["eventAttributes"] = traceAttributes["eventAttributes"]

				eventAttrs["time"] = int64(span.StartTimeUnixNano)
				// Now we need to wrap the eventAttrs in an event so we can specify the timestamp
				// which is the StartTime as a time.Time object
				timestamp := time.Unix(0, int64(span.StartTimeUnixNano)).UTC()
				events = append(events, Event{
					Attributes: eventAttrs,
					Timestamp:  timestamp,
					SampleRate: sampleRate,
					SpanEvents: spanEvents,
				})
			}
		}
		batches = append(batches, Batch{
			Dataset:   dataset,
			SizeBytes: proto.Size(resourceSpan),
			Events:    events,
		})
	}
	return &TranslateOTLPRequestResult{
		RequestSize: proto.Size(request),
		Batches:     batches,
	}, nil
}

func getSpanKind(kind tracepb.Span_SpanKind) string {
	switch kind {
	case tracepb.Span_SPAN_KIND_CLIENT:
		return "client"
	case tracepb.Span_SPAN_KIND_SERVER:
		return "server"
	case tracepb.Span_SPAN_KIND_PRODUCER:
		return "producer"
	case tracepb.Span_SPAN_KIND_CONSUMER:
		return "consumer"
	case tracepb.Span_SPAN_KIND_INTERNAL:
		return "internal"
	case tracepb.Span_SPAN_KIND_UNSPECIFIED:
		fallthrough
	default:
		return "unspecified"
	}
}

// BytesToTraceID returns an ID suitable for use for spans and traces. Before
// encoding the bytes as a hex string, we want to handle cases where we are
// given 128-bit IDs with zero padding, e.g. 0000000000000000f798a1e7f33c8af6.
// There are many ways to achieve this, but careful benchmarking and testing
// showed the below as the most performant, avoiding memory allocations
// and the use of flexible but expensive library functions. As this is hot code,
// it seemed worthwhile to do it this way.
func BytesToTraceID(traceID []byte) string {
	var encoded []byte
	switch len(traceID) {
	case traceIDLongLength: // 16 bytes, trim leading 8 bytes if all 0's
		if shouldTrimTraceId(traceID) {
			encoded = make([]byte, 16)
			traceID = traceID[traceIDShortLength:]
		} else {
			encoded = make([]byte, 32)
		}
	case traceIDShortLength: // 8 bytes
		encoded = make([]byte, 16)
	default:
		encoded = make([]byte, len(traceID)*2)
	}
	hex.Encode(encoded, traceID)
	return string(encoded)
}

func shouldTrimTraceId(traceID []byte) bool {
	for i := 0; i < 8; i++ {
		if traceID[i] != 0 {
			return false
		}
	}
	return true
}

func getSampleRate(attrs map[string]interface{}) int32 {
	sampleRateKey := getSampleRateKey(attrs)
	if sampleRateKey == "" {
		return defaultSampleRate
	}

	sampleRate := defaultSampleRate
	sampleRateVal := attrs[sampleRateKey]
	switch v := sampleRateVal.(type) {
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			if i < math.MaxInt32 {
				sampleRate = int32(i)
			} else {
				sampleRate = math.MaxInt32
			}
		}
	case int32:
		sampleRate = v
	case int:
		if v < math.MaxInt32 {
			sampleRate = int32(v)
		} else {
			sampleRate = math.MaxInt32
		}
	case int64:
		if v < math.MaxInt32 {
			sampleRate = int32(v)
		} else {
			sampleRate = math.MaxInt32
		}
	}
	// To make sampleRate consistent between Otel and Honeycomb, we coerce all 0 values to 1 here
	// A value of 1 means the span was not sampled
	// For full explanation, see https://app.asana.com/0/365940753298424/1201973146987622/f
	if sampleRate == 0 {
		sampleRate = defaultSampleRate
	}
	delete(attrs, sampleRateKey) // remove attr
	return sampleRate
}

func getSampleRateKey(attrs map[string]interface{}) string {
	if _, ok := attrs["sampleRate"]; ok {
		return "sampleRate"
	}
	if _, ok := attrs["SampleRate"]; ok {
		return "SampleRate"
	}
	return ""
}
