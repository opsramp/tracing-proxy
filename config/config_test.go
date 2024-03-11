package config

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// creates two temporary toml files from the strings passed in and returns their filenames
func createTempConfigs(t *testing.T, configBody, rulesBody string) (string, string) {
	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)

	configFile, err := os.CreateTemp(tmpDir, "*.yaml")
	assert.NoError(t, err)

	if configBody != "" {
		_, err = configFile.WriteString(configBody)
		assert.NoError(t, err)
	}
	configFile.Close()

	rulesFile, err := os.CreateTemp(tmpDir, "*.yaml")
	assert.NoError(t, err)

	if rulesBody != "" {
		_, err = rulesFile.WriteString(rulesBody)
		assert.NoError(t, err)
	}
	rulesFile.Close()

	return configFile.Name(), rulesFile.Name()
}

func TestReload(t *testing.T) {
	config, rules := createTempConfigs(t, `
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
MetricsConfig:
  Enable: true
  ListenAddr: '0.0.0.0:2112'
  OpsRampAPI: "https://portal.opsramp.net"
  ReportingInterval: 10
  MetricsList: [ ".*" ]
`, "")
	defer os.Remove(rules)
	defer os.Remove(config)

	c, err := NewConfig(config, rules, func(err error) {})
	assert.NoError(t, err)

	if err != nil {
		t.Error(err)
	}

	if d, _ := c.GetListenAddr(); d != "0.0.0.0:8080" {
		t.Error("received", d, "expected", "0.0.0.0:8080")
	}

	wg := &sync.WaitGroup{}

	ch := make(chan interface{}, 1)

	c.RegisterReloadCallback(func() {
		close(ch)
	})

	// Hey race detector, we're doing some concurrent config reads.
	// That's cool, right?
	go func() {
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		for {
			c.GetListenAddr() // nolint:all // no need to handle error here since we are testing concurrent reads
			select {
			case <-ch:
				return
			case <-tick.C:
			}
		}
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Error("No callback")
		}
	}()

	cfg, err := os.ReadFile(config)
	if err != nil {
		t.Error(err)
	}
	s := string(cfg)
	s = strings.ReplaceAll(s, "ListenAddr: 0.0.0.0:8080", "ListenAddr: 0.0.0.0:9000")
	if file, err := os.OpenFile(config, os.O_RDWR, 0o644); err == nil {
		if err := file.Truncate(0); err != nil {
			t.Error(err)
		}
		if _, err := file.Seek(0, 0); err != nil {
			t.Error(err)
		}
		if _, err := file.WriteString(s); err != nil {
			t.Error(err)
		}
		file.Close() //nolint:all
	}

	wg.Wait()

	if d, _ := c.GetListenAddr(); d != "0.0.0.0:9000" {
		t.Error("received", d, "expected", "0.0.0.0:9000")
	}
}

func TestReadDefaults(t *testing.T) {
	c, err := NewConfig("../config_unit_tests.yaml", "../rules_unit_tests.yaml", func(err error) {})
	if err != nil {
		t.Error(err)
	}

	if d, _ := c.GetSendDelay(); d != 2*time.Second {
		t.Error("received", d, "expected", 2*time.Second)
	}

	if d, _ := c.GetTraceTimeout(); d != 60*time.Second {
		t.Error("received", d, "expected", 60*time.Second)
	}

	if d := c.GetSendTickerValue(); d != 100*time.Millisecond {
		t.Error("received", d, "expected", 100*time.Millisecond)
	}

	if d, _ := c.GetUseIPV6Identifier(); d != false {
		t.Error("received", d, "expected", false) // nolint:all // test case
	}

	if d := c.GetIsDryRun(); d != false {
		t.Error("received", d, "expected", false) // nolint:all // test case
	}

	if d := c.GetDryRunFieldName(); d != "trace_proxy_kept" {
		t.Error("received", d, "expected", "trace_proxy_kept")
	}

	if d := c.GetAddHostMetadataToTrace(); d != false {
		t.Error("received", d, "expected", false) // nolint:all // test case
	}

	if d := c.GetEnvironmentCacheTTL(); d != time.Hour {
		t.Error("received", d, "expected", time.Hour)
	}

	d, name, err := c.GetSamplerConfigForDataset("dataset-doesnt-exist")
	assert.NoError(t, err)
	assert.IsType(t, &DeterministicSamplerConfig{}, d)
	assert.Equal(t, "DeterministicSampler", name)

	collectorConfig, err := c.GetInMemCollectorCacheCapacity()
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, collectorConfig.CacheCapacity, 1000)
}

