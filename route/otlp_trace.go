package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/opsramp/tracing-proxy/pkg/libtrace/proto/proxypb"
	"github.com/opsramp/tracing-proxy/pkg/libtrace/transmission"

	"github.com/opsramp/tracing-proxy/pkg/convert"
	"github.com/opsramp/tracing-proxy/types"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// postOTLP - processes requests from HTTP1.1
func (r *Router) postOTLP(w http.ResponseWriter, req *http.Request) {
	ri := convert.GetRequestInfoFromHttpHeaders(req.Header)

	if ri.ApiTenantId == "" {
		ri.ApiTenantId, _ = r.Config.GetTenantId()
	}
	if ri.Dataset == "" {
		ri.Dataset, _ = r.Config.GetDataset()
	}

	result, err := convert.TranslateTraceRequestFromReader(req.Body, ri, r.Config.GetAddAdditionalMetadata(), r.Config.GetSendEvents())
	if err != nil {
		r.handlerReturnWithError(w, ErrUpstreamFailed, err)
		return
	}

	if err := processTraceRequest(req.Context(), r, result.Batches, ri.ApiToken, ri.ApiTenantId); err != nil {
		r.handlerReturnWithError(w, ErrUpstreamFailed, err)
	}
}

// Export - processes requests from GRPC
func (r *Router) Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	ri := convert.GetRequestInfoFromGrpcMetadata(ctx)

	if ri.ApiTenantId == "" {
		ri.ApiTenantId, _ = r.Config.GetTenantId()
	}
	if ri.Dataset == "" {
		ri.Dataset, _ = r.Config.GetDataset()
	}

	r.Metrics.Increment(r.incomingOrPeer + "_router_batch")

	result, err := convert.TranslateTraceRequest(req, ri, r.Config.GetAddAdditionalMetadata(), r.Config.GetSendEvents())
	if err != nil {
		return nil, convert.AsGRPCError(err)
	}

	if err := processTraceRequest(ctx, r, result.Batches, ri.ApiToken, ri.ApiTenantId); err != nil {
		return nil, convert.AsGRPCError(err)
	}

	return &coltracepb.ExportTraceServiceResponse{}, nil
}

// processTraceRequest - process requests from peers
func processTraceRequest(ctx context.Context, router *Router, batches []convert.Batch, token, tenantID string) error {
	var requestID types.RequestIDContextKey
	apiHost, err := router.Config.GetOpsrampAPI()
	if err != nil {
		router.Logger.Error().Logf("Unable to retrieve APIHost from config while processing OTLP batch")
		return err
	}
	datasetName, err := router.Config.GetDataset()
	if err != nil {
		router.Logger.Error().Logf("Unable to retrieve DataSet from config while processing OTLP batch")
		return err
	}

	for _, batch := range batches {
		for _, ev := range batch.Events {
			var typeSpanEvents []types.SpanEvent
			for _, spanEvent := range ev.SpanEvents {
				typeSpanEvent := types.SpanEvent{
					Attributes: spanEvent.Attributes,
					Timestamp:  spanEvent.Timestamp,
					Name:       spanEvent.Name,
				}
				typeSpanEvents = append(typeSpanEvents, typeSpanEvent)
			}
			event := &types.Event{
				Context:     ctx,
				APIHost:     apiHost,
				APIToken:    token,
				APITenantId: tenantID,
				Dataset:     datasetName,
				Environment: datasetName,
				SampleRate:  uint(ev.SampleRate),
				Timestamp:   ev.Timestamp,
				Data:        ev.Attributes,
				SpanEvents:  typeSpanEvents,
			}
			if err := router.processEvent(event, requestID); err != nil {
				router.Logger.Error().Logf("Error processing event: " + err.Error())
			}
		}
	}
	return nil
}

