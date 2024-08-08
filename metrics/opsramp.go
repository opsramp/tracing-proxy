package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/prometheus/common/model"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang/snappy"
	"github.com/gorilla/mux"
	"github.com/opsramp/tracing-proxy/pkg/utils"
	"github.com/opsramp/tracing-proxy/proxy"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/prompb"

	"github.com/gogo/protobuf/proto"
	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	missingMetricsWriteScope = "auth token provided not not have metrics:write scope"
)

var (
	muxer     *mux.Router
	server    *http.Server
	serverMut sync.Mutex
	hostname  string
)
var timeSeriesChannel = make(chan []prompb.TimeSeries)

func init() {
	muxer = mux.NewRouter()

	hostname, _ = os.Hostname()
}

type metricType int

const (
	GAUGE metricType = iota
	COUNTER
	HISTOGRAM
	SUMMARY
)

const labelValuesDelimiter = "#*#"

type metricData struct {
	Data        interface{}
	Type        metricType
	Labels      []string                           // always should be in sorted order
	LabelValues utils.SyncedMap[string, time.Time] // key must be sorted in the same order as labels
}

type OpsRampMetrics struct {
	Config config.Config `inject:""`
	Logger logger.Logger `inject:""`
	// metrics keeps a record of all the registered metrics so that we can increment
	// them by name
	metrics map[string]*metricData

	lock sync.RWMutex

	Client *http.Client
	Proxy  *proxy.Proxy `inject:"proxyConfig"`

	apiEndpoint string
	tenantID    string
	re          *regexp.Regexp
	prefix      string

	authTokenEndpoint string
	apiKey            string
	apiSecret         string
	oAuthToken        *OpsRampAuthTokenResponse

	promRegistry *prometheus.Registry
}

func (p *OpsRampMetrics) Start() error {
	p.Logger.Debug().Logf("Starting OpsRampMetrics")
	defer func() { p.Logger.Debug().Logf("Finished starting OpsRampMetrics") }()

	metricsConfig := p.Config.GetMetricsConfig()

	p.metrics = make(map[string]*metricData)

	// Create non-global registry.
	p.promRegistry = prometheus.NewRegistry()

	// Add go runtime metrics and process collectors to default metrics prefix
	if p.prefix == "" {
		p.promRegistry.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)
	}

	listenURI := "/metrics"
	if p.prefix != "" {
		listenURI = fmt.Sprintf("/metrics/%s", strings.TrimSpace(p.prefix))
	}
	muxer.Handle(listenURI, promhttp.HandlerFor(
		p.promRegistry,
		promhttp.HandlerOpts{Registry: p.promRegistry, Timeout: 10 * time.Second},
	),
	)
	p.Logger.Info().Logf("registered metrics at %s for prefix: %s", listenURI, p.prefix)

	if server != nil {
		err := server.Shutdown(context.Background())
		if err != nil {
			p.Logger.Error().Logf("metrics server shutdown: %v", err)
		}
	}
	serverMut.Lock() // nolint:all // cant unlock with defer since that will release the lock once the goroutine spins up
	server = &http.Server{
		Addr:              metricsConfig.ListenAddr,
		Handler:           muxer,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		defer serverMut.Unlock()
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			p.Logger.Error().Logf("%v", err)
		}
	}()

	if p.Config.GetSendMetricsToOpsRamp() {
		go func() {
			metricsTicker := time.NewTicker(time.Duration(metricsConfig.ReportingInterval) * time.Second)
			defer metricsTicker.Stop()
			p.Populate()

			// populating the oAuth Token Initially
			err := p.RenewOAuthToken()
			if err != nil {
				p.Logger.Error().Logf("error while initializing oAuth Token Err: %v", err)
			}

			// Start timeseries listener thread
			go p.ListenTimeseries()

			for range metricsTicker.C {
				p.Push()
			}
		}()
	}

	return nil
}

