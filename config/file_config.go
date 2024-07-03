package config

import (
	"crypto/md5" // #nosec
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opsramp/memory"
	"github.com/opsramp/tracing-proxy/pkg/libtrace/constants"
	"github.com/opsramp/tracing-proxy/pkg/retry"

	"github.com/fsnotify/fsnotify"
	"github.com/go-playground/validator"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	DefaultDataset = "ds"
)

type fileConfig struct {
	config        *viper.Viper
	rules         *viper.Viper
	conf          *configContents
	callbacks     []func()
	errorCallback func(error)
	mux           sync.RWMutex
	lastLoadTime  time.Time
}

type configContents struct {
	ListenAddr                string `validate:"required"`
	PeerListenAddr            string `validate:"required"`
	CompressPeerCommunication bool
	GRPCListenAddr            string
	GRPCPeerListenAddr        string
	OpsrampAPI                string `validate:"required,url"`
	Dataset                   string
	LoggingLevel              string        `validate:"required"`
	Collector                 string        `validate:"required,oneof= InMemCollector"`
	Sampler                   string        `validate:"required,oneof= DeterministicSampler DynamicSampler EMADynamicSampler RulesBasedSampler TotalThroughputSampler"`
	SendDelay                 time.Duration `validate:"required"`
	BatchTimeout              time.Duration
	TraceTimeout              time.Duration `validate:"required"`
	MaxBatchSize              uint          `validate:"required"`
	SendTicker                time.Duration `validate:"required"`
	UpstreamBufferSize        int           `validate:"required"`
	PeerBufferSize            int           `validate:"required"`
	DebugServiceAddr          string
	DryRun                    bool
	DryRunFieldName           string
	PeerManagement            PeerManagementConfig           `validate:"required"`
	InMemCollector            InMemoryCollectorCacheCapacity `validate:"required"`
	AddHostMetadataToTrace    bool
	AddAdditionalMetadata     map[string]string
	Threshold                 float64
	LogsEndpoint              string
	SendEvents                bool
	AddRuleReasonToTrace      bool
	EnvironmentCacheTTL       time.Duration
	DatasetPrefix             string
	QueryAuthToken            string
	GRPCServerParameters      GRPCServerParameters
	AdditionalErrorFields     []string
	AddSpanCountToRoot        bool
	CacheOverrunStrategy      string
	SampleCache               SampleCacheConfig `validate:"required"`

	UseTls         bool
	UseTlsInSecure bool

	ProxyConfiguration
	AuthConfiguration
	MetricsConfig
	RetryConfiguration *retry.Config
}

type ProxyConfiguration struct {
	Protocol string
	Host     string
	Port     string
	Username string
	Password string
}

type AuthConfiguration struct {
	SkipAuth bool
	Endpoint string `validate:"url"`
	Key      string
	Secret   string
	TenantId string
}

type InMemoryCollectorCacheCapacity struct {
	// CacheCapacity must be less than math.MaxInt32
	CacheCapacity int `validate:"required,lt=2147483647"`
	MaxAlloc      uint64
}

type LogrusLoggerConfig struct {
	LogFormatter string `validate:"required" toml:"LogFormatter"`
	LogOutput    string `validate:"required,oneof= stdout stderr file" toml:"LogOutput"`
	File         struct {
		FileName   string `toml:"FileName"`
		MaxSize    int    `toml:"MaxSize"`
		MaxBackups int    `toml:"MaxBackups"`
		Compress   bool   `toml:"Compress"`
	} `toml:"File"`
}

type MetricsConfig struct {
	Enable            bool
	ListenAddr        string `validate:"required"`
	OpsRampAPI        string
	ReportingInterval int64
	MetricsList       []string
}

type PeerManagementConfig struct {
	Type                    string   `validate:"required,oneof= file redis"`
	Peers                   []string `validate:"dive,url"`
	RedisHost               string
	RedisUsername           string
	RedisPassword           string
	UseTLS                  bool
	UseTLSInsecure          bool
	IdentifierInterfaceName string
	UseIPV6Identifier       bool
	RedisIdentifier         string
	Timeout                 time.Duration
	Strategy                string `validate:"required,oneof= legacy hash"`
}

