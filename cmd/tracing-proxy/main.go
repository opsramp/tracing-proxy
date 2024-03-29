package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/facebookgo/inject"
	"github.com/facebookgo/startstop"
	flag "github.com/jessevdk/go-flags"
	"github.com/opsramp/tracing-proxy/app"
	"github.com/opsramp/tracing-proxy/collect"
	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/internal/peer"
	"github.com/opsramp/tracing-proxy/logger"
	"github.com/opsramp/tracing-proxy/metrics"
	"github.com/opsramp/tracing-proxy/pkg/libtrace"
	"github.com/opsramp/tracing-proxy/pkg/libtrace/constants"
	"github.com/opsramp/tracing-proxy/pkg/libtrace/transmission"
	"github.com/opsramp/tracing-proxy/pkg/retry"
	"github.com/opsramp/tracing-proxy/proxy"
	"github.com/opsramp/tracing-proxy/sample"
	"github.com/opsramp/tracing-proxy/service/debug"
	"github.com/opsramp/tracing-proxy/sharder"
	"github.com/opsramp/tracing-proxy/transmit"
)

// set by travis.
var (
	BuildID          string
	CollectorVersion string
)

type Options struct {
	ConfigFile     string `short:"c" long:"config" description:"Path to config file" default:"/etc/tracing-proxy/config.toml"`
	RulesFile      string `short:"r" long:"rules_config" description:"Path to rules config file" default:"/etc/tracing-proxy/rules.toml"`
	Version        bool   `short:"v" long:"version" description:"Print version number and exit"`
	Debug          bool   `short:"d" long:"debug" description:"If enabled, runs debug service (runs on the first open port between localhost:6060 and :6069 by default)"`
	InterfaceNames bool   `long:"interface-names" description:"If set, print system's network interface names and exit."`
}