func (p *OpsRampMetrics) ListenTimeseries() {

	//for timeSeries := range timeSeriesChannel {
	//	// call request push with timeSeries payload
	//	p.frameRequest(timeSeries)
	//}
	for {
		select {
		case timeSeries, ok := <-timeSeriesChannel:
			if ok {
				if timeSeries != nil {
					p.frameRequest(timeSeries)
				}
			} else {
				p.Logger.Error().Logf("timeSeries channel error")
			}
		}
	}

}

// Register takes a name and a metric type. The type should be one of "counter",
// "gauge", or "histogram"
func (p *OpsRampMetrics) Register(name, metricType string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	_, exists := p.metrics[name]

	// don't attempt to add the metric again as this will cause a panic
	if exists {
		return
	}

	newMetric := &metricData{
		Data:        nil,
		LabelValues: utils.SyncedMap[string, time.Time]{},
	}

	switch metricType {
	case "counter":
		counterMet := promauto.With(p.promRegistry).NewCounter(prometheus.CounterOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      name,
		})

		newMetric.Data = counterMet
		newMetric.Type = COUNTER
	case "gauge":
		gaugeMet := promauto.With(p.promRegistry).NewGauge(prometheus.GaugeOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      name,
		})

		newMetric.Data = gaugeMet
		newMetric.Type = GAUGE
	case "histogram":
		histogramMet := promauto.With(p.promRegistry).NewHistogram(prometheus.HistogramOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      name,
			// This is an attempt at a usable set of buckets for a wide range of metrics
			// 16 buckets, first upper bound of 1, each following upper bound is 4x the previous
			Buckets: prometheus.ExponentialBuckets(1, 4, 16),
		})

		newMetric.Data = histogramMet
		newMetric.Type = HISTOGRAM
	}

	p.metrics[name] = newMetric
}

// RegisterWithDescriptionLabels takes a name, a metric type, description, labels. The type should be one of "counter",
// "gauge", or "histogram"
func (p *OpsRampMetrics) RegisterWithDescriptionLabels(name, metricType, desc string, labels []string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	_, exists := p.metrics[name]

	// don't attempt to add the metric again as this will cause a panic
	if exists {
		return
	}

	// sorting the labels in alphabetical order
	sort.Strings(labels)

	newMetric := &metricData{
		Data:        nil,
		Labels:      labels,
		LabelValues: utils.SyncedMap[string, time.Time]{},
	}

	switch metricType {
	case "counter":
		counterMet := promauto.With(p.promRegistry).NewCounterVec(prometheus.CounterOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      desc,
		}, labels)

		newMetric.Type = COUNTER
		newMetric.Data = counterMet
	case "gauge":
		gaugeMet := promauto.With(p.promRegistry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name:      name,
				Namespace: p.prefix,
				Help:      desc,
			},
			labels)

		newMetric.Type = GAUGE
		newMetric.Data = gaugeMet
	case "histogram":
		histogramMet := promauto.With(p.promRegistry).NewHistogramVec(prometheus.HistogramOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      desc,
			// This is an attempt at a usable set of buckets for a wide range of metrics
			// 16 buckets, first upper bound of 1, each following upper bound is 4x the previous
			Buckets: prometheus.ExponentialBuckets(1, 4, 16),
		}, labels)

		newMetric.Type = HISTOGRAM
		newMetric.Data = histogramMet
	}

	p.metrics[name] = newMetric
}

func (p *OpsRampMetrics) RegisterGauge(name string, labels []string, desc string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	_, exists := p.metrics[name]
	// don't attempt to add the metric again as this will cause a panic
	if exists {
		return
	}

	// sorting the labels in alphabetical order
	sort.Strings(labels)

	newMetric := &metricData{
		Data:        nil,
		Labels:      labels,
		LabelValues: utils.SyncedMap[string, time.Time]{},
	}

	newMetric.Type = GAUGE
	newMetric.Data = promauto.With(p.promRegistry).NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      desc,
		},
		labels)

	p.metrics[name] = newMetric
}