type SampleCacheConfig struct {
	Type              string        `validate:"required,oneof= legacy cuckoo"`
	KeptSize          uint          `validate:"gte=500"`
	DroppedSize       uint          `validate:"gte=100_000"`
	SizeCheckInterval time.Duration `validate:"gte=1_000_000_000"` // 1 second minimum
}

// GRPCServerParameters allow you to configure the GRPC ServerParameters used
// by refinery's own GRPC server:
// https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters
type GRPCServerParameters struct {
	MaxConnectionIdle     time.Duration
	MaxConnectionAge      time.Duration
	MaxConnectionAgeGrace time.Duration
	Time                  time.Duration
	Timeout               time.Duration
}

// NewConfig creates a new config struct
func NewConfig(config, rules string, errorCallback func(error)) (Config, error) {
	c := viper.New()

	c.SetDefault("ListenAddr", "0.0.0.0:8082")
	c.SetDefault("PeerListenAddr", "0.0.0.0:8083")
	c.SetDefault("CompressPeerCommunication", true) // nolint:all // setting default value
	c.SetDefault("PeerManagement.Peers", []string{"http://127.0.0.1:8082"})
	c.SetDefault("PeerManagement.Type", "file")
	c.SetDefault("PeerManagement.UseTLS", false)            // nolint:all // setting default value
	c.SetDefault("PeerManagement.UseTLSInsecure", false)    // nolint:all // setting default value
	c.SetDefault("PeerManagement.UseIPV6Identifier", false) // nolint:all // setting default value
	c.SetDefault("OpsrampAPI", "")
	c.SetDefault("Dataset", DefaultDataset)
	c.SetDefault("PeerManagement.Timeout", 5*time.Second)
	c.SetDefault("PeerManagement.Strategy", "legacy")
	c.SetDefault("LoggingLevel", "info")
	c.SetDefault("Collector", "InMemCollector")
	c.SetDefault("SendDelay", 2*time.Second)
	c.SetDefault("BatchTimeout", constants.DefaultBatchTimeout)
	c.SetDefault("TraceTimeout", 60*time.Second)
	c.SetDefault("MaxBatchSize", 500)
	c.SetDefault("SendTicker", 100*time.Millisecond)
	c.SetDefault("UpstreamBufferSize", constants.DefaultPendingWorkCapacity)
	c.SetDefault("PeerBufferSize", constants.DefaultPendingWorkCapacity)
	c.SetDefault("MaxAlloc", uint64(0))
	c.SetDefault("AddHostMetadataToTrace", false) // nolint:all // setting default value
	c.SetDefault("AddAdditionalMetadata", map[string]string{"app": "default"})
	c.SetDefault("AddRuleReasonToTrace", false) // nolint:all // setting default value
	c.SetDefault("EnvironmentCacheTTL", time.Hour)
	c.SetDefault("GRPCServerParameters.MaxConnectionIdle", 1*time.Minute)
	c.SetDefault("GRPCServerParameters.MaxConnectionAge", time.Duration(0))
	c.SetDefault("GRPCServerParameters.MaxConnectionAgeGrace", time.Duration(0))
	c.SetDefault("GRPCServerParameters.Time", 10*time.Second)
	c.SetDefault("GRPCServerParameters.Timeout", 2*time.Second)
	c.SetDefault("AdditionalErrorFields", []string{"trace.span_id"})
	c.SetDefault("AddSpanCountToRoot", false) // nolint:all // setting default value
	c.SetDefault("CacheOverrunStrategy", "resize")
	c.SetDefault("SampleCache.Type", "legacy")
	c.SetDefault("SampleCache.KeptSize", 10_000)
	c.SetDefault("SampleCache.DroppedSize", 1_000_000)
	c.SetDefault("SampleCache.SizeCheckInterval", 10*time.Second)

	// AuthConfig Defaults
	c.SetDefault("AuthConfiguration.SkipAuth", false) // nolint:all // setting default value

	// MetricsConfig Defaults
	c.SetDefault("MetricsConfig.Enable", false) // nolint:all // setting default value
	c.SetDefault("MetricsConfig.ListenAddr", "0.0.0.0:2112")
	c.SetDefault("MetricsConfig.ReportingInterval", 10)
	c.SetDefault("MetricsConfig.MetricsList", []string{".*"})

	// InMemoryCollector Defaults
	// cpuRequests := os.Getenv("CONTAINER_CPU_REQUEST")
	// cpuLimit := os.Getenv("CONTAINER_CPU_LIMIT")
	memReq := os.Getenv("CONTAINER_MEM_REQUEST")
	memLimit := os.Getenv("CONTAINER_MEM_LIMIT")

	memoryRequests, err := strconv.Atoi(memReq)
	if err != nil || memoryRequests == 0 {
		memoryRequests, _ = strconv.Atoi(memLimit)
	}
	if memoryRequests != 0 {
		memoryRequests = int(float64(memoryRequests) * 0.8)
		c.SetDefault("InMemCollector.MaxAlloc", memoryRequests)
	} else {
		// If it is a normal bare metal machine
		if totalMemory := memory.TotalMemory(); totalMemory != 0 {
			memoryRequests = int(float64(totalMemory) * 0.8)
			c.SetDefault("InMemCollector.MaxAlloc", memoryRequests)
		}
	}

	c.SetConfigFile(config)
	err = c.ReadInConfig()
	if err != nil {
		return nil, err
	}

	if c.GetInt("InMemCollector.MaxAlloc") <= 0 {
		c.Set("InMemCollector.MaxAlloc", memoryRequests)
	}

	r := viper.New()

	r.SetDefault("Sampler", "DeterministicSampler")
	r.SetDefault("SampleRate", 1)
	r.SetDefault("DryRun", false) // nolint:all // setting default value
	r.SetDefault("DryRunFieldName", "tracing-proxy_kept")

	r.SetConfigFile(rules)
	err = r.ReadInConfig()
	if err != nil {
		return nil, err
	}

	fc := &fileConfig{
		config:        c,
		rules:         r,
		conf:          &configContents{},
		callbacks:     []func(){},
		errorCallback: errorCallback,
	}

	err = fc.unmarshal()
	if err != nil {
		return nil, err
	}

	v := validator.New()
	err = v.Struct(fc.conf)
	if err != nil {
		return nil, err
	}

	err = fc.validateGeneralConfigs()
	if err != nil {
		return nil, err
	}

	err = fc.validateSamplerConfigs()
	if err != nil {
		return nil, err
	}

	c.WatchConfig()
	c.OnConfigChange(fc.onChange)

	r.WatchConfig()
	r.OnConfigChange(fc.onChange)

	return fc, nil
}

