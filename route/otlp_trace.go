package route

import (
	"context"
	"encoding/json"
	"fmt"
	proxypb "github.com/honeycombio/libhoney-go/proto/proxypb"
	"google.golang.org/grpc/metadata"
	"log"
	"net/http"
	"time"

	huskyotlp "github.com/honeycombio/husky/otlp"
	"github.com/jirs5/tracing-proxy/types"

	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)





func (router *Router) postOTLP(w http.ResponseWriter, req *http.Request) {
	ri := huskyotlp.GetRequestInfoFromHttpHeaders(req.Header)
	/*if err := ri.ValidateHeaders(); err != nil {
		if errors.Is(err, huskyotlp.ErrInvalidContentType) {
			router.handlerReturnWithError(w, ErrInvalidContentType, err)
		} else {
			router.handlerReturnWithError(w, ErrAuthNeeded, err)
		}
		return
	}*/

	result, err := huskyotlp.TranslateTraceReqFromReader(req.Body, ri)
	if err != nil {
		router.handlerReturnWithError(w, ErrUpstreamFailed, err)
		return
	}

	token := ri.ApiToken
	tenantId := ri.ApiTenantId
	if err := processTraceRequest(req.Context(), router, result.Batches, ri.Dataset, token, tenantId); err != nil {
		router.handlerReturnWithError(w, ErrUpstreamFailed, err)
	}
}

func (router *Router) Export(ctx context.Context, req *collectortrace.ExportTraceServiceRequest) (*collectortrace.ExportTraceServiceResponse, error) {
	ri := huskyotlp.GetRequestInfoFromGrpcMetadata(ctx)
	/*if err := ri.ValidateHeaders(); err != nil {
		return nil, huskyotlp.AsGRPCError(err)
	}*/
	router.Metrics.Increment(router.incomingOrPeer + "_router_batch")
	fmt.Println("Translating Trace Req ..")
	result, err := huskyotlp.TranslateTraceReq(req, ri)
	//fmt.Println("req",result.Batches[0])
	if err != nil {
		return nil, huskyotlp.AsGRPCError(err)
	}
	token := ri.ApiToken
	tenantId := ri.ApiTenantId
	if len(tenantId) == 0 {
		OpsrampTenantId, _ := router.Config.GetTenantId()
		tenantId = OpsrampTenantId
	}

	if len(ri.Dataset) == 0 {
		dataset, _ :=  router.Config.GetDataset()
		ri.Dataset = dataset
	}

	fmt.Println("TenantId:", tenantId)
	fmt.Println("dataset:", ri.Dataset)

	if err := processTraceRequest(ctx, router, result.Batches, ri.Dataset, token, tenantId); err != nil {
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
	tenantId string) error {

	var requestID types.RequestIDContextKey
	apiHost, err := router.Config.GetOpsrampAPI()
	if err != nil {
		router.Logger.Error().Logf("Unable to retrieve APIHost from config while processing OTLP batch")
		return err
	}
	//fmt.Println("datasetName",datasetName)

	for _, batch := range batches {
		for _, ev := range batch.Events {
			event := &types.Event{
				Context:     ctx,
				APIHost:     apiHost,
				APIToken:    token,
				APITenantId: tenantId,
				Dataset:     datasetName,
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

	fmt.Println("Received Trace data from peer \n")
	r.Metrics.Increment(r.incomingOrPeer + "_router_batch")

	var token, tenantId, datasetName string
	apiHost, err := r.Config.GetOpsrampAPI()
	if err != nil {
		r.Logger.Error().Logf("Unable to retrieve APIHost from config while processing OTLP batch")
		return &proxypb.ExportTraceProxyServiceResponse{Message: "Failed to get apihost", Status: "Failed"}, nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		log.Println("Failed to get metadata")
		return &proxypb.ExportTraceProxyServiceResponse{Message: "Failed to get request metadata", Status: "Failed"}, nil
	} else {
		authorization := md.Get("Authorization")
		fmt.Println("authorization is ", authorization)
		if len(authorization) == 0 {
			return &proxypb.ExportTraceProxyServiceResponse{Message: "Failed to get Authorization", Status: "Failed"}, nil
		} else {
			token = authorization[0]
			recvdTenantId := md.Get("tenantId")
			if len(recvdTenantId) == 0 {
				return &proxypb.ExportTraceProxyServiceResponse{Message: "Failed to get TenantId", Status: "Failed"}, nil
			} else {
				tenantId = recvdTenantId[0]
				datasetName = md.Get("dataset")[0]
			}
		}
		log.Printf("\nauthorization:%v", token)
		log.Printf("\nTenantId:%v", tenantId)
	}

	var requestID types.RequestIDContextKey

	for _, item := range in.Items {
		layout := "2006-01-02 15:04:05.000000000 +0000 UTC"
		timestamp, err := time.Parse(layout, item.Timestamp)

		var data map[string]interface{}
		inrec, _ := json.Marshal(item.Data)
		json.Unmarshal(inrec, &data)

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