func (p *OpsRampMetrics) RegisterCounter(name string, labels []string, desc string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	_, exists := p.metrics[name]
	// don't attempt to add the metric again as this will cause a panic
	if exists {
		return
	}

	// sorting the labels in alphabetical order
	sort.Strings(labels)

	newMetric := &metricData{
		Data:        nil,
		Labels:      labels,
		LabelValues: utils.SyncedMap[string, time.Time]{},
	}

	newMetric.Type = COUNTER
	newMetric.Data = promauto.With(p.promRegistry).NewCounterVec(
		prometheus.CounterOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      desc,
		},
		labels)

	p.metrics[name] = newMetric
}

func (p *OpsRampMetrics) RegisterHistogram(name string, labels []string, desc string, buckets []float64) {
	p.lock.Lock()
	defer p.lock.Unlock()

	_, exists := p.metrics[name]
	// don't attempt to add the metric again as this will cause a panic
	if exists {
		return
	}

	// sorting the labels in alphabetical order
	sort.Strings(labels)

	newMetric := &metricData{
		Data:        nil,
		Labels:      labels,
		LabelValues: utils.SyncedMap[string, time.Time]{},
	}

	newMetric.Type = HISTOGRAM
	newMetric.Data = promauto.With(p.promRegistry).NewHistogramVec(prometheus.HistogramOpts{
		Name:      name,
		Namespace: p.prefix,
		Help:      desc,
		Buckets:   buckets,
	}, labels)

	p.metrics[name] = newMetric
}

func (p *OpsRampMetrics) Increment(name string) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if metData, ok := p.metrics[name]; ok && metData.Data != nil {
		if counter, ok := metData.Data.(prometheus.Counter); ok {
			counter.Inc()
		}
		metData.LabelValues.Set("", time.Now().UTC())
	}
}

func (p *OpsRampMetrics) Count(name string, n interface{}) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if metData, ok := p.metrics[name]; ok && metData.Data != nil {
		if counter, ok := metData.Data.(prometheus.Counter); ok {
			counter.Add(ConvertNumeric(n))
		}
		metData.LabelValues.Set("", time.Now().UTC())
	}
}

func (p *OpsRampMetrics) Gauge(name string, val interface{}) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if metData, ok := p.metrics[name]; ok && metData.Data != nil {
		if gauge, ok := metData.Data.(prometheus.Gauge); ok {
			gauge.Set(ConvertNumeric(val))
		}
		metData.LabelValues.Set("", time.Now().UTC())
	}
}

func (p *OpsRampMetrics) Histogram(name string, obs interface{}) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if metData, ok := p.metrics[name]; ok && metData.Data != nil {
		if hist, ok := metData.Data.(prometheus.Histogram); ok {
			hist.Observe(ConvertNumeric(obs))
		}
		metData.LabelValues.Set("", time.Now().UTC())
	}
}

func (p *OpsRampMetrics) HistogramWithLabels(name string, labels map[string]string, obs interface{}) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if metData, ok := p.metrics[name]; ok && metData.Data != nil {
		if histVec, ok := metData.Data.(*prometheus.HistogramVec); ok {
			histVec.With(labels).Observe(ConvertNumeric(obs))
		}
		vals := getLabelValues(metData.Labels, labels)
		key := strings.Join(vals, labelValuesDelimiter)
		metData.LabelValues.Set(key, time.Now().UTC())
	}
}

func (p *OpsRampMetrics) GaugeWithLabels(name string, labels map[string]string, value float64) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if metData, ok := p.metrics[name]; ok && metData.Data != nil {
		if gaugeVec, ok := metData.Data.(*prometheus.GaugeVec); ok {
			gaugeVec.With(labels).Set(value)
		}
		vals := getLabelValues(metData.Labels, labels)
		key := strings.Join(vals, labelValuesDelimiter)
		metData.LabelValues.Set(key, time.Now().UTC())
	}
}

func (p *OpsRampMetrics) AddWithLabels(name string, labels map[string]string, value float64) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if metData, ok := p.metrics[name]; ok && metData.Data != nil {
		if counterVec, ok := metData.Data.(*prometheus.CounterVec); ok {
			counterVec.With(labels).Add(value)
		}
		vals := getLabelValues(metData.Labels, labels)
		key := strings.Join(vals, labelValuesDelimiter)
		metData.LabelValues.Set(key, time.Now().UTC())
	}
}