func (f *fileConfig) IsTest() bool {
	return false
}

func (f *fileConfig) onChange(in fsnotify.Event) {
	v := validator.New()
	err := v.Struct(f.conf)
	if err != nil {
		f.errorCallback(err)
		return
	}

	err = f.validateGeneralConfigs()
	if err != nil {
		f.errorCallback(err)
		return
	}

	err = f.validateSamplerConfigs()
	if err != nil {
		f.errorCallback(err)
		return
	}

	_ = f.unmarshal()

	for _, c := range f.callbacks {
		c()
	}
}

func (f *fileConfig) unmarshal() error {
	f.mux.Lock()
	defer f.mux.Unlock()
	err := f.config.Unmarshal(f.conf)
	if err != nil {
		return err
	}

	err = f.rules.Unmarshal(f.conf)
	if err != nil {
		return err
	}

	return nil
}

func (f *fileConfig) validateGeneralConfigs() error {
	f.lastLoadTime = time.Now()

	// validate metrics config
	metricsConfig := f.GetMetricsConfig()
	if metricsConfig.ReportingInterval < 10 {
		return fmt.Errorf("mertics reporting interval %d not allowed, must be >= 10", metricsConfig.ReportingInterval)
	}
	if len(metricsConfig.MetricsList) < 1 {
		return fmt.Errorf("mertics list cant be empty")
	}

	return nil
}

