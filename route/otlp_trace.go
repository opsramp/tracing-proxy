package route

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/opsramp/libtrace-go/proto/proxypb"
	"github.com/opsramp/libtrace-go/transmission"
	"google.golang.org/grpc/metadata"
	"net/http"
	"strings"
	"time"

	huskyotlp "github.com/opsramp/husky/otlp"
	"github.com/opsramp/tracing-proxy/types"

	collectortrace "github.com/opsramp/husky/proto/otlp/collector/trace/v1"
)

func (r *Router) postOTLP(w http.ResponseWriter, req *http.Request) {
	ri := huskyotlp.GetRequestInfoFromHttpHeaders(req.Header)

	if ri.ApiTenantId == "" {
		ri.ApiTenantId, _ = r.Config.GetTenantId()
	}
	if ri.Dataset == "" {
		ri.Dataset, _ = r.Config.GetDataset()
	}

	result, err := huskyotlp.TranslateTraceRequestFromReader(req.Body, ri)
	if err != nil {
		r.handlerReturnWithError(w, ErrUpstreamFailed, err)
		return
	}

	if err := processTraceRequest(req.Context(), r, result.Batches, ri.Dataset, ri.ApiToken, ri.ApiTenantId); err != nil {
		r.handlerReturnWithError(w, ErrUpstreamFailed, err)
	}
}

func (r *Router) Export(ctx context.Context, req *collectortrace.ExportTraceServiceRequest) (*collectortrace.ExportTraceServiceResponse, error) {
	ri := huskyotlp.GetRequestInfoFromGrpcMetadata(ctx)

	if ri.ApiTenantId == "" {
		ri.ApiTenantId, _ = r.Config.GetTenantId()
	}
	if ri.Dataset == "" {
		ri.Dataset, _ = r.Config.GetDataset()
	}

	r.Metrics.Increment(r.incomingOrPeer + "_router_batch")

	result, err := huskyotlp.TranslateTraceRequest(req, ri)
	if err != nil {
		return nil, huskyotlp.AsGRPCError(err)
	}

	if err := processTraceRequest(ctx, r, result.Batches, ri.Dataset, ri.ApiToken, ri.ApiTenantId); err != nil {
		return nil, huskyotlp.AsGRPCError(err)
	}

	return &collectortrace.ExportTraceServiceResponse{}, nil
}

func processTraceRequest(
	ctx context.Context,
	router *Router,
	batches []huskyotlp.Batch,
	datasetName string,
	token string,
	tenantID string) error {
	var requestID types.RequestIDContextKey
	apiHost, err := router.Config.GetOpsrampAPI()
	if err != nil {
		router.Logger.Error().Logf("Unable to retrieve APIHost from config while processing OTLP batch")
		return err
	}

	for _, batch := range batches {
		for _, ev := range batch.Events {
			event := &types.Event{
				Context:     ctx,
				APIHost:     apiHost,
				APIToken:    token,
				APITenantId: tenantID,
				Dataset:     datasetName,
				Environment: "",
				SampleRate:  uint(ev.SampleRate),
				Timestamp:   ev.Timestamp,
				Data:        ev.Attributes,
			}
			if err = router.processEvent(event, requestID); err != nil {
				router.Logger.Error().Logf("Error processing event: " + err.Error())
			}
		}
	}

	return nil
}

func (r *Router) ExportTraceProxy(ctx context.Context, in *proxypb.ExportTraceProxyServiceRequest) (*proxypb.ExportTraceProxyServiceResponse, error) {

	r.Logger.Debug().Logf("Received Trace data from peer")
	r.Metrics.Increment(r.incomingOrPeer + "_router_batch")

	var token, tenantId, datasetName string
	apiHost, err := r.Config.GetOpsrampAPI()
	if err != nil {
		r.Logger.Error().Logf("Unable to retrieve APIHost from config while processing OTLP batch")
		return &proxypb.ExportTraceProxyServiceResponse{Message: "Failed to get apihost", Status: "Failed"}, nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return &proxypb.ExportTraceProxyServiceResponse{Message: "Failed to get request metadata", Status: "Failed"}, nil
	} else {
		authorization := md.Get("Authorization")
		if len(authorization) == 0 {
			return &proxypb.ExportTraceProxyServiceResponse{Message: "Failed to get Authorization", Status: "Failed"}, nil
		} else {
			token = authorization[0]
			recvdTenantId := md.Get("tenantId")
			if len(recvdTenantId) == 0 {
				tenantId = strings.TrimSpace(in.TenantId)
				if tenantId == "" {
					return &proxypb.ExportTraceProxyServiceResponse{Message: "Failed to get TenantId", Status: "Failed"}, nil
				}
			} else {
				tenantId = recvdTenantId[0]
			}
		}

		if dataSets := md.Get("dataset"); len(dataSets) > 0 {
			datasetName = dataSets[0]
		} else {
			return &proxypb.ExportTraceProxyServiceResponse{Message: "Failed to get dataset", Status: "Failed"}, nil
		}
	}

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

		//Translate ResourceAttributes , SpanAttributes, EventAttributes from proto format to interface{}
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

		//Type cast start and end time
		data["startTime"] = item.Data.StartTime
		data["endTime"] = item.Data.EndTime

		event := &types.Event{
			Context:  ctx,
			APIHost:  apiHost,
			APIToken: token,
			//APIKey:      "token", //Hardcoded for time-being. This need to be cleaned
			APITenantId: tenantId,
			Dataset:     datasetName,
			Timestamp:   timestamp,
			Data:        data,
		}
		if err = r.processEvent(event, requestID); err != nil {
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
