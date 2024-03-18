package metrics

import "sync"

// MockMetrics collects metrics that were registered and changed to allow tests to
// verify expected behavior
type MockMetrics struct {
	Registrations     map[string]string
	CounterIncrements map[string]int
	GaugeRecords      map[string]float64
	Histograms        map[string][]float64

	lock sync.Mutex
}

func (m *MockMetrics) RegisterWithDescriptionLabels(name, metricType, desc string, labels []string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.Registrations[name] = metricType
}

func (m *MockMetrics) RegisterGauge(name string, labels []string, desc string) {
	// TODO implement me
	panic("implement me")
}

func (m *MockMetrics) RegisterCounter(name string, labels []string, desc string) {
	// TODO implement me
	panic("implement me")
}

func (m *MockMetrics) RegisterHistogram(name string, labels []string, desc string, buckets []float64) {
	// TODO implement me
	panic("implement me")
}

func (m *MockMetrics) GaugeWithLabels(name string, labels map[string]string, value float64) {
	// TODO implement me
	panic("implement me")
}

func (m *MockMetrics) IncrementWithLabels(name string, labels map[string]string) {
	// TODO implement me
	panic("implement me")
}

func (m *MockMetrics) HistogramWithLabels(name string, labels map[string]string, obs interface{}) {
	// TODO implement me
	panic("implement me")
}

func (m *MockMetrics) AddWithLabels(name string, labels map[string]string, value float64) {
	// TODO implement me
	panic("implement me")
}

// Start initializes all metrics or resets all metrics to zero
func (m *MockMetrics) Start() {
	m.Registrations = make(map[string]string)
	m.CounterIncrements = make(map[string]int)
	m.GaugeRecords = make(map[string]float64)
	m.Histograms = make(map[string][]float64)
}

func (m *MockMetrics) Register(name, metricType string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.Registrations[name] = metricType
}

func (m *MockMetrics) Increment(name string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.CounterIncrements[name] += 1
}

func (m *MockMetrics) Gauge(name string, val interface{}) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.GaugeRecords[name] = ConvertNumeric(val)
}

func (m *MockMetrics) Count(name string, val interface{}) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.CounterIncrements[name] += int(ConvertNumeric(val))
}

func (m *MockMetrics) Histogram(name string, val interface{}) {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, ok := m.Histograms[name]
	if !ok {
		m.Histograms[name] = []float64{}
	}
	m.Histograms[name] = append(m.Histograms[name], ConvertNumeric(val))
}