func (f *fileConfig) validateSamplerConfigs() error {
	logrus.Debugf("Sampler rules config: %+v", f.rules)

	keys := f.rules.AllKeys()
	for _, key := range keys {
		parts := strings.Split(key, ".")

		// verify default sampler config
		if parts[0] == "sampler" {
			t := f.rules.GetString(key)
			var i interface{}
			switch t {
			case "DeterministicSampler":
				i = &DeterministicSamplerConfig{}
			case "DynamicSampler":
				i = &DynamicSamplerConfig{}
			case "EMADynamicSampler":
				i = &EMADynamicSamplerConfig{}
			case "RulesBasedSampler":
				i = &RulesBasedSamplerConfig{}
			case "TotalThroughputSampler":
				i = &TotalThroughputSamplerConfig{}
			default:
				return fmt.Errorf("Invalid or missing default sampler type: %s", t)
			}
			err := f.rules.Unmarshal(i)
			if err != nil {
				return fmt.Errorf("Failed to unmarshal sampler rule: %w", err)
			}
			v := validator.New()
			err = v.Struct(i)
			if err != nil {
				return fmt.Errorf("Failed to validate sampler rule: %w", err)
			}
		}

		// verify dataset sampler configs
		if len(parts) > 1 && parts[1] == "sampler" {
			t := f.rules.GetString(key)
			var i interface{}
			switch t {
			case "DeterministicSampler":
				i = &DeterministicSamplerConfig{}
			case "DynamicSampler":
				i = &DynamicSamplerConfig{}
			case "EMADynamicSampler":
				i = &EMADynamicSamplerConfig{}
			case "RulesBasedSampler":
				i = &RulesBasedSamplerConfig{}
			case "TotalThroughputSampler":
				i = &TotalThroughputSamplerConfig{}
			default:
				return fmt.Errorf("Invalid or missing dataset sampler type: %s", t)
			}
			datasetName := parts[0]
			if sub := f.rules.Sub(datasetName); sub != nil {
				err := sub.Unmarshal(i)
				if err != nil {
					return fmt.Errorf("Failed to unmarshal dataset sampler rule: %w", err)
				}
				v := validator.New()
				err = v.Struct(i)
				if err != nil {
					return fmt.Errorf("Failed to validate dataset sampler rule: %w", err)
				}
			}
		}
	}
	return nil
}

func (f *fileConfig) RegisterReloadCallback(cb func()) {
	f.mux.Lock()
	defer f.mux.Unlock()

	f.callbacks = append(f.callbacks, cb)
}

func (f *fileConfig) GetListenAddr() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	_, _, err := net.SplitHostPort(f.conf.ListenAddr)
	if err != nil {
		return "", err
	}
	return f.conf.ListenAddr, nil
}

func (f *fileConfig) GetPeerListenAddr() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	_, _, err := net.SplitHostPort(f.conf.PeerListenAddr)
	if err != nil {
		return "", err
	}
	return f.conf.PeerListenAddr, nil
}

func (f *fileConfig) GetCompressPeerCommunication() bool {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.CompressPeerCommunication
}

func (f *fileConfig) GetThreshold() float64 {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.Threshold
}

func (f *fileConfig) GetLogsEndpoint() string {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.LogsEndpoint
}

func (f *fileConfig) GetSendEvents() bool {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.SendEvents
}

func (f *fileConfig) GetGRPCListenAddr() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	// GRPC listen addr is optional, only check value is valid if not empty
	if f.conf.GRPCListenAddr != "" {
		_, _, err := net.SplitHostPort(f.conf.GRPCListenAddr)
		if err != nil {
			return "", err
		}
	}
	return f.conf.GRPCListenAddr, nil
}