func (r *Router) ExportTraceProxy(ctx context.Context, in *proxypb.ExportTraceProxyServiceRequest) (*proxypb.ExportTraceProxyServiceResponse, error) {
	r.Logger.Debug().Logf("Received Trace data from peer")
	r.Metrics.Increment(r.incomingOrPeer + "_router_batch")

	apiHost, err := r.Config.GetOpsrampAPI()
	if err != nil {
		r.Logger.Error().Logf("Unable to retrieve APIHost from config while processing OTLP batch")
		return &proxypb.ExportTraceProxyServiceResponse{Message: "Failed to get apihost", Status: "Failed"}, nil
	}
	dataset, _ := r.Config.GetDataset()
	tenantId, _ := r.Config.GetTenantId()

	var requestID types.RequestIDContextKey

	for _, item := range in.Items {
		timestamp, err := time.Parse(time.RFC3339Nano, item.Timestamp)
		if err != nil {
			r.Logger.Error().Logf("failed to parse timestamp: %v", err)
			continue
		}

		var data map[string]interface{}
		inrec, err := json.Marshal(item.Data)
		if err != nil {
			r.Logger.Error().Logf("failed to marshal: %v", err)
			continue
		}
		err = json.Unmarshal(inrec, &data)
		if err != nil {
			r.Logger.Error().Logf("failed to unmarshal: %v", err)
			continue
		}

		// Translate ResourceAttributes , SpanAttributes, EventAttributes from proto format to interface{}
		attributes := make(map[string]interface{})
		for _, kv := range item.Data.ResourceAttributes {
			attributes[kv.Key] = extractKeyValue(kv.Value)
		}
		data["resourceAttributes"] = attributes

		attributes = make(map[string]interface{})
		for _, kv := range item.Data.SpanAttributes {
			attributes[kv.Key] = extractKeyValue(kv.Value)
		}
		data["spanAttributes"] = attributes

		attributes = make(map[string]interface{})
		for _, kv := range item.Data.EventAttributes {
			attributes[kv.Key] = extractKeyValue(kv.Value)
		}
		data["eventAttributes"] = attributes

		// Type cast start and end time
		data["startTime"] = item.Data.StartTime
		data["endTime"] = item.Data.EndTime

		event := &types.Event{
			Context:     ctx,
			APIHost:     apiHost,
			APITenantId: tenantId,
			Dataset:     dataset,
			Timestamp:   timestamp,
			Data:        data,
		}
		if err := r.processEvent(event, requestID); err != nil {
			r.Logger.Error().Logf("Error processing event: " + err.Error())
		}
	}
	return &proxypb.ExportTraceProxyServiceResponse{Message: "Received Successfully by peer", Status: "Success"}, nil
}

func (r *Router) ExportLogTraceProxy(ctx context.Context, in *proxypb.ExportLogTraceProxyServiceRequest) (*proxypb.ExportTraceProxyServiceResponse, error) {
	r.Logger.Debug().Logf("Received Trace data from peer")
	r.Metrics.Increment(r.incomingOrPeer + "_router_batch")

	apiHost, err := r.Config.GetOpsrampAPI()
	if err != nil {
		r.Logger.Error().Logf("Unable to retrieve APIHost from config while processing OTLP batch")
		return &proxypb.ExportTraceProxyServiceResponse{Message: "Failed to get apihost", Status: "Failed"}, nil
	}
	dataset, _ := r.Config.GetDataset()
	tenantId, _ := r.Config.GetTenantId()

	var requestID types.RequestIDContextKey

	for _, item := range in.Items {
		timestamp, err := time.Parse(time.RFC3339Nano, item.Timestamp)
		if err != nil {
			r.Logger.Error().Logf("failed to parse timestamp: %v", err)
			continue
		}

		var data map[string]interface{}
		inrec, err := json.Marshal(item.Data)
		if err != nil {
			r.Logger.Error().Logf("failed to marshal: %v", err)
			continue
		}
		err = json.Unmarshal(inrec, &data)
		if err != nil {
			r.Logger.Error().Logf("failed to unmarshal: %v", err)
			continue
		}

		// Translate ResourceAttributes , SpanAttributes, EventAttributes from proto format to interface{}
		attributes := make(map[string]interface{})
		for _, kv := range item.Data.ResourceAttributes {
			attributes[kv.Key] = extractKeyValue(kv.Value)
		}
		data["resourceAttributes"] = attributes

		attributes = make(map[string]interface{})
		for _, kv := range item.Data.SpanAttributes {
			attributes[kv.Key] = extractKeyValue(kv.Value)
		}
		data["spanAttributes"] = attributes

		attributes = make(map[string]interface{})
		for _, kv := range item.Data.EventAttributes {
			attributes[kv.Key] = extractKeyValue(kv.Value)
		}
		data["eventAttributes"] = attributes

		// Type cast start and end time
		data["startTime"] = item.Data.StartTime
		data["endTime"] = item.Data.EndTime

		var proxySpanEvents []types.SpanEvent
		for _, spanEvent := range item.Data.SpanEvents {
			proxySpanEvent := types.SpanEvent{
				Attributes: ConvertKeyValueSliceToMap(spanEvent.SpanEventAttributes),
				Timestamp:  spanEvent.TimeStamp,
				Name:       spanEvent.Name,
			}
			proxySpanEvents = append(proxySpanEvents, proxySpanEvent)
		}

		event := &types.Event{
			Context:     ctx,
			APIHost:     apiHost,
			APITenantId: tenantId,
			Dataset:     dataset,
			Timestamp:   timestamp,
			Data:        data,
			SpanEvents:  proxySpanEvents,
		}
		if err := r.processEvent(event, requestID); err != nil {
			r.Logger.Error().Logf("Error processing event: " + err.Error())
		}
	}
	return &proxypb.ExportTraceProxyServiceResponse{Message: "Received Successfully by peer", Status: "Success"}, nil
}

