package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang/snappy"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

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
	muxer  *mux.Router
	server *http.Server
)

func init() {
	muxer = mux.NewRouter()
}

type OpsRampMetrics struct {
	Config config.Config `inject:""`
	Logger logger.Logger `inject:""`
	// metrics keeps a record of all the registered metrics so that we can increment
	// them by name
	metrics map[string]interface{}
	lock    sync.RWMutex

	Client http.Client

	apiEndpoint string
	tenantID    string
	retryCount  int64
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

	p.metrics = make(map[string]interface{})

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
	server = &http.Server{
		Addr:              metricsConfig.ListenAddr,
		Handler:           muxer,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			p.Logger.Error().Logf("failed to start metrics server: %v", err)
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

			for range metricsTicker.C {
				statusCode, err := p.PushMetrics()
				if err != nil {
					p.Logger.Error().Logf("error while pushing metrics with statusCode: %d and Error: %v", statusCode, err)
					if err.Error() == missingMetricsWriteScope {
						p.Logger.Info().Logf("renewing auth token since the existing token is missing metrics:write scope")
						err := p.RenewOAuthToken()
						if err != nil {
							p.Logger.Error().Logf("error while initializing oAuth Token Err: %v", err)
						}
					}
				}
			}
		}()
	}

	return nil
}

// Register takes a name and a metric type. The type should be one of "counter",
// "gauge", or "histogram"
func (p *OpsRampMetrics) Register(name string, metricType string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	newMetric, exists := p.metrics[name]

	// don't attempt to add the metric again as this will cause a panic
	if exists {
		return
	}

	constantLabels := make(map[string]string)

	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		constantLabels["hostname"] = hostname
	}

	switch metricType {
	case "counter":
		newMetric = promauto.NewCounter(prometheus.CounterOpts{
			Name:        name,
			Namespace:   p.prefix,
			Help:        name,
			ConstLabels: constantLabels,
		})
	case "gauge":
		newMetric = promauto.NewGauge(prometheus.GaugeOpts{
			Name:        name,
			Namespace:   p.prefix,
			Help:        name,
			ConstLabels: constantLabels,
		})
	case "histogram":
		newMetric = promauto.NewHistogram(prometheus.HistogramOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      name,
			// This is an attempt at a usable set of buckets for a wide range of metrics
			// 16 buckets, first upper bound of 1, each following upper bound is 4x the previous
			Buckets:     prometheus.ExponentialBuckets(1, 4, 16),
			ConstLabels: constantLabels,
		})
	}

	p.metrics[name] = newMetric
}

// RegisterWithDescriptionLabels takes a name, a metric type, description, labels. The type should be one of "counter",
// "gauge", or "histogram"
func (p *OpsRampMetrics) RegisterWithDescriptionLabels(name string, metricType string, desc string, labels []string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	newMetric, exists := p.metrics[name]

	// don't attempt to add the metric again as this will cause a panic
	if exists {
		return
	}
	hostMap := make(map[string]string)
	if hostname, err := os.Hostname(); err == nil && hostname != "" {

		hostMap["hostname"] = hostname
	}

	switch metricType {
	case "counter":
		newMetric = promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        name,
			Namespace:   p.prefix,
			Help:        desc,
			ConstLabels: hostMap,
		}, labels)
	case "gauge":
		newMetric = promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name:        name,
				Namespace:   p.prefix,
				Help:        desc,
				ConstLabels: hostMap,
			},
			labels)
	case "histogram":
		newMetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        name,
			Namespace:   p.prefix,
			Help:        desc,
			ConstLabels: hostMap,
			// This is an attempt at a usable set of buckets for a wide range of metrics
			// 16 buckets, first upper bound of 1, each following upper bound is 4x the previous
			Buckets: prometheus.ExponentialBuckets(1, 4, 16),
		}, labels)

	}

	p.metrics[name] = newMetric
}