func (p *OpsRampMetrics) IncrementWithLabels(name string, labels map[string]string) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if metData, ok := p.metrics[name]; ok && metData.Data != nil {
		if counterVec, ok := metData.Data.(*prometheus.CounterVec); ok {
			counterVec.With(labels).Inc()
		}
		vals := getLabelValues(metData.Labels, labels)
		key := strings.Join(vals, labelValuesDelimiter)
		metData.LabelValues.Set(key, time.Now().UTC())
	}
}

func (p *OpsRampMetrics) gaugeDeleteLabelValues(name string, labelVals []string) {
	if metData, ok := p.metrics[name]; ok && metData.Data != nil {
		if gaugeVec, ok := metData.Data.(*prometheus.GaugeVec); ok {
			gaugeVec.DeleteLabelValues(labelVals...)
		}
		key := strings.Join(labelVals, labelValuesDelimiter)
		metData.LabelValues.Delete(key)
	}
}

func (p *OpsRampMetrics) counterDeleteLabelValues(name string, labelVals []string) {
	if metData, ok := p.metrics[name]; ok && metData.Data != nil {
		if counterVec, ok := metData.Data.(*prometheus.CounterVec); ok {
			counterVec.DeleteLabelValues(labelVals...)
		}
		key := strings.Join(labelVals, labelValuesDelimiter)
		metData.LabelValues.Delete(key)
	}
}

func (p *OpsRampMetrics) histogramDeleteLabelValues(name string, labelVals []string) {
	if metData, ok := p.metrics[name]; ok && metData.Data != nil {
		if histogramVec, ok := metData.Data.(*prometheus.HistogramVec); ok {
			histogramVec.DeleteLabelValues(labelVals...)
		}
		key := strings.Join(labelVals, labelValuesDelimiter)
		metData.LabelValues.Delete(key)
	}
}

type OpsRampAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

func (p *OpsRampMetrics) Populate() {
	metricsConfig := p.Config.GetMetricsConfig()
	authConfig := p.Config.GetAuthConfig()

	p.apiEndpoint = metricsConfig.OpsRampAPI

	p.authTokenEndpoint = authConfig.Endpoint
	p.apiKey = authConfig.Key
	p.apiSecret = authConfig.Secret
	p.tenantID = authConfig.TenantId

	// Creating Regex for a list of metrics
	regexString := ".*" // the default value is to take everything
	if len(metricsConfig.MetricsList) >= 1 {
		regexString = metricsConfig.MetricsList[0]
		for index := 0; index < len(metricsConfig.MetricsList); index++ {
			regexString = fmt.Sprintf("%s|%s", regexString, metricsConfig.MetricsList[index])
		}
	}
	p.re = regexp.MustCompile(regexString)

	_ = p.Proxy.UpdateProxyEnvVars()

	p.RenewClient()
}

func ConvertLabelsToMap(labels []prompb.Label) map[string]string {
	labelMap := make(map[string]string)
	for _, label := range labels {
		labelMap[label.Name] = label.Value
	}
	return labelMap
}

func (p *OpsRampMetrics) calculateTraceOperationError(metricFamilySlice []*io_prometheus_client.MetricFamily) {
	var labelMap map[string]string
	uniqueLabelsMap := make(map[string][]prompb.Label)
	uniqueFailedMap := make(map[string]float64)
	uniqueSpansMap := make(map[string]float64)
	for _, metricFamily := range metricFamilySlice {
		if !p.re.MatchString(metricFamily.GetName()) {
			continue
		}
		if metricFamily.GetName() == "trace_operations_failed" || metricFamily.GetName() == "trace_spans_count" {
			for _, metric := range metricFamily.GetMetric() {
				var labels []prompb.Label
				for _, label := range metric.GetLabel() {
					labels = append(labels, prompb.Label{
						Name:  label.GetName(),
						Value: label.GetValue(),
					})
				}
				key := "trace_operations_failed&trace_spans_count&"
				labelSlice := metric.GetLabel()
				sort.Slice(labelSlice, func(i, j int) bool {
					return labelSlice[i].GetName()+labelSlice[i].GetValue() > labelSlice[j].GetName()+labelSlice[i].GetValue()
				})
				for _, label := range labelSlice {
					key += label.GetName() + label.GetValue()
				}
				if metricFamily.GetName() == "trace_operations_failed" {
					uniqueFailedMap[key] = *metric.Counter.Value
				} else {
					uniqueSpansMap[key] = *metric.Counter.Value
				}
				uniqueLabelsMap[key] = labels
			}
		}
	}
	for key := range uniqueLabelsMap {
		labelMap = ConvertLabelsToMap(uniqueLabelsMap[key])
		p.GaugeWithLabels("trace_operations_error", labelMap, uniqueFailedMap[key]/uniqueSpansMap[key])
	}
}

