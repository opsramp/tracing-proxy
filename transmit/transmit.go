package transmit

import (
	"context"
	"os"
	"sync"

	"github.com/opsramp/libtrace-go"
	"github.com/opsramp/libtrace-go/transmission"

	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/logger"
	"github.com/opsramp/tracing-proxy/metrics"
	"github.com/opsramp/tracing-proxy/types"
)

type Transmission interface {
	// Enqueue accepts a single event and schedules it for transmission to Opsramp
	EnqueueEvent(ev *types.Event)
	EnqueueSpan(ev *types.Span)
	// Flush flushes the in-flight queue of all events and spans
	Flush()
}

const (
	counterEnqueueErrors  = "enqueue_errors"
	counterResponse20x    = "response_20x"
	counterResponseErrors = "response_errors"
)

type DefaultTransmission struct {
	Config     config.Config   `inject:""`
	Logger     logger.Logger   `inject:""`
	Metrics    metrics.Metrics `inject:"metrics"`
	Version    string          `inject:"version"`
	LibhClient *libtrace.Client

	// Type is peer or upstream, and used only for naming metrics
	Name string

	builder          *libtrace.Builder
	responseCanceler context.CancelFunc
}

var once sync.Once

func (d *DefaultTransmission) Start() error {
	d.Logger.Debug().Logf("Starting DefaultTransmission: %s type", d.Name)
	defer func() { d.Logger.Debug().Logf("Finished starting DefaultTransmission: %s type", d.Name) }()

	// upstreamAPI doesn't get set when the client is initialized, because
	// it can be reloaded from the config file while live
	upstreamAPI, err := d.Config.GetOpsrampAPI()
	if err != nil {
		return err
	}

	if d.Config.GetAddHostMetadataToTrace() {
		if hostname, err := os.Hostname(); err == nil && hostname != "" {
			// add hostname to spans
			d.LibhClient.AddResourceField("meta.local_hostname", hostname)
		}
	}
	for key, value := range d.Config.GetAddAdditionalMetadata() {
		if !d.LibhClient.CheckResourceField(key) {
			d.LibhClient.AddResourceField(key, value)
		}
	}

	d.builder = d.LibhClient.NewBuilder()
	d.builder.APIHost = upstreamAPI

	once.Do(func() {
		libtrace.UserAgentAddition = "tracing-proxy/" + d.Version
	})

	d.Metrics.Register(d.Name+counterEnqueueErrors, "counter")
	d.Metrics.Register(d.Name+counterResponse20x, "counter")
	d.Metrics.Register(d.Name+counterResponseErrors, "counter")

	processCtx, canceler := context.WithCancel(context.Background())
	d.responseCanceler = canceler
	go d.processResponses(processCtx, d.LibhClient.TxResponses())

	// listen for config reloads
	d.Config.RegisterReloadCallback(d.reloadTransmissionBuilder)
	return nil
}

func (d *DefaultTransmission) reloadTransmissionBuilder() {
	d.Logger.Debug().Logf("reloading transmission config")
	upstreamAPI, err := d.Config.GetOpsrampAPI()
	if err != nil {
		// log and skip reload
		d.Logger.Error().Logf("Failed to reload Opsramp API when reloading configs:", err)
	}
	builder := d.LibhClient.NewBuilder()
	builder.APIHost = upstreamAPI
}

func (d *DefaultTransmission) EnqueueEvent(ev *types.Event) {
	d.Logger.Debug().
		WithField("request_id", ev.Context.Value(types.RequestIDContextKey{})).
		WithString("api_host", ev.APIHost).
		WithString("dataset", ev.Dataset).
		Logf("transmit sending event")
	libhEv := d.builder.NewEvent()
	libhEv.APIHost = ev.APIHost
	//libhEv.WriteKey = ev.APIKey
	libhEv.Dataset = ev.Dataset
	libhEv.SampleRate = ev.SampleRate
	libhEv.Timestamp = ev.Timestamp
	libhEv.APIToken = ev.APIToken
	libhEv.APITenantId = ev.APITenantId
	// metadata is used to make error logs more helpful when processing libhoney responses
	metadata := map[string]any{
		"api_host":    ev.APIHost,
		"dataset":     ev.Dataset,
		"environment": ev.Environment,
	}

	for _, k := range d.Config.GetAdditionalErrorFields() {
		if v, ok := ev.Data[k]; ok {
			metadata[k] = v
		}
	}
	libhEv.Metadata = metadata

	for k, v := range ev.Data {
		libhEv.AddField(k, v)
	}

	err := libhEv.SendPresampled()
	if err != nil {
		d.Metrics.Increment(d.Name + counterEnqueueErrors)
		d.Logger.Error().
			WithString("error", err.Error()).
			WithField("request_id", ev.Context.Value(types.RequestIDContextKey{})).
			WithString("dataset", ev.Dataset).
			WithString("api_host", ev.APIHost).
			WithString("environment", ev.Environment).
			Logf("failed to enqueue event")
	}
}

func (d *DefaultTransmission) EnqueueSpan(sp *types.Span) {
	// we don't need the trace ID anymore, but it's convenient to accept spans.
	d.EnqueueEvent(&sp.Event)
}

func (d *DefaultTransmission) Flush() {
	d.LibhClient.Flush()
}

func (d *DefaultTransmission) Stop() error {
	// signal processResponses to stop
	if d.responseCanceler != nil {
		d.responseCanceler()
	}
	// purge the queue of any in-flight events
	d.LibhClient.Flush()
	return nil
}

func (d *DefaultTransmission) processResponses(
	ctx context.Context,
	responses chan transmission.Response,
) {
	for {
		select {
		case r := <-responses:
			if r.Err != nil || r.StatusCode > 202 {
				var apiHost, dataset, environment string
				if metadata, ok := r.Metadata.(map[string]any); ok {
					apiHost, _ = metadata["api_host"].(string)
					dataset, _ = metadata["dataset"].(string)
					environment, _ = metadata["environment"].(string)
				}
				log := d.Logger.Error().WithFields(map[string]interface{}{
					"status_code": r.StatusCode,
					"api_host":    apiHost,
					"dataset":     dataset,
					"environment": environment,
				})
				for _, k := range d.Config.GetAdditionalErrorFields() {
					if v, ok := r.Metadata.(map[string]any)[k]; ok {
						log = log.WithField(k, v)
					}
				}
				if r.Err != nil {
					log = log.WithField("error", r.Err.Error())
				}
				log.Logf("error when sending event")
				d.Metrics.Increment(d.Name + counterResponseErrors)
			} else {
				d.Metrics.Increment(d.Name + counterResponse20x)
			}
		case <-ctx.Done():
			return
		}
	}
}
