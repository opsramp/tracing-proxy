package sample

import (
	"os"
	"testing"

	"github.com/facebookgo/inject"
	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/logger"
	"github.com/opsramp/tracing-proxy/metrics"
	"github.com/stretchr/testify/assert"
)

func TestDependencyInjection(t *testing.T) {
	var g inject.Graph
	err := g.Provide(
		&inject.Object{Value: &SamplerFactory{}},

		&inject.Object{Value: &config.MockConfig{}},
		&inject.Object{Value: &logger.NullLogger{}},
		&inject.Object{Value: &metrics.NullMetrics{}, Name: "metrics"},
	)
	if err != nil {
		t.Error(err)
	}
	if err := g.Populate(); err != nil {
		t.Error(err)
	}
}

func TestDatasetPrefix(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configFile, err := os.CreateTemp(tmpDir, "*.yaml")
	assert.NoError(t, err)

	_, err = configFile.Write([]byte(`
DatasetPrefix: "dataset"
ListenAddr: 0.0.0.0:8080
GRPCListenAddr: 0.0.0.0:9090
PeerListenAddr: 0.0.0.0:8083
GRPCPeerListenAddr: 0.0.0.0:8084
OpsrampAPI: "https://portal.opsramp.net"
Dataset: "ds"
UseTls: false
UseTlsInsecure: true
AuthConfiguration: 
  Endpoint: "https://portal.opsramp.net"
  Key: "12345678900987654321"
  Secret: "123456789009876543211234567890098765432112345678900987654321"
  TenantId: "12345678900987654321-12345678900987654321-12345678900987654321"
InMemCollector:
  CacheCapacity: 1000
`))
	assert.NoError(t, err)
	configFile.Close()

	rulesFile, err := os.CreateTemp(tmpDir, "*.yaml")
	assert.NoError(t, err)

	_, err = rulesFile.Write([]byte(`
Sampler: DeterministicSampler
SampleRate: 1
production:
  Sampler: DeterministicSampler
  SampleRate: 10
dataset_1:
  production:
    Sampler: DeterministicSampler
    SampleRate: 20
`))
	assert.NoError(t, err)
	rulesFile.Close()

	c, err := config.NewConfig(configFile.Name(), rulesFile.Name(), func(err error) {})
	assert.NoError(t, err)

	assert.Equal(t, "dataset", c.GetDatasetPrefix())

	factory := SamplerFactory{Config: c, Logger: &logger.NullLogger{}, Metrics: &metrics.NullMetrics{}}

	defaultSampler := &DeterministicSampler{
		Config: &config.DeterministicSamplerConfig{SampleRate: 1},
		Logger: &logger.NullLogger{},
	}
	err = defaultSampler.Start()
	if err != nil {
		t.Error(err)
	}

	envSampler := &DeterministicSampler{
		Config: &config.DeterministicSamplerConfig{SampleRate: 10},
		Logger: &logger.NullLogger{},
	}
	if err := envSampler.Start(); err != nil {
		t.Error(err)
	}

	datasetSampler := &DeterministicSampler{
		Config: &config.DeterministicSamplerConfig{SampleRate: 20},
		Logger: &logger.NullLogger{},
	}
	if err := datasetSampler.Start(); err != nil {
		t.Error(err)
	}

	assert.Equal(t, defaultSampler, factory.GetSamplerImplementationForKey("unknown", false))              // nolint:all // test case
	assert.Equal(t, defaultSampler, factory.GetSamplerImplementationForKey("unknown", true))               // nolint:all // test case
	assert.Equal(t, envSampler, factory.GetSamplerImplementationForKey("production", false))               // nolint:all // test case
	assert.Equal(t, datasetSampler, factory.GetSamplerImplementationForKey("dataset_1.production", false)) // nolint:all // test case
}