func (p *OpsRampMetrics) Push() {
	metricsConfig := p.Config.GetMetricsConfig()

	// setting up default values and removing metrics older than 5 minutes
	p.lock.Lock() // nolint: all // no linting here since we need to release the lock sooner than end of function

	for metricName, metData := range p.metrics {
		for labelValStr, t := range metData.LabelValues.Copy() {
			timeDiff := time.Now().UTC().Sub(t)
			if timeDiff < time.Duration(metricsConfig.ReportingInterval)*time.Second*2 {
				continue
			}

			labelVals := strings.Split(labelValStr, labelValuesDelimiter)
			switch metData.Type {
			case GAUGE:
				if timeDiff > time.Duration(metricsConfig.ReportingInterval)*time.Second*2 {
					if gaugeVec, ok := metData.Data.(*prometheus.GaugeVec); ok {
						gaugeVec.WithLabelValues(labelVals...).Set(0)
					}
				}

				if timeDiff > time.Minute*15 {
					p.Logger.Error().WithField("metric: ", metricName).
						WithField("Label Values: ", labelVals).
						Logf("Deleting a Gauge metric for ")
					p.gaugeDeleteLabelValues(metricName, labelVals)
				}
			case COUNTER:
				if timeDiff > time.Hour*24 {
					p.Logger.Error().WithField("metric: ", metricName).
						WithField("Label Values: ", labelVals).
						Logf("Deleting a Counter metric for ")
					p.counterDeleteLabelValues(metricName, labelVals)
				}
			case HISTOGRAM:
				if timeDiff > time.Hour*24 {
					p.Logger.Error().WithField("metric: ", metricName).
						WithField("Label Values: ", labelVals).
						Logf("Deleting a Histogram metric for ")
					p.histogramDeleteLabelValues(metricName, labelVals)
				}
			}
		}
	}

	p.lock.Unlock()

	metricFamilySlice, err := p.promRegistry.Gather()
	if err != nil {
		p.Logger.Error().Logf("Error during gather: %v", err)
		return
	}

	presentTime := time.Now().UnixMilli()

	var timeSeries []prompb.TimeSeries

	for _, metricFamily := range metricFamilySlice {
		if !p.re.MatchString(metricFamily.GetName()) {
			continue
		}
		var metricsLength int
		var hasSent bool
		for _, metric := range metricFamily.GetMetric() {
			metricLength := len(metric.String())
			if metricLength > 250000 {
				p.Logger.Info().Logf("Metric size is too big (%d characters)", metricLength) // larger than my tummy
				continue
			}
			if metricsLength+metricLength > 250000 {
				hasSent = true
				go func() {
					if len(timeSeries) > 0 {
						timeSeriesChannel <- timeSeries // Push timeSeries payload to channel
					}
				}()
				metricsLength = 0
				timeSeries = []prompb.TimeSeries{}
			}
			metricsLength += metricLength
			labels := []prompb.Label{
				{
					Name:  model.JobLabel,
					Value: p.prefix,
				},
				{
					Name:  "hostname",
					Value: hostname,
				},
			}
			for _, label := range metric.GetLabel() {
				labels = append(labels, prompb.Label{
					Name:  label.GetName(),
					Value: label.GetValue(),
				})
			}

			switch metricFamily.GetType() {
			case io_prometheus_client.MetricType_COUNTER:
				timeSeries = append(timeSeries, prompb.TimeSeries{
					Labels: append(labels, prompb.Label{
						Name:  model.MetricNameLabel,
						Value: metricFamily.GetName(),
					}),
					Samples: []prompb.Sample{
						{
							Value:     metric.GetCounter().GetValue(),
							Timestamp: presentTime,
						},
					},
				})
			case io_prometheus_client.MetricType_GAUGE:
				timeSeries = append(timeSeries, prompb.TimeSeries{
					Labels: append(labels, prompb.Label{
						Name:  model.MetricNameLabel,
						Value: metricFamily.GetName(),
					}),
					Samples: []prompb.Sample{
						{
							Value:     metric.GetGauge().GetValue(),
							Timestamp: presentTime,
						},
					},
				})
			case io_prometheus_client.MetricType_HISTOGRAM:
				// samples for all the buckets
				buckets := metric.GetHistogram().GetBucket()
				for index := range buckets {
					timeSeries = append(timeSeries, prompb.TimeSeries{
						Labels: append([]prompb.Label{
							{
								Name:  model.MetricNameLabel,
								Value: fmt.Sprintf("%s_bucket", metricFamily.GetName()),
							},
							{
								Name:  model.BucketLabel,
								Value: fmt.Sprintf("%v", buckets[index].GetUpperBound()),
							},
						}, labels...),
						Samples: []prompb.Sample{
							{
								Value:     float64(buckets[index].GetCumulativeCount()),
								Timestamp: presentTime,
							},
						},
					})
				}

				// samples for count and sum
				timeSeries = append(timeSeries, prompb.TimeSeries{
					Labels: append([]prompb.Label{
						{
							Name:  model.MetricNameLabel,
							Value: fmt.Sprintf("%s_sum", metricFamily.GetName()),
						},
					}, labels...),
					Samples: []prompb.Sample{
						{
							Value:     metric.GetHistogram().GetSampleSum(),
							Timestamp: presentTime,
						},
					},
				})
				timeSeries = append(timeSeries, prompb.TimeSeries{
					Labels: append([]prompb.Label{
						{
							Name:  model.MetricNameLabel,
							Value: fmt.Sprintf("%s_count", metricFamily.GetName()),
						},
					}, labels...),
					Samples: []prompb.Sample{
						{
							Value:     float64(metric.GetHistogram().GetSampleCount()),
							Timestamp: presentTime,
						},
					},
				})
			case io_prometheus_client.MetricType_SUMMARY:
				// samples for all the quantiles
				for _, quantile := range metric.GetSummary().GetQuantile() {
					timeSeries = append(timeSeries, prompb.TimeSeries{
						Labels: append([]prompb.Label{
							{
								Name:  model.MetricNameLabel,
								Value: metricFamily.GetName(),
							},
							{
								Name:  model.QuantileLabel,
								Value: fmt.Sprintf("%v", quantile.GetQuantile()),
							},
						}, labels...),
						Samples: []prompb.Sample{
							{
								Value:     quantile.GetValue(),
								Timestamp: presentTime,
							},
						},
					})
				}
				// samples for count and sum
				timeSeries = append(timeSeries, prompb.TimeSeries{
					Labels: append([]prompb.Label{
						{
							Name:  model.MetricNameLabel,
							Value: fmt.Sprintf("%s_sum", metricFamily.GetName()),
						},
					}, labels...),
					Samples: []prompb.Sample{
						{
							Value:     metric.GetSummary().GetSampleSum(),
							Timestamp: presentTime,
						},
					},
				})
				timeSeries = append(timeSeries, prompb.TimeSeries{
					Labels: append([]prompb.Label{
						{
							Name:  model.MetricNameLabel,
							Value: fmt.Sprintf("%s_count", metricFamily.GetName()),
						},
					}, labels...),
					Samples: []prompb.Sample{
						{
							Value:     float64(metric.GetSummary().GetSampleCount()),
							Timestamp: presentTime,
						},
					},
				})
			}
			hasSent = false
		}
		if !hasSent && len(timeSeries) > 0 {
			go func() {
				timeSeriesChannel <- timeSeries
			}()

		}
	}
}