func TestReadRulesConfig(t *testing.T) {
	c, err := NewConfig("../config_unit_tests.yaml", "../rules_unit_tests.yaml", func(err error) {})
	if err != nil {
		t.Error(err)
	}

	d, name, err := c.GetSamplerConfigForDataset("dataset-doesnt-exist")
	assert.NoError(t, err)
	assert.IsType(t, &DeterministicSamplerConfig{}, d)
	assert.Equal(t, "DeterministicSampler", name)

	d, name, err = c.GetSamplerConfigForDataset("dataset1")
	assert.NoError(t, err)
	assert.IsType(t, &DynamicSamplerConfig{}, d)
	assert.Equal(t, "DynamicSampler", name)

	d, name, err = c.GetSamplerConfigForDataset("dataset4")
	assert.NoError(t, err)
	switch r := d.(type) {
	case *RulesBasedSamplerConfig:
		assert.Len(t, r.Rule, 6)

		var rule *RulesBasedSamplerRule

		rule = r.Rule[0]
		assert.True(t, rule.Drop)
		assert.Equal(t, 0, rule.SampleRate)
		assert.Len(t, rule.Condition, 1)

		rule = r.Rule[1]
		assert.Equal(t, 1, rule.SampleRate)
		assert.Equal(t, "keep slow 500 errors", rule.Name)
		assert.Len(t, rule.Condition, 2)

		rule = r.Rule[4]
		assert.Equal(t, 5, rule.SampleRate)
		assert.Equal(t, "span", rule.Scope)

		rule = r.Rule[5]
		assert.Equal(t, 10, rule.SampleRate)
		assert.Equal(t, "", rule.Scope)

		assert.Equal(t, "RulesBasedSampler", name)

	default:
		assert.Fail(t, "dataset4 should have a rules based sampler", d)
	}
}

func TestPeerManagementType(t *testing.T) {
	config, rules := createTempConfigs(t, `
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
MetricsConfig:
  Enable: true
  ListenAddr: '0.0.0.0:2112'
  OpsRampAPI: "https://portal.opsramp.net"
  ReportingInterval: 10
  MetricsList: [ ".*" ]
PeerManagement:
  Strategy: "hash"
  Type: "redis"
`, "")
	defer os.Remove(rules)
	defer os.Remove(config)

	c, err := NewConfig(config, rules, func(err error) {})
	assert.NoError(t, err)

	if d, _ := c.GetPeerManagementType(); d != "redis" {
		t.Error("received", d, "expected", "redis")
	}
}

func TestAbsentTraceKeyField(t *testing.T) {
	config, rules := createTempConfigs(t, `
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
MetricsConfig:
  Enable: true
  ListenAddr: '0.0.0.0:2112'
  OpsRampAPI: "https://portal.opsramp.net"
  ReportingInterval: 10
  MetricsList: [ ".*" ]
`, `
dataset1:
  Sampler: EMADynamicSampler
  GoalSampleRate: 10
  UseTraceLength: true
  AddSampleRateKeyToTrace: true
  FieldList: '[request.method]'
  Weight: 0.4
`)
	defer os.Remove(rules)
	defer os.Remove(config)

	_, err := NewConfig(config, rules, func(err error) {})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Error:Field validation for 'AddSampleRateKeyToTraceField'")
}

func TestDebugServiceAddr(t *testing.T) {
	config, rules := createTempConfigs(t, `
DebugServiceAddr: "localhost:8085"
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
MetricsConfig:
  Enable: true
  ListenAddr: '0.0.0.0:2112'
  OpsRampAPI: "https://portal.opsramp.net"
  ReportingInterval: 10
  MetricsList: [ ".*" ]
`, "")
	defer os.Remove(rules)
	defer os.Remove(config)

	c, err := NewConfig(config, rules, func(err error) {})
	assert.NoError(t, err)

	if d, _ := c.GetDebugServiceAddr(); d != "localhost:8085" {
		t.Error("received", d, "expected", "localhost:8085")
	}
}

func TestDryRun(t *testing.T) {
	config, rules := createTempConfigs(t, `
DebugServiceAddr: "localhost:8085"
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
`, `
DryRun: true
`)
	defer os.Remove(rules)  // nolint:all // test case
	defer os.Remove(config) // nolint:all // test case

	c, err := NewConfig(config, rules, func(err error) {})
	assert.NoError(t, err)

	if d := c.GetIsDryRun(); d != true {
		t.Error("received", d, "expected", true) // nolint:all // test case
	}
}

func TestMaxAlloc(t *testing.T) {
	config, rules := createTempConfigs(t, `
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
  MaxAlloc: 17179869184
`, "")
	defer os.Remove(rules)
	defer os.Remove(config)

	c, err := NewConfig(config, rules, func(err error) {})
	assert.NoError(t, err)

	expected := uint64(16 * 1024 * 1024 * 1024)
	inMemConfig, err := c.GetInMemCollectorCacheCapacity()
	assert.NoError(t, err)
	assert.Equal(t, expected, inMemConfig.MaxAlloc)
}