func (f *fileConfig) GetGRPCPeerListenAddr() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	// GRPC listen addr is optional, only check value is valid if not empty
	if f.conf.GRPCPeerListenAddr != "" {
		_, _, err := net.SplitHostPort(f.conf.GRPCPeerListenAddr)
		if err != nil {
			return "", err
		}
	}
	return f.conf.GRPCPeerListenAddr, nil
}

func (f *fileConfig) GetPeerManagementType() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.PeerManagement.Type, nil
}

func (f *fileConfig) GetPeerManagementStrategy() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.PeerManagement.Strategy, nil
}

func (f *fileConfig) GetPeers() ([]string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.PeerManagement.Peers, nil
}

func (f *fileConfig) GetRedisHost() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.config.GetString("PeerManagement.RedisHost"), nil
}

func (f *fileConfig) GetRedisUsername() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.config.GetString("PeerManagement.RedisUsername"), nil
}

func (f *fileConfig) GetRedisPassword() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.config.GetString("PeerManagement.RedisPassword"), nil
}

func (f *fileConfig) GetRedisPrefix() string {
	f.mux.RLock()
	defer f.mux.RUnlock()

	prefix := f.config.GetString("PeerManagement.RedisPrefix")
	if prefix == "" {
		prefix = "tracing-proxy"
	}

	return prefix
}

func (f *fileConfig) GetRedisDatabase() int {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.config.GetInt("PeerManagement.RedisDatabase")
}

func (f *fileConfig) GetProxyConfig() ProxyConfiguration {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.ProxyConfiguration
}

func (f *fileConfig) GetUseTLS() (bool, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.config.GetBool("PeerManagement.UseTLS"), nil
}

func (f *fileConfig) GetUseTLSInsecure() (bool, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.config.GetBool("PeerManagement.UseTLSInsecure"), nil
}

func (f *fileConfig) GetIdentifierInterfaceName() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.config.GetString("PeerManagement.IdentifierInterfaceName"), nil
}

func (f *fileConfig) GetUseIPV6Identifier() (bool, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.config.GetBool("PeerManagement.UseIPV6Identifier"), nil
}

func (f *fileConfig) GetRedisIdentifier() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.config.GetString("PeerManagement.RedisIdentifier"), nil
}

func (f *fileConfig) GetOpsrampAPI() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	_, err := url.Parse(f.conf.OpsrampAPI)
	if err != nil {
		return "", err
	}

	return f.conf.OpsrampAPI, nil
}

func (f *fileConfig) GetAuthConfig() AuthConfiguration {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.AuthConfiguration
}

func (f *fileConfig) GetRetryConfig() *retry.Config {
	f.mux.RLock()
	defer f.mux.RUnlock()

	if f.conf.RetryConfiguration == nil {
		return retry.NewDefaultRetrySettings()
	}

	return f.conf.RetryConfiguration
}

func (f *fileConfig) GetDataset() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.Dataset, nil
}

func (f *fileConfig) GetTenantId() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.AuthConfiguration.TenantId, nil
}

func (f *fileConfig) GetLoggingLevel() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.LoggingLevel, nil
}

func (f *fileConfig) GetCollectorType() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.Collector, nil
}

func (f *fileConfig) GetAllSamplerRules() (map[string]interface{}, error) {
	samplers := make(map[string]interface{})

	keys := f.rules.AllKeys()
	for _, key := range keys {
		parts := strings.Split(key, ".")

		// extract default sampler rules
		if parts[0] == "sampler" {
			err := f.rules.Unmarshal(&samplers)
			if err != nil {
				return nil, fmt.Errorf("unmarshal sampler rule: %w", err)
			}
			t := f.rules.GetString(key)
			samplers["sampler"] = t
			continue
		}

		// extract all dataset sampler rules
		if len(parts) > 1 && parts[1] == "sampler" {
			t := f.rules.GetString(key)
			m := make(map[string]interface{})
			datasetName := parts[0]
			if sub := f.rules.Sub(datasetName); sub != nil {
				err := sub.Unmarshal(&m)
				if err != nil {
					return nil, fmt.Errorf("unmarshal sampler rule for dataset %s: %w", datasetName, err)
				}
			}
			m["sampler"] = t
			samplers[datasetName] = m
		}
	}
	return samplers, nil
}