func (p *OpsRampMetrics) frameRequest(timeSeries []prompb.TimeSeries) {

	request := prompb.WriteRequest{Timeseries: timeSeries}

	out, err := proto.Marshal(&request)
	if err != nil {
		p.Logger.Error().Logf("Unable to marshal the request: %v", err)
		return
	}

	compressed := snappy.Encode(nil, out)

	URL := fmt.Sprintf("%s/metricsql/api/v7/tenants/%s/metrics", strings.TrimRight(p.apiEndpoint, "/"), p.tenantID)

	req, err := http.NewRequest(http.MethodPost, URL, bytes.NewBuffer(compressed))
	if err != nil {
		p.Logger.Error().Logf("Unable to build request: %v", err)
		return
	}

	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")

	if !strings.Contains(p.oAuthToken.Scope, "metrics:write") {
		p.Logger.Error().Logf(missingMetricsWriteScope)
		p.RenewOAuthToken()
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.oAuthToken.AccessToken))

	resp, err := p.Send(req)
	if err != nil {
		p.Logger.Error().Logf("Unable to send request: %v", err)
		return
	}
	defer resp.Body.Close()
	// Depending on the version and configuration of the PGW, StatusOK or StatusAccepted may be returned.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.Logger.Error().Logf("failed to parse response body Err: %v", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		p.Logger.Error().Logf("unexpected status code %d while pushing: %s", resp.StatusCode, body)
		return
	}
	//p.Logger.Debug().Logf("metrics %s push response: %v", p.prefix, string(body))
}

