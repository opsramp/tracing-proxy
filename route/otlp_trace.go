package route

import (
	"context"
	"fmt"
	"net/http"

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
	if err := processTraceRequest(req.Context(), router, result.Batches, ri.ApiKey, ri.Dataset, token, tenantId); err != nil {
		router.handlerReturnWithError(w, ErrUpstreamFailed, err)
	}
}

func (router *Router) Export(ctx context.Context, req *collectortrace.ExportTraceServiceRequest) (*collectortrace.ExportTraceServiceResponse, error) {
	ri := huskyotlp.GetRequestInfoFromGrpcMetadata(ctx)
	/*if err := ri.ValidateHeaders(); err != nil {
		return nil, huskyotlp.AsGRPCError(err)
	}*/
	fmt.Println("Translating Trace Req ..")
	result, err := huskyotlp.TranslateTraceReq(req, ri)
	if err != nil {
		return nil, huskyotlp.AsGRPCError(err)
	}
	token := ri.ApiToken
	tenantId := ri.ApiTenantId

	fmt.Println("Token:", token)
	fmt.Println("TenantId:", tenantId)

	if err := processTraceRequest(ctx, router, result.Batches, ri.ApiKey, ri.Dataset, token, tenantId); err != nil {
		return nil, huskyotlp.AsGRPCError(err)
	}

	return &collectortrace.ExportTraceServiceResponse{}, nil
}

func processTraceRequest(
	ctx context.Context,
	router *Router,
	batches []huskyotlp.Batch,
	apiKey string,
	datasetName string,
	token string,
	tenantId string) error {

	var requestID types.RequestIDContextKey
	apiHost, err := router.Config.GetHoneycombAPI()
	if err != nil {
		router.Logger.Error().Logf("Unable to retrieve APIHost from config while processing OTLP batch")
		return err
	}

	for _, batch := range batches {
		for _, ev := range batch.Events {
			event := &types.Event{
				Context:     ctx,
				APIHost:     apiHost,
				APIKey:      apiKey,
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