func main() {
	var opts Options
	flagParser := flag.NewParser(&opts, flag.Default)
	if extraArgs, err := flagParser.Parse(); err != nil || len(extraArgs) != 0 {
		fmt.Println("command line parsing error - call with --help for usage")
		os.Exit(1)
	}

	if BuildID == "" {
		CollectorVersion = "dev"
	} else {
		CollectorVersion = BuildID
	}

	if opts.Version {
		fmt.Println("Version: " + CollectorVersion)
		os.Exit(0)
	}

	if opts.InterfaceNames {
		ifaces, err := net.Interfaces()
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			os.Exit(1)
		}
		for _, i := range ifaces {
			fmt.Println(i.Name)
		}
		os.Exit(0)
	}

	a := app.App{
		Version: CollectorVersion,
	}

	c, err := config.NewConfig(opts.ConfigFile, opts.RulesFile, func(err error) {
		if a.Logger != nil {
			a.Logger.Error().WithField("error", err).Logf("error reloading config")
		}
	})
	if err != nil {
		fmt.Printf("unable to load config: %+v\n", err)
		os.Exit(1)
	}

	// get desired implementation for each dependency to inject
	lc, err := c.GetLogrusConfig()
	if err != nil {
		fmt.Printf("unable to load config: %+v\n", err)
		os.Exit(1)
	}
	lgr := logger.GetLoggerImplementation(
		lc.LogFormatter,
		lc.LogOutput,
		lc.File.FileName,
		lc.File.MaxSize,
		lc.File.MaxBackups,
		lc.File.Compress)
	collector := collect.GetCollectorImplementation(c)
	metricsConfig := metrics.GetMetricsImplementation("")
	shrdr := sharder.GetSharderImplementation(c)
	samplerFactory := &sample.SamplerFactory{}

	// set log level
	logLevel, err := c.GetLoggingLevel()
	if err != nil {
		fmt.Printf("unable to get logging level from config: %v\n", err)
		os.Exit(1)
	}
	logrusLogger := lgr.Init()
	if err := lgr.SetLevel(logLevel); err != nil {
		fmt.Printf("unable to set logging level: %v\n", err)
		os.Exit(1)
	}

	upstreamMetricsConfig := metrics.GetMetricsImplementation("upstream")
	peerMetricsConfig := metrics.GetMetricsImplementation("peer")

	authConfig := c.GetAuthConfig()
	opsrampAPI, err := c.GetOpsrampAPI()
	if err != nil {
		logrusLogger.Fatal(err)
	}
	dataset, err := c.GetDataset()
	if err != nil {
		logrusLogger.Fatal(err)
	}
	retryConfig := c.GetRetryConfig()
	logsEndpoint := c.GetLogsEndpoint()
	sendEvents := c.GetSendEvents()

	// set proxy details
	prxy := proxy.NewProxy(c.GetProxyConfig(), opsrampAPI, logsEndpoint, authConfig.Endpoint, c.GetMetricsConfig().OpsRampAPI)
	prxy.Logger = lgr
	// connect to working proxy at start-up
	if prxy.Enabled() {
		_ = prxy.SwitchProxy()
		lgr.Info().Logf("valid proxy config received: %d", prxy.Len())
	}

	userAgentAddition := "tracing-proxy/" + CollectorVersion
	upstreamClient, err := libtrace.NewClient(libtrace.ClientConfig{ // nolint:all
		Logger: lgr,
		Transmission: &transmission.TraceProxy{
			MaxBatchSize:          c.GetMaxBatchSize(),
			BatchTimeout:          c.GetBatchTimeout(),
			MaxConcurrentBatches:  constants.DefaultMaxConcurrentBatches,
			PendingWorkCapacity:   uint(c.GetUpstreamBufferSize()),
			UserAgentAddition:     userAgentAddition,
			BlockOnSend:           true,
			EnableMsgpackEncoding: false,
			Metrics:               upstreamMetricsConfig,
			IsPeer:                false,
			UseTls:                c.GetGlobalUseTLS(),
			UseTlsInsecure:        c.GetGlobalUseTLSInsecureSkip(),
			AuthTokenEndpoint:     authConfig.Endpoint,
			AuthTokenKey:          authConfig.Key,
			AuthTokenSecret:       authConfig.Secret,
			TraceEndpoint:         opsrampAPI,
			TenantId:              authConfig.TenantId,
			Dataset:               dataset,
			RetrySettings: &retry.Config{
				InitialInterval:     retryConfig.InitialInterval,
				RandomizationFactor: retryConfig.RandomizationFactor,
				Multiplier:          retryConfig.Multiplier,
				MaxInterval:         retryConfig.MaxInterval,
				MaxElapsedTime:      retryConfig.MaxElapsedTime,
			},
			Logger:       lgr,
			LogsEndpoint: logsEndpoint,
			SendEvents:   sendEvents,
			Proxy:        prxy,
		},
	})
	if err != nil {
		fmt.Printf("unable to initialize upstream libtrace client: %v", err)
		os.Exit(1)
	}

	peerClient, err := libtrace.NewClient(libtrace.ClientConfig{ // nolint:all
		Logger: lgr,
		Transmission: &transmission.TraceProxy{
			MaxBatchSize:          c.GetMaxBatchSize(),
			BatchTimeout:          c.GetBatchTimeout(),
			MaxConcurrentBatches:  constants.DefaultMaxConcurrentBatches,
			PendingWorkCapacity:   uint(c.GetPeerBufferSize()),
			UserAgentAddition:     userAgentAddition,
			DisableCompression:    !c.GetCompressPeerCommunication(),
			EnableMsgpackEncoding: false,
			Metrics:               peerMetricsConfig,
			IsPeer:                true,
			AuthTokenEndpoint:     authConfig.Endpoint,
			AuthTokenKey:          authConfig.Key,
			AuthTokenSecret:       authConfig.Secret,
			TraceEndpoint:         opsrampAPI,
			TenantId:              authConfig.TenantId,
			Dataset:               dataset,
			RetrySettings: &retry.Config{
				InitialInterval:     retryConfig.InitialInterval,
				RandomizationFactor: retryConfig.RandomizationFactor,
				Multiplier:          retryConfig.Multiplier,
				MaxInterval:         retryConfig.MaxInterval,
				MaxElapsedTime:      retryConfig.MaxElapsedTime,
			},
			Logger:       lgr,
			LogsEndpoint: logsEndpoint,
			SendEvents:   sendEvents,
			Proxy:        prxy,
		},
	})
	if err != nil {
		fmt.Printf("unable to initialize upstream libtrace client: %v", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.GetPeerTimeout())
	defer cancel()
	done := make(chan struct{})
	peers, err := peer.NewPeers(ctx, c, done)
	if err != nil {
		fmt.Printf("unable to load peers: %+v\n", err)
		os.Exit(1)
	}

	var g inject.Graph
	err = g.Provide(
		&inject.Object{Value: c},
		&inject.Object{Value: peers},
		&inject.Object{Value: lgr},
		&inject.Object{Value: prxy, Name: "proxyConfig"},
		&inject.Object{
			Name: "upstreamTransmission",
			Value: &transmit.DefaultTransmission{
				Name:   "upstream_",
				Client: upstreamClient,
			},
		},
		&inject.Object{
			Name: "peerTransmission",
			Value: &transmit.DefaultTransmission{
				Name:   "peer_",
				Client: peerClient,
			},
		},
		&inject.Object{Value: shrdr},
		&inject.Object{Value: collector},
		&inject.Object{Value: metricsConfig, Name: "metrics"},
		&inject.Object{Value: upstreamMetricsConfig, Name: "upstreamMetrics"},
		&inject.Object{Value: peerMetricsConfig, Name: "peerMetrics"},
		&inject.Object{Value: CollectorVersion, Name: "version"},
		&inject.Object{Value: samplerFactory},
		&inject.Object{Value: &a},
	)
	if err != nil {
		fmt.Printf("failed to provide injection graph. error: %+v\n", err)
		os.Exit(1)
	}

	if opts.Debug {
		err = g.Provide(&inject.Object{Value: &debug.DebugService{Config: c}})
		if err != nil {
			fmt.Printf("failed to provide injection graph. error: %+v\n", err)
			os.Exit(1)
		}
	}

	if err := g.Populate(); err != nil {
		fmt.Printf("failed to populate injection graph. error: %+v\n", err)
		os.Exit(1)
	}

	defer func(objects []*inject.Object, log startstop.Logger) {
		err := startstop.Stop(objects, log)
		if err != nil {
			fmt.Printf("failed to stop injected depencies. error: %+v\n", err)
		}
	}(g.Objects(), logrusLogger)
	if err := startstop.Start(g.Objects(), logrusLogger); err != nil {
		fmt.Printf("failed to start injected dependencies. error: %+v\n", err)
		os.Exit(1)
	}

	// set up signal channel to exit
	sigsToExit := make(chan os.Signal, 1)
	signal.Notify(sigsToExit, syscall.SIGINT, syscall.SIGTERM)

	// block on our signal handler to exit
	sig := <-sigsToExit
	// unregister ourselves before we go
	close(done)
	time.Sleep(100 * time.Millisecond)
	a.Logger.Error().Logf("Caught signal \"%s\"", sig)
}