func (p *OpsRampMetrics) RenewClient() {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.Client.CloseIdleConnections()

	p.Client = &http.Client{
		Transport: utils.CreateNewHTTPTransport(),
		Timeout:   time.Duration(240) * time.Second,
	}
}

func (p *OpsRampMetrics) RenewOAuthToken() error {
	p.oAuthToken = new(OpsRampAuthTokenResponse)

	endpoint := fmt.Sprintf("%s/auth/oauth/token", strings.TrimRight(p.authTokenEndpoint, "/"))

	requestBody := strings.NewReader("client_id=" + p.apiKey + "&client_secret=" + p.apiSecret + "&grant_type=client_credentials")

	req, err := http.NewRequest(http.MethodPost, endpoint, requestBody)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	req.Header.Set("Connection", "close")

	resp, err := p.Client.Do(req)
	if err != nil {
		retry := false
		if strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "unreachable") {
			if p.Proxy.Enabled() {
				_ = p.Proxy.SwitchProxy(p.prefix)
				p.RenewClient()
				retry = true
			}
		}

		if retry {
			resp, err = p.Client.Do(req)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	err = json.Unmarshal(respBody, p.oAuthToken)
	if err != nil {
		return err
	}

	return nil
}

func (p *OpsRampMetrics) Send(request *http.Request) (*http.Response, error) {
	var err error
	var response *http.Response
	retry := false
	response, err = p.Client.Do(request)
	if err == nil && response != nil && (response.StatusCode == http.StatusOK || response.StatusCode == http.StatusAccepted) {
		return response, nil
	}
	if err != nil &&
		(strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "unreachable")) {
		if p.Proxy.Enabled() {
			_ = p.Proxy.SwitchProxy(p.prefix)
			p.RenewClient()
			retry = true
		}
	}
	if retry {
		response, err = p.Client.Do(request)
	}

	if response != nil && response.StatusCode == http.StatusProxyAuthRequired { // OpsRamp uses this for bad auth token
		err = p.RenewOAuthToken()
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.oAuthToken.AccessToken))
		response, err = p.Client.Do(request)
		if err == nil && response != nil && (response.StatusCode == http.StatusOK || response.StatusCode == http.StatusAccepted) {
			return response, nil
		}
	}
	return response, err
}

func getLabelValues(l []string, m map[string]string) []string {
	var result []string
	for _, key := range l {
		result = append(result, m[key])
	}
	return result
}