func (p *OpsRampMetrics) Increment(name string) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if counterInterface, ok := p.metrics[name]; ok {
		if counter, ok := counterInterface.(prometheus.Counter); ok {
			counter.Inc()
		}
	}
}
func (p *OpsRampMetrics) Count(name string, n interface{}) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if counterInterface, ok := p.metrics[name]; ok {
		if counter, ok := counterInterface.(prometheus.Counter); ok {
			counter.Add(ConvertNumeric(n))
		}
	}
}
func (p *OpsRampMetrics) Gauge(name string, val interface{}) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if gaugeInterface, ok := p.metrics[name]; ok {
		if gauge, ok := gaugeInterface.(prometheus.Gauge); ok {
			gauge.Set(ConvertNumeric(val))
		}
	}
}
func (p *OpsRampMetrics) Histogram(name string, obs interface{}) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if histInterface, ok := p.metrics[name]; ok {
		if hist, ok := histInterface.(prometheus.Histogram); ok {
			hist.Observe(ConvertNumeric(obs))
		}
	}
}

func (p *OpsRampMetrics) GaugeWithLabels(name string, labels map[string]string, value float64) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if gaugeInterface, ok := p.metrics[name]; ok {
		if gaugeVec, ok := gaugeInterface.(*prometheus.GaugeVec); ok {
			gaugeVec.With(labels).Set(value)
		}
	}
}

func (p *OpsRampMetrics) IncrementWithLabels(name string, labels map[string]string) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if gaugeInterface, ok := p.metrics[name]; ok {
		if gaugeVec, ok := gaugeInterface.(*prometheus.CounterVec); ok {
			gaugeVec.With(labels).Inc()
		}
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
	proxyConfig := p.Config.GetProxyConfig()

	p.apiEndpoint = metricsConfig.OpsRampAPI
	p.retryCount = metricsConfig.RetryCount

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

	proxyUrl := ""
	if proxyConfig.Host != "" && proxyConfig.Protocol != "" {
		proxyUrl = fmt.Sprintf("%s://%s:%d/", proxyConfig.Protocol, proxyConfig.Host, proxyConfig.Port)
		if proxyConfig.Username != "" && proxyConfig.Password != "" {
			proxyUrl = fmt.Sprintf("%s://%s:%s@%s:%d", proxyConfig.Protocol, proxyConfig.Username, proxyConfig.Password, proxyConfig.Host, proxyConfig.Port)
			p.Logger.Debug().Logf("Using Authentication for ProxyConfiguration Communication for Metrics")
		}
	}

	p.Client = http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			MaxIdleConns:    10,
			MaxConnsPerHost: 10,
			IdleConnTimeout: 5 * time.Minute,
		},
		Timeout: time.Duration(240) * time.Second,
	}
	if proxyUrl != "" {
		proxyURL, err := url.Parse(proxyUrl)
		if err != nil {
			p.Logger.Error().Logf("skipping proxy err: %v", err)
		} else {
			p.Client = http.Client{
				Transport: &http.Transport{
					Proxy:           http.ProxyURL(proxyURL),
					MaxIdleConns:    10,
					MaxConnsPerHost: 10,
					IdleConnTimeout: 5 * time.Minute,
				},
				Timeout: time.Duration(240) * time.Second,
			}
		}
	}
}

