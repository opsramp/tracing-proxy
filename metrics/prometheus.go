package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/golang/snappy"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/prompb"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/gorilla/mux"
	"github.com/jirs5/tracing-proxy/config"
	"github.com/jirs5/tracing-proxy/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type PromMetrics struct {
	Config config.Config `inject:""`
	Logger logger.Logger `inject:""`
	// metrics keeps a record of all the registered metrics so we can increment
	// them by name
	metrics map[string]interface{}
	lock    sync.RWMutex

	prefix string
}

func (p *PromMetrics) Start() error {
	p.Logger.Debug().Logf("Starting PromMetrics")
	defer func() { p.Logger.Debug().Logf("Finished starting PromMetrics") }()
	pc, err := p.Config.GetPrometheusMetricsConfig()
	if err != nil {
		return err
	}

	p.metrics = make(map[string]interface{})

	muxxer := mux.NewRouter()

	muxxer.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(pc.MetricsListenAddr, muxxer)
	return nil
}

// Register takes a name and a metric type. The type should be one of "counter",
// "gauge", or "histogram"
func (p *PromMetrics) Register(name string, metricType string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	newmet, exists := p.metrics[name]

	// don't attempt to add the metric again as this will cause a panic
	if exists {
		return
	}

	switch metricType {
	case "counter":
		newmet = promauto.NewCounter(prometheus.CounterOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      name,
		})
	case "gauge":
		newmet = promauto.NewGauge(prometheus.GaugeOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      name,
		})
	case "histogram":
		newmet = promauto.NewHistogram(prometheus.HistogramOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      name,
			// This is an attempt at a usable set of buckets for a wide range of metrics
			// 16 buckets, first upper bound of 1, each following upper bound is 4x the previous
			Buckets: prometheus.ExponentialBuckets(1, 4, 16),
		})
	}

	p.metrics[name] = newmet
}

// RegisterWithDescriptionLabels takes a name, a metric type, description, labels. The type should be one of "counter",
// "gauge", or "histogram"
func (p *PromMetrics) RegisterWithDescriptionLabels(name string, metricType string, desc string, labels []string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	newmet, exists := p.metrics[name]

	// don't attempt to add the metric again as this will cause a panic
	if exists {
		return
	}

	switch metricType {
	case "counter":
		newmet = promauto.NewCounterVec(prometheus.CounterOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      desc,
		}, labels)
	case "gauge":
		newmet = promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name:      name,
				Namespace: p.prefix,
				Help:      desc,
			},
			labels)
	case "histogram":
		newmet = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:      name,
			Namespace: p.prefix,
			Help:      desc,
			// This is an attempt at a usable set of buckets for a wide range of metrics
			// 16 buckets, first upper bound of 1, each following upper bound is 4x the previous
			Buckets: prometheus.ExponentialBuckets(1, 4, 16),
		}, labels)

	}

	p.metrics[name] = newmet
}

func (p *PromMetrics) Increment(name string) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if counterIface, ok := p.metrics[name]; ok {
		if counter, ok := counterIface.(prometheus.Counter); ok {
			counter.Inc()
		}
	}
}
func (p *PromMetrics) Count(name string, n interface{}) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if counterIface, ok := p.metrics[name]; ok {
		if counter, ok := counterIface.(prometheus.Counter); ok {
			counter.Add(ConvertNumeric(n))
		}
	}
}
func (p *PromMetrics) Gauge(name string, val interface{}) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if gaugeIface, ok := p.metrics[name]; ok {
		if gauge, ok := gaugeIface.(prometheus.Gauge); ok {
			gauge.Set(ConvertNumeric(val))
		}
	}
}
func (p *PromMetrics) Histogram(name string, obs interface{}) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if histIface, ok := p.metrics[name]; ok {
		if hist, ok := histIface.(prometheus.Histogram); ok {
			hist.Observe(ConvertNumeric(obs))
		}
	}
}

