package config

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-playground/validator"
	libtrace "github.com/honeycombio/libhoney-go"
	"github.com/sirupsen/logrus"
	viper "github.com/spf13/viper"
)

type fileConfig struct {
	config        *viper.Viper
	rules         *viper.Viper
	conf          *configContents
	callbacks     []func()
	errorCallback func(error)
	mux           sync.RWMutex
}

type configContents struct {
	ListenAddr                string `validate:"required"`
	PeerListenAddr            string `validate:"required"`
	CompressPeerCommunication bool
	GRPCListenAddr            string
	GRPCPeerListenAddr        string
	APIKeys                   []string      `validate:"required"`
	HoneycombAPI              string        `validate:"required,url"`
	LoggingLevel              string        `validate:"required"`
	Collector                 string        `validate:"required,oneof= InMemCollector"`
	Sampler                   string        `validate:"required,oneof= DeterministicSampler DynamicSampler EMADynamicSampler RulesBasedSampler TotalThroughputSampler"`
	SendDelay                 time.Duration `validate:"required"`
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
	SendMetricsToOpsRamp      bool
	UseTls                    bool
	UseTlsInSecure            bool
}

type InMemoryCollectorCacheCapacity struct {
	// CacheCapacity must be less than math.MaxInt32
	CacheCapacity int `validate:"required,lt=2147483647"`
	MaxAlloc      uint64
}

type LogrusLoggerConfig struct {
	LogFormatter string `validate:"required",toml:"LogFormatter"`
	LogOutput    string `validate:"required,oneof= stdout stderr file",toml:"LogOutput"`
	File         struct {
		FileName   string `toml:"FileName"`
		MaxSize    int    `toml:"MaxSize"`
		MaxBackups int    `toml:"MaxBackups"`
		Compress   bool   `toml:"Compress"`
	} `toml:"File"`
}

type OpsRampMetricsConfig struct {
	MetricsListenAddr               string `validate:"required"`
	OpsRampMetricsAPI               string `validate:"required,url"`
	OpsRampTenantID                 string `validate:"required"`
	OpsRampMetricsAPIKey            string `validate:"required"`
	OpsRampMetricsAPISecret         string `validate:"required"`
	OpsRampMetricsReportingInterval int64  `validate:"required"`
	OpsRampMetricsRetryCount        int64  `validate:"required"`
	ProxyProtocol                   string
	ProxyServer                     string
	ProxyPort                       int64
	ProxyUserName                   string
	ProxyPassword                   string
	OpsRampMetricsList              []string
}

type PeerManagementConfig struct {
	Type                    string   `validate:"required,oneof= file redis"`
	Peers                   []string `validate:"dive,url"`
	RedisHost               string
	RedisPassword           string
	UseTLS                  bool
	UseTLSInsecure          bool
	IdentifierInterfaceName string
	UseIPV6Identifier       bool
	RedisIdentifier         string
}

