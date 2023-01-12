package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/facebookgo/inject"
	"github.com/facebookgo/startstop"
	flag "github.com/jessevdk/go-flags"
	"github.com/opsramp/libtrace-go"
	"github.com/opsramp/libtrace-go/transmission"
	"github.com/opsramp/tracing-proxy/app"
	"github.com/opsramp/tracing-proxy/collect"
	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/internal/peer"
	"github.com/opsramp/tracing-proxy/logger"
	"github.com/opsramp/tracing-proxy/metrics"
	"github.com/opsramp/tracing-proxy/sample"
	"github.com/opsramp/tracing-proxy/service/debug"
	"github.com/opsramp/tracing-proxy/sharder"
	"github.com/opsramp/tracing-proxy/transmit"
)

// set by travis.
var BuildID string
var version string

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
		version = "dev"
	} else {
		version = BuildID
	}

	if opts.Version {
		fmt.Println("Version: " + version)
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
		Version: version,
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
	lgr := logger.GetLoggerImplementation()
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

	ctx, cancel := context.WithTimeout(context.Background(), c.GetPeerTimeout())
	defer cancel()
	done := make(chan struct{})
	peers, err := peer.NewPeers(ctx, c, done)

	if err != nil {
		fmt.Printf("unable to load peers: %+v\n", err)
		os.Exit(1)
	}

	// upstreamTransport is the http transport used to send things on to Honeycomb
	upstreamTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 15 * time.Second,
	}

	// peerTransport is the http transport used to send things to a local peer
	peerTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 3 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 1200 * time.Millisecond,
	}

	upstreamMetricsConfig := metrics.GetMetricsImplementation("libtrace_upstream")
	peerMetricsConfig := metrics.GetMetricsImplementation("libtrace_peer")

	opsrampkey, _ := c.GetOpsrampKey()
	opsrampsecret, _ := c.GetOpsrampSecret()
	opsrampapi, _ := c.GetOpsrampAPI()

	userAgentAddition := "tracing-proxy/" + version
	upstreamClient, err := libtrace.NewClient(libtrace.ClientConfig{
		Transmission: &transmission.Opsramptraceproxy{
			MaxBatchSize:          c.GetMaxBatchSize(),
			BatchTimeout:          c.GetBatchTimeout(),
			MaxConcurrentBatches:  libtrace.DefaultMaxConcurrentBatches,
			PendingWorkCapacity:   uint(c.GetUpstreamBufferSize()),
			UserAgentAddition:     userAgentAddition,
			Transport:             upstreamTransport,
			BlockOnSend:           true,
			EnableMsgpackEncoding: false,
			Metrics:               upstreamMetricsConfig,
			UseTls:                c.GetGlobalUseTLS(),
			UseTlsInsecure:        c.GetGlobalUseTLSInsecureSkip(),
			OpsrampKey:            opsrampkey,
			OpsrampSecret:         opsrampsecret,
			ApiHost:               opsrampapi,
		},
	})
	if err != nil {
		fmt.Printf("unable to initialize upstream libtrace client")
		os.Exit(1)
	}

	fmt.Println("upstream client created..")

	peerClient, err := libtrace.NewClient(libtrace.ClientConfig{
		Transmission: &transmission.Opsramptraceproxy{
			MaxBatchSize:          c.GetMaxBatchSize(),
			BatchTimeout:          c.GetBatchTimeout(),
			MaxConcurrentBatches:  libtrace.DefaultMaxConcurrentBatches,
			PendingWorkCapacity:   uint(c.GetPeerBufferSize()),
			UserAgentAddition:     userAgentAddition,
			Transport:             peerTransport,
			DisableCompression:    !c.GetCompressPeerCommunication(),
			EnableMsgpackEncoding: false,
			Metrics:               peerMetricsConfig,
			OpsrampKey:            opsrampkey,
			OpsrampSecret:         opsrampsecret,
			ApiHost:               opsrampapi,
		},
	})
	if err != nil {
		fmt.Printf("unable to initialize upstream libtrace client")
		os.Exit(1)
	}

	var g inject.Graph
	err = g.Provide(
		&inject.Object{Value: c},
		&inject.Object{Value: peers},
		&inject.Object{Value: lgr},
		&inject.Object{Value: upstreamTransport, Name: "upstreamTransport"},
		&inject.Object{Value: peerTransport, Name: "peerTransport"},
		&inject.Object{Value: &transmit.DefaultTransmission{LibhClient: upstreamClient, Name: "upstream_"}, Name: "upstreamTransmission"},
		&inject.Object{Value: &transmit.DefaultTransmission{LibhClient: peerClient, Name: "peer_"}, Name: "peerTransmission"},
		&inject.Object{Value: shrdr},
		&inject.Object{Value: collector},
		&inject.Object{Value: metricsConfig, Name: "metrics"},
		&inject.Object{Value: upstreamMetricsConfig, Name: "upstreamMetrics"},
		&inject.Object{Value: peerMetricsConfig, Name: "peerMetrics"},
		&inject.Object{Value: version, Name: "version"},
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

	defer startstop.Stop(g.Objects(), logrusLogger)
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