func (r *Router) Status(context.Context, *proxypb.StatusRequest) (*proxypb.StatusResponse, error) {
	return &proxypb.StatusResponse{
		PeerActive: transmission.DefaultAvailability.Status(),
	}, nil
}

func extractKeyValue(v *proxypb.AnyValue) string {
	if x, ok := v.GetValue().(*proxypb.AnyValue_StringValue); ok {
		return x.StringValue
	} else if x, ok := v.GetValue().(*proxypb.AnyValue_IntValue); ok {
		return fmt.Sprintf("%d", x.IntValue)
	} else if x, ok := v.GetValue().(*proxypb.AnyValue_BoolValue); ok {
		return fmt.Sprintf("%v", x.BoolValue)
	} else if x, ok := v.GetValue().(*proxypb.AnyValue_DoubleValue); ok {
		return fmt.Sprintf("%f", x.DoubleValue)
	} else if x, ok := v.GetValue().(*proxypb.AnyValue_BytesValue); ok {
		return fmt.Sprintf("%v", x.BytesValue)
	} else if x, ok := v.GetValue().(*proxypb.AnyValue_ArrayValue); ok {
		return x.ArrayValue.String()
	} else if x, ok := v.GetValue().(*proxypb.AnyValue_KvlistValue); ok {
		return x.KvlistValue.String()
	}
	return v.String()
}

// ConvertKeyValueSliceToMap converts a slice of KeyValue messages to a map of string to interface{}
func ConvertKeyValueSliceToMap(kvSlice []*proxypb.KeyValue) map[string]interface{} {
	kvMap := make(map[string]interface{})
	for _, kv := range kvSlice {
		// Get the key and value from the KeyValue message
		key := kv.GetKey()
		value := kv.GetValue()
		if value == nil {
			kvMap[key] = ""
			continue
		}

		// Convert the value to an interface{} based on the type of the AnyValue message
		var valueInterface interface{}
		switch v := value.Value.(type) {
		case *proxypb.AnyValue_StringValue:
			valueInterface = v.StringValue
		case *proxypb.AnyValue_BoolValue:
			valueInterface = v.BoolValue
		case *proxypb.AnyValue_IntValue:
			valueInterface = v.IntValue
		case *proxypb.AnyValue_DoubleValue:
			valueInterface = v.DoubleValue
		default:
			// If the value is unknown, use nil
			valueInterface = ""
		}

		// Add the key and value to the map
		kvMap[key] = valueInterface
	}
	return kvMap
}