// NewConfig creates a new config struct
func NewConfig(config, rules string, errorCallback func(error)) (Config, error) {
	c := viper.New()

	c.BindEnv("PeerManagement.RedisHost", "tracing-proxy_REDIS_HOST")
	c.BindEnv("PeerManagement.RedisPassword", "tracing-proxy_REDIS_PASSWORD")
	c.SetDefault("ListenAddr", "0.0.0.0:8080")
	c.SetDefault("PeerListenAddr", "0.0.0.0:8081")
	c.SetDefault("CompressPeerCommunication", true)
	c.SetDefault("APIKeys", []string{"*"})
	c.SetDefault("PeerManagement.Peers", []string{"http://127.0.0.1:8081"})
	c.SetDefault("PeerManagement.Type", "file")
	c.SetDefault("PeerManagement.UseTLS", false)
	c.SetDefault("PeerManagement.UseTLSInsecure", false)
	c.SetDefault("PeerManagement.UseIPV6Identifier", false)
	c.SetDefault("HoneycombAPI", "https://api.jirs5")
	c.SetDefault("LoggingLevel", "debug")
	c.SetDefault("Collector", "InMemCollector")
	c.SetDefault("SendDelay", 2*time.Second)
	c.SetDefault("TraceTimeout", 60*time.Second)
	c.SetDefault("MaxBatchSize", 500)
	c.SetDefault("SendTicker", 100*time.Millisecond)
	c.SetDefault("UpstreamBufferSize", libtrace.DefaultPendingWorkCapacity)
	c.SetDefault("PeerBufferSize", libtrace.DefaultPendingWorkCapacity)
	c.SetDefault("MaxAlloc", uint64(0))
	c.SetDefault("AddHostMetadataToTrace", false)
	c.SetDefault("SendMetricsToOpsRamp", false)

	c.SetConfigFile(config)
	err := c.ReadInConfig()

	if err != nil {
		return nil, err
	}

	r := viper.New()

	r.SetDefault("Sampler", "DeterministicSampler")
	r.SetDefault("SampleRate", 1)
	r.SetDefault("DryRun", false)
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
		callbacks:     make([]func(), 0),
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

	err = fc.validateConditionalConfigs()
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

func (f *fileConfig) onChange(in fsnotify.Event) {
	v := validator.New()
	err := v.Struct(f.conf)
	if err != nil {
		f.errorCallback(err)
		return
	}

	err = f.validateConditionalConfigs()
	if err != nil {
		f.errorCallback(err)
		return
	}

	err = f.validateSamplerConfigs()
	if err != nil {
		f.errorCallback(err)
		return
	}

	f.unmarshal()

	f.mux.RLock()
	defer f.mux.RUnlock()

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

func (f *fileConfig) validateConditionalConfigs() error {
	// validate metrics config
	_, err := f.GetOpsRampMetricsConfig()
	if err != nil {
		return err
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

func (f *fileConfig) GetAPIKeys() ([]string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.APIKeys, nil
}

func (f *fileConfig) GetPeerManagementType() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.PeerManagement.Type, nil
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

func (f *fileConfig) GetRedisPassword() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.config.GetString("PeerManagement.RedisPassword"), nil
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

func (f *fileConfig) GetHoneycombAPI() (string, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.HoneycombAPI, nil
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

func (f *fileConfig) GetSamplerConfigForDataset(dataset string) (interface{}, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

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
			return nil, errors.New("No Sampler found")
		}

		if sub := f.rules.Sub(dataset); sub != nil {
			return i, sub.Unmarshal(i)
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
			return nil, errors.New("No Sampler found")
		}

		return i, f.rules.Unmarshal(i)
	}

	return nil, errors.New("No Sampler found")
}

func (f *fileConfig) GetInMemCollectorCacheCapacity() (InMemoryCollectorCacheCapacity, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	capacity := &InMemoryCollectorCacheCapacity{}
	if sub := f.config.Sub("InMemCollector"); sub != nil {
		err := sub.UnmarshalExact(capacity)
		if err != nil {
			return *capacity, err
		}
		return *capacity, nil
	}
	return *capacity, errors.New("No config found for inMemCollector")
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

func (f *fileConfig) GetOpsRampMetricsConfig() (*OpsRampMetricsConfig, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	opsRampMetricsConfig := &OpsRampMetricsConfig{}

	if sub := f.config.Sub("OpsRampMetrics"); sub != nil {
		err := sub.UnmarshalExact(opsRampMetricsConfig)
		if err != nil {
			return opsRampMetricsConfig, err
		}

		if opsRampMetricsConfig.OpsRampMetricsRetryCount < 0 || opsRampMetricsConfig.OpsRampMetricsRetryCount > 10 {
			opsRampMetricsConfig.OpsRampMetricsRetryCount = 2
		}

		if opsRampMetricsConfig.OpsRampMetricsReportingInterval < 10 {
			opsRampMetricsConfig.OpsRampMetricsReportingInterval = 10
		}

		if len(opsRampMetricsConfig.OpsRampMetricsList) < 1 {
			opsRampMetricsConfig.OpsRampMetricsList = []string{".*"}
		}

		v := validator.New()
		err = v.Struct(opsRampMetricsConfig)
		if err != nil {
			return opsRampMetricsConfig, err
		}

		return opsRampMetricsConfig, nil
	}
	return nil, errors.New("No config found for OpsRampMetrics")
}

func (f *fileConfig) GetSendDelay() (time.Duration, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.SendDelay, nil
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

func (f *fileConfig) GetSendMetricsToOpsRamp() bool {
	f.mux.RLock()
	defer f.mux.RUnlock()

	return f.conf.SendMetricsToOpsRamp
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