func (p *PromMetrics) GaugeWithLabels(name string, labels map[string]string, value float64) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if gaugeIface, ok := p.metrics[name]; ok {
		if gaugeVec, ok := gaugeIface.(*prometheus.GaugeVec); ok {
			gaugeVec.With(labels).Set(value)
		}
	}
}

func (p *PromMetrics) IncrementWithLabels(name string, labels map[string]string) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if gaugeIface, ok := p.metrics[name]; ok {
		if gaugeVec, ok := gaugeIface.(*prometheus.CounterVec); ok {
			gaugeVec.With(labels).Inc()
		}
	}
}

func PushMetricsToOpsRamp(apiEndpoint, tenantID string, oauthToken OpsRampAuthTokenResponse) (int, error) {
	metricFamilySlice, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return -1, err
	}

	timeSeries := []*prompb.TimeSeries{}

	for _, metricFamily := range metricFamilySlice {

		for _, metric := range metricFamily.GetMetric() {
			samples := []prompb.Sample{}
			labels := []*prompb.Label{
				{
					Name:  "__name__",
					Value: metricFamily.GetName(),
				},
			}
			for _, label := range metric.GetLabel() {
				labels = append(labels, &prompb.Label{
					Name:  label.GetName(),
					Value: label.GetValue(),
				})
			}

			switch metricFamily.GetType() {
			case io_prometheus_client.MetricType_COUNTER:
				samples = append(samples, prompb.Sample{
					Value:     metric.GetCounter().GetValue(),
					Timestamp: time.Now().UnixMilli(),
				})
			case io_prometheus_client.MetricType_GAUGE:
				samples = append(samples, prompb.Sample{
					Value:     metric.GetGauge().GetValue(),
					Timestamp: time.Now().UnixMilli(),
				})

			}
			timeSeries = append(timeSeries, &prompb.TimeSeries{Labels: labels, Samples: samples})
		}

	}

	request := prompb.WriteRequest{Timeseries: timeSeries}

	out, err := proto.Marshal(&request)
	if err != nil {
		return -1, err
	}

	compressed := snappy.Encode(nil, out)

	URL := fmt.Sprintf("%s/metricsql/api/v7/tenants/%s/metrics", strings.TrimRight(apiEndpoint, "/"), tenantID)

	req, err := http.NewRequest(http.MethodPost, URL, bytes.NewBuffer(compressed))
	if err != nil {
		return -1, err
	}

	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	req.Header.Set("Connection", "close")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")

	if !strings.Contains(oauthToken.Scope, "metrics:write") {
		return -1, fmt.Errorf("auth token provided not not have metrics:write scope")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", oauthToken.AccessToken))

	client := http.Client{Timeout: time.Duration(10) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()
	// Depending on version and configuration of the PGW, StatusOK or StatusAccepted may be returned.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := ioutil.ReadAll(resp.Body) // Ignore any further error as this is for an error message only.
		return resp.StatusCode, fmt.Errorf("unexpected status code %d while pushing: %s", resp.StatusCode, body)
	}

	return resp.StatusCode, nil
}

type OpsRampAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

func GetOpsRampOAuthToken(apiEndpoint, apiKey, apiSecret string) (*OpsRampAuthTokenResponse, error) {

	authTokenResponse := new(OpsRampAuthTokenResponse)

	url := fmt.Sprintf("%s/auth/oauth/token", strings.TrimRight(apiEndpoint, "/"))

	requestBody := strings.NewReader("client_id=" + apiKey + "&client_secret=" + apiSecret + "&grant_type=client_credentials")

	req, err := http.NewRequest(http.MethodPost, url, requestBody)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	req.Header.Set("Connection", "close")

	client := http.Client{Timeout: time.Duration(10) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	err = json.Unmarshal(respBody, authTokenResponse)
	if err != nil {
		return nil, err
	}

	return authTokenResponse, nil
}