func (p *OpsRampMetrics) PushMetrics() (int, error) {
	metricFamilySlice, err := p.promRegistry.Gather()
	if err != nil {
		return -1, err
	}

	presentTime := time.Now().UnixMilli()

	var timeSeries []prompb.TimeSeries

	for _, metricFamily := range metricFamilySlice {
		if !p.re.MatchString(metricFamily.GetName()) {
			continue
		}
		for _, metric := range metricFamily.GetMetric() {
			var labels []prompb.Label
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
				for _, bucket := range metric.GetHistogram().GetBucket() {
					timeSeries = append(timeSeries, prompb.TimeSeries{
						Labels: append(labels, []prompb.Label{
							{
								Name:  model.MetricNameLabel,
								Value: metricFamily.GetName(),
							},
							{
								Name:  model.BucketLabel,
								Value: fmt.Sprintf("%v", bucket.GetUpperBound()),
							},
						}...),
						Samples: []prompb.Sample{
							{
								Value:     float64(bucket.GetCumulativeCount()),
								Timestamp: presentTime,
							},
						},
					})
				}
				// samples for count and sum
				timeSeries = append(timeSeries, prompb.TimeSeries{
					Labels: append(labels, prompb.Label{
						Name:  model.MetricNameLabel,
						Value: fmt.Sprintf("%s_sum", metricFamily.GetName()),
					}),
					Samples: []prompb.Sample{
						{
							Value:     metric.GetHistogram().GetSampleSum(),
							Timestamp: presentTime,
						},
					},
				})
				timeSeries = append(timeSeries, prompb.TimeSeries{
					Labels: append(labels, prompb.Label{
						Name:  model.MetricNameLabel,
						Value: fmt.Sprintf("%s_count", metricFamily.GetName()),
					}),
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
						Labels: append(labels, []prompb.Label{
							{
								Name:  model.MetricNameLabel,
								Value: metricFamily.GetName(),
							},
							{
								Name:  model.QuantileLabel,
								Value: fmt.Sprintf("%v", quantile.GetQuantile()),
							},
						}...),
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
					Labels: append(labels, prompb.Label{
						Name:  model.MetricNameLabel,
						Value: fmt.Sprintf("%s_sum", metricFamily.GetName()),
					}),
					Samples: []prompb.Sample{
						{
							Value:     metric.GetSummary().GetSampleSum(),
							Timestamp: presentTime,
						},
					},
				})
				timeSeries = append(timeSeries, prompb.TimeSeries{
					Labels: append(labels, prompb.Label{
						Name:  model.MetricNameLabel,
						Value: fmt.Sprintf("%s_count", metricFamily.GetName()),
					}),
					Samples: []prompb.Sample{
						{
							Value:     float64(metric.GetSummary().GetSampleCount()),
							Timestamp: presentTime,
						},
					},
				})
			}
		}
	}

	request := prompb.WriteRequest{Timeseries: timeSeries}

	out, err := proto.Marshal(&request)

	if err != nil {
		return -1, err
	}

	compressed := snappy.Encode(nil, out)

	URL := fmt.Sprintf("%s/metricsql/api/v7/tenants/%s/metrics", strings.TrimRight(p.apiEndpoint, "/"), p.tenantID)

	req, err := http.NewRequest(http.MethodPost, URL, bytes.NewBuffer(compressed))
	if err != nil {
		return -1, err
	}

	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")

	if !strings.Contains(p.oAuthToken.Scope, "metrics:write") {
		return -1, fmt.Errorf(missingMetricsWriteScope)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.oAuthToken.AccessToken))

	resp, err := p.SendWithRetry(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()
	// Depending on the version and configuration of the PGW, StatusOK or StatusAccepted may be returned.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.Logger.Error().Logf("failed to parse response body Err: %v", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return resp.StatusCode, fmt.Errorf("unexpected status code %d while pushing: %s", resp.StatusCode, body)
	}
	p.Logger.Debug().Logf("metrics push response: %v", string(body))

	return resp.StatusCode, nil
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
		return err
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

func (p *OpsRampMetrics) SendWithRetry(request *http.Request) (*http.Response, error) {
	response, err := p.Client.Do(request)
	if err == nil && response != nil && (response.StatusCode == http.StatusOK || response.StatusCode == http.StatusAccepted) {
		return response, nil
	}
	if response != nil && response.StatusCode == http.StatusProxyAuthRequired { // OpsRamp uses this for bad auth token
		p.RenewOAuthToken()
		request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.oAuthToken.AccessToken))
	}

	// retry if the error is not nil
	for retries := p.retryCount; retries > 0; retries-- {
		response, err = p.Client.Do(request)
		if err == nil && response != nil && (response.StatusCode == http.StatusOK || response.StatusCode == http.StatusAccepted) {
			return response, nil
		}
		if response != nil && response.StatusCode == http.StatusProxyAuthRequired { // OpsRamp uses this for bad auth token
			p.RenewOAuthToken()
			request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.oAuthToken.AccessToken))
		}
	}

	return response, err
}