func (f *fileConfig) GetSamplerConfigForDataset(dataset string) (interface{}, string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	const notfound = "not found"

	key := fmt.Sprintf("%s.Sampler", dataset)
	if ok := f.rules.IsSet(key); ok {
		t := f.rules.GetString(key)
		var i interface{}

		switch t {
		case "DeterministicSampler":
			i = &DeterministicSamplerConfig{}
		case "DynamicSampler":
			i = &DynamicSamplerConfig{}
		case "EMADynamicSampler":
			i = &EMADynamicSamplerConfig{}
		case "RulesBasedSampler":
			i = &RulesBasedSamplerConfig{}
		case "TotalThroughputSampler":
			i = &TotalThroughputSamplerConfig{}
		default:
			return nil, notfound, errors.New("No Sampler found")
		}

		if sub := f.rules.Sub(dataset); sub != nil {
			return i, t, sub.Unmarshal(i)
		}
	} else if ok := f.rules.IsSet("Sampler"); ok {
		t := f.rules.GetString("Sampler")
		var i interface{}

		switch t {
		case "DeterministicSampler":
			i = &DeterministicSamplerConfig{}
		case "DynamicSampler":
			i = &DynamicSamplerConfig{}
		case "EMADynamicSampler":
			i = &EMADynamicSamplerConfig{}
		case "RulesBasedSampler":
			i = &RulesBasedSamplerConfig{}
		case "TotalThroughputSampler":
			i = &TotalThroughputSamplerConfig{}
		default:
			return nil, notfound, errors.New("No Sampler found")
		}

		return i, t, f.rules.Unmarshal(i)
	}

	return nil, notfound, errors.New("No Sampler found")
}

func (f *fileConfig) GetInMemCollectorCacheCapacity() (InMemoryCollectorCacheCapacity, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return InMemoryCollectorCacheCapacity{
		CacheCapacity: f.conf.InMemCollector.CacheCapacity,
		MaxAlloc:      f.conf.InMemCollector.MaxAlloc,
	}, nil
}

func (f *fileConfig) GetLogrusConfig() (*LogrusLoggerConfig, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	logrusConfig := &LogrusLoggerConfig{}

	if sub := f.config.Sub("LogrusLogger"); sub != nil {
		err := sub.UnmarshalExact(logrusConfig)
		if err != nil {
			return logrusConfig, err
		}

		v := validator.New()
		err = v.Struct(logrusConfig)
		if err != nil {
			return logrusConfig, err
		}

		return logrusConfig, nil
	}
	return nil, errors.New("No config found for LogrusConfig")
}

func (f *fileConfig) GetMetricsConfig() MetricsConfig {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.MetricsConfig
}

func (f *fileConfig) GetSendDelay() (time.Duration, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.SendDelay, nil
}

func (f *fileConfig) GetBatchTimeout() time.Duration {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.BatchTimeout
}

func (f *fileConfig) GetTraceTimeout() (time.Duration, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.TraceTimeout, nil
}

func (f *fileConfig) GetMaxBatchSize() uint {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.MaxBatchSize
}

func (f *fileConfig) GetOtherConfig(name string, iface interface{}) error {
	f.mux.RLock()
	defer f.mux.RUnlock()

	if sub := f.config.Sub(name); sub != nil {
		return sub.Unmarshal(iface)
	}

	if sub := f.rules.Sub(name); sub != nil {
		return sub.Unmarshal(iface)
	}

	return fmt.Errorf("failed to find config tree for %s", name)
}

func (f *fileConfig) GetUpstreamBufferSize() int {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.UpstreamBufferSize
}

func (f *fileConfig) GetPeerBufferSize() int {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.PeerBufferSize
}

func (f *fileConfig) GetSendTickerValue() time.Duration {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.SendTicker
}

