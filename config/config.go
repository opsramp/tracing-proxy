package config

import "time"

// Config defines the interface the rest of the code uses to get items from the
// config. There are different implementations of the config using different
// backends to store the config. FileConfig is the default and uses a
// TOML-formatted config file. RedisPeerFileConfig uses a redis cluster to store
// the list of peers and then falls back to a filesystem config file for all
// other config elements.

type Config interface {
	// RegisterReloadCallback takes a name and a function that will be called
	// when the configuration is reloaded. This will happen infrequently. If
	// consumers of configuration set config values on startup, they should
	// check their values haven't changed and re-start anything that needs
	// restarting with the new values.
	RegisterReloadCallback(callback func())

	// GetListenAddr returns the address and port on which to listen for
	// incoming events
	GetListenAddr() (string, error)

	// GetPeerListenAddr returns the address and port on which to listen for
	// peer traffic
	GetPeerListenAddr() (string, error)

	// GetPeerListenAddr returns the address and port on which to listen for
	// GRPC peer traffic
	GetGRPCPeerListenAddr() (string, error)

	// GetCompressPeerCommunication will be true if tracing-proxy should compress
	// data before forwarding it to a peer.
	GetCompressPeerCommunication() bool

	// GetGRPCListenAddr returns the address and port on which to listen for
	// incoming events over gRPC
	GetGRPCListenAddr() (string, error)

	// GetAPIKeys returns a list of Honeycomb API keys
	GetAPIKeys() ([]string, error)

	// GetPeers returns a list of other servers participating in this proxy cluster
	GetPeers() ([]string, error)

	GetPeerManagementType() (string, error)

	// GetRedisHost returns the address of a Redis instance to use for peer
	// management.
	GetRedisHost() (string, error)

	// GetRedisPassword returns the password of a Redis instance to use for peer
	// management.
	GetRedisPassword() (string, error)

	// GetUseTLS returns true when TLS must be enabled to dial the Redis instance to
	// use for peer management.
	GetUseTLS() (bool, error)

	// UseTLSInsecure returns true when certificate checks are disabled
	GetUseTLSInsecure() (bool, error)

	// GetOpsrampAPI returns the base URL (protocol, hostname, and port) of
	// the upstream Honeycomb API server
	GetOpsrampAPI() (string, error)

	// GetLoggingLevel returns the verbosity with which we should log
	GetLoggingLevel() (string, error)

	// GetSendDelay returns the number of seconds to pause after a trace is
	// complete before sending it, to allow stragglers to arrive
	GetSendDelay() (time.Duration, error)

	// GetTraceTimeout is how long to wait before sending a trace even if it's
	// not complete. This should be longer than the longest expected trace
	// duration.
	GetTraceTimeout() (time.Duration, error)

	// GetMaxBatchSize is the number of events to be included in the batch for sending
	GetMaxBatchSize() uint

	// GetOtherConfig attempts to fill the passed in struct with the contents of
	// a subsection of the config.   This is used by optional configurations to
	// allow different implementations of necessary interfaces configure
	// themselves
	GetOtherConfig(name string, configStruct interface{}) error

	// GetCollectorType returns the type of the collector to use. Valid types
	// are in the collect package
	GetCollectorType() (string, error)

	// GetInMemCollectorCacheCapacity returns the config specific to the InMemCollector
	GetInMemCollectorCacheCapacity() (InMemoryCollectorCacheCapacity, error)

	// GetSamplerConfigForDataset returns the sampler type to use for the given dataset
	GetSamplerConfigForDataset(string) (interface{}, error)

	// GetLogrusConfig returns the config specific to Logrus
	GetLogrusConfig() (*LogrusLoggerConfig, error)

	// GetOpsRampMetricsConfig returns the config specific to PrometheusMetrics
	GetOpsRampMetricsConfig() (*OpsRampMetricsConfig, error)

	// GetUpstreamBufferSize returns the size of the libtrace buffer to use for the upstream
	// libtrace client
	GetUpstreamBufferSize() int
	// GetPeerBufferSize returns the size of the libtrace buffer to use for the peer forwarding
	// libtrace client
	GetPeerBufferSize() int

	GetIdentifierInterfaceName() (string, error)

	GetUseIPV6Identifier() (bool, error)

	GetRedisIdentifier() (string, error)

	// GetSendTickerValue returns the duration to use to check for traces to send
	GetSendTickerValue() time.Duration

	// GetDebugServiceAddr sets the IP and port the debug service will run on (you must provide the
	// command line flag -d to start the debug service)
	GetDebugServiceAddr() (string, error)

	GetIsDryRun() bool

	GetDryRunFieldName() string

	GetAddHostMetadataToTrace() bool

	GetSendMetricsToOpsRamp() bool

	// GetUseTLS returns true when TLS must be enabled to dial
	GetGlobalUseTLS() bool

	// GetUseTLSInsecureSkip returns false when certificate checks are disabled
	GetGlobalUseTLSInsecureSkip() bool

	// GetProxyProtocol returns protocol on which to listen for
	// proxy traffic
	GetProxyProtocol()(string,error )

	// GetProxyServer returns the address on which to listen for
	// proxy traffic
	GetProxyServer() (string, error)

	// GetProxyPort returns the port on which to listen for
	// proxy traffic
	GetProxyPort() (int64)

	// GetProxyUsername returns the username on which to listen for
	// proxy traffic
	GetProxyUsername()(string,error)

	// GetProxyPassword returns the password of proxy user on which to listen for
	// proxy traffic
	GetProxyPassword()(string,error)
}