func TestGetSamplerTypes(t *testing.T) {
	config, rules := createTempConfigs(t, `
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
`, `
Sampler: DeterministicSampler
SampleRate: 2
dataset 1:
  Sampler: DynamicSampler
  SampleRate: 2
  FieldList:
    - request.method
    - response.status_code
  UseTraceLength: true
  AddSampleRateKeyToTrace: true
  AddSampleRateKeyToTraceField: meta.tracing-proxy.dynsampler_key
  ClearFrequencySec: 60
dataset2:
  Sampler: DeterministicSampler
  SampleRate: 10
dataset3:
  Sampler: EMADynamicSampler
  GoalSampleRate: 10
  UseTraceLength: true
  AddSampleRateKeyToTrace: true
  AddSampleRateKeyToTraceField: meta.tracing-proxy.dynsampler_key
  FieldList: '[request.method]'
  Weight: 0.3
dataset4:
  Sampler: TotalThroughputSampler
  GoalThroughputPerSec: 100
  FieldList: '[request.method]'
`)
	defer os.Remove(rules)
	defer os.Remove(config)

	c, err := NewConfig(config, rules, func(err error) {})
	assert.NoError(t, err)

	if d, name, err := c.GetSamplerConfigForDataset("dataset-doesnt-exist"); assert.Equal(t, nil, err) {
		assert.IsType(t, &DeterministicSamplerConfig{}, d)
		assert.Equal(t, "DeterministicSampler", name)
	}

	if d, name, err := c.GetSamplerConfigForDataset("dataset 1"); assert.Equal(t, nil, err) {
		assert.IsType(t, &DynamicSamplerConfig{}, d)
		assert.Equal(t, "DynamicSampler", name)
	}

	if d, name, err := c.GetSamplerConfigForDataset("dataset2"); assert.Equal(t, nil, err) {
		assert.IsType(t, &DeterministicSamplerConfig{}, d)
		assert.Equal(t, "DeterministicSampler", name)
	}

	if d, name, err := c.GetSamplerConfigForDataset("dataset3"); assert.Equal(t, nil, err) {
		assert.IsType(t, &EMADynamicSamplerConfig{}, d)
		assert.Equal(t, "EMADynamicSampler", name)
	}

	if d, name, err := c.GetSamplerConfigForDataset("dataset4"); assert.Equal(t, nil, err) {
		assert.IsType(t, &TotalThroughputSamplerConfig{}, d)
		assert.Equal(t, "TotalThroughputSampler", name)
	}
}

func TestDefaultSampler(t *testing.T) {
	config, rules := createTempConfigs(t, `
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
`, "")
	defer os.Remove(rules)
	defer os.Remove(config)

	c, err := NewConfig(config, rules, func(err error) {})

	assert.NoError(t, err)

	s, name, err := c.GetSamplerConfigForDataset("nonexistent")

	assert.NoError(t, err)
	assert.Equal(t, "DeterministicSampler", name)

	assert.IsType(t, &DeterministicSamplerConfig{}, s)
}

func TestDatasetPrefix(t *testing.T) {
	config, rules := createTempConfigs(t, `
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
`, "")
	defer os.Remove(rules)
	defer os.Remove(config)

	c, err := NewConfig(config, rules, func(err error) {})
	assert.NoError(t, err)

	assert.Equal(t, "dataset", c.GetDatasetPrefix())
}

func TestSampleCacheParameters(t *testing.T) {
	config, rules := createTempConfigs(t, `
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
`, "")
	defer os.Remove(rules)
	defer os.Remove(config)

	c, err := NewConfig(config, rules, func(err error) {})
	assert.NoError(t, err)

	s := c.GetSampleCacheConfig()
	assert.Equal(t, "legacy", s.Type)
	assert.Equal(t, uint(10_000), s.KeptSize)
	assert.Equal(t, uint(1_000_000), s.DroppedSize)
	assert.Equal(t, 10*time.Second, s.SizeCheckInterval)
}

func TestSampleCacheParametersCuckoo(t *testing.T) {
	config, rules := createTempConfigs(t, `
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
SampleCache:
  Type: cuckoo
  KeptSize: 100000
  DroppedSize: 10000000
  SizeCheckInterval: 60s
`, "")
	defer os.Remove(rules)
	defer os.Remove(config)

	c, err := NewConfig(config, rules, func(err error) {})
	assert.NoError(t, err)

	s := c.GetSampleCacheConfig()
	assert.Equal(t, "cuckoo", s.Type)
	assert.Equal(t, uint(100_000), s.KeptSize)
	assert.Equal(t, uint(10_000_000), s.DroppedSize)
	assert.Equal(t, 1*time.Minute, s.SizeCheckInterval)
}
