package metrics

// NullMetrics discards all metrics
type NullMetrics struct{}

// Start initializes all metrics or resets all metrics to zero
func (n *NullMetrics) Start() {}

func (n *NullMetrics) Register(name string, metricType string) {}
func (n *NullMetrics) Increment(name string)                   {}
func (n *NullMetrics) Gauge(name string, val interface{})      {}
func (n *NullMetrics) Count(name string, val interface{})      {}
func (n *NullMetrics) Histogram(name string, obs interface{})  {}
func (n *NullMetrics) RegisterWithDescriptionLabels(name string, metricType string, desc string, labels []string) {
}
func (n *NullMetrics) RegisterGauge(name string, labels []string, desc string)   {}
func (n *NullMetrics) RegisterCounter(name string, labels []string, desc string) {}
func (n *NullMetrics) RegisterHistogram(name string, labels []string, desc string, buckets []float64) {
}
func (n *NullMetrics) GaugeWithLabels(name string, labels map[string]string, value float64)       {}
func (n *NullMetrics) IncrementWithLabels(name string, labels map[string]string)                  {}
func (n *NullMetrics) AddWithLabels(name string, labels map[string]string, value float64)         {}
func (n *NullMetrics) HistogramWithLabels(name string, labels map[string]string, obs interface{}) {}