func (f *fileConfig) GetDebugServiceAddr() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	_, _, err := net.SplitHostPort(f.conf.DebugServiceAddr)
	if err != nil {
		return "", err
	}
	return f.conf.DebugServiceAddr, nil
}

func (f *fileConfig) GetIsDryRun() bool {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.DryRun
}

func (f *fileConfig) GetDryRunFieldName() string {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.DryRunFieldName
}

func (f *fileConfig) GetAddHostMetadataToTrace() bool {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.AddHostMetadataToTrace
}

func (f *fileConfig) GetAddAdditionalMetadata() map[string]string {
	f.mux.RLock()
	defer f.mux.RUnlock()

	if len(f.conf.AddAdditionalMetadata) <= 5 {
		return f.conf.AddAdditionalMetadata
	}

	// sorting the keys and sending the first 5
	var keys []string
	for k := range f.conf.AddAdditionalMetadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	m := map[string]string{}
	for index := 0; index < 5; index++ {
		if val, ok := f.conf.AddAdditionalMetadata[keys[index]]; ok {
			m[keys[index]] = val
		}
	}

	return m
}

func (f *fileConfig) GetSendMetricsToOpsRamp() bool {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.MetricsConfig.Enable
}

func (f *fileConfig) GetGlobalUseTLS() bool {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.UseTls
}

func (f *fileConfig) GetGlobalUseTLSInsecureSkip() bool {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return !f.conf.UseTlsInSecure
}

func (f *fileConfig) GetAddRuleReasonToTrace() bool {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.AddRuleReasonToTrace
}

func (f *fileConfig) GetEnvironmentCacheTTL() time.Duration {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.EnvironmentCacheTTL
}

func (f *fileConfig) GetDatasetPrefix() string {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.DatasetPrefix
}

func (f *fileConfig) GetQueryAuthToken() string {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.QueryAuthToken
}

func (f *fileConfig) GetGRPCMaxConnectionIdle() time.Duration {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.GRPCServerParameters.MaxConnectionIdle
}

func (f *fileConfig) GetGRPCMaxConnectionAge() time.Duration {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.GRPCServerParameters.MaxConnectionAge
}

func (f *fileConfig) GetGRPCMaxConnectionAgeGrace() time.Duration {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.GRPCServerParameters.MaxConnectionAgeGrace
}

func (f *fileConfig) GetGRPCTime() time.Duration {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.GRPCServerParameters.Time
}

func (f *fileConfig) GetGRPCTimeout() time.Duration {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.GRPCServerParameters.Timeout
}

func (f *fileConfig) GetPeerTimeout() time.Duration {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.PeerManagement.Timeout
}

func (f *fileConfig) GetAdditionalErrorFields() []string {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.AdditionalErrorFields
}

func (f *fileConfig) GetAddSpanCountToRoot() bool {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.AddSpanCountToRoot
}

func (f *fileConfig) GetCacheOverrunStrategy() string {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.CacheOverrunStrategy
}

func (f *fileConfig) GetSampleCacheConfig() SampleCacheConfig {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.SampleCache
}

// calculates an MD5 sum for a file that returns the same result as the md5sum command
func calcMD5For(filename string) string {
	f, err := os.Open(filename)
	if err != nil {
		return err.Error()
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return err.Error()
	}
	h := md5.New() // #nosec
	if _, err := h.Write(data); err != nil {
		return err.Error()
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (f *fileConfig) GetConfigMetadata() []ConfigMetadata {
	ret := make([]ConfigMetadata, 2)
	ret[0] = ConfigMetadata{
		Type:     "config",
		ID:       f.config.ConfigFileUsed(),
		Hash:     calcMD5For(f.config.ConfigFileUsed()),
		LoadedAt: f.lastLoadTime.Format(time.RFC3339),
	}
	ret[1] = ConfigMetadata{
		Type:     "rules",
		ID:       f.rules.ConfigFileUsed(),
		Hash:     calcMD5For(f.rules.ConfigFileUsed()),
		LoadedAt: f.lastLoadTime.Format(time.RFC3339),
	}
	return ret
}
