package app

import (
	"net/http"

	"github.com/opsramp/tracing-proxy/collect"
	"github.com/opsramp/tracing-proxy/config"
	"github.com/opsramp/tracing-proxy/logger"
	"github.com/opsramp/tracing-proxy/metrics"
	"github.com/opsramp/tracing-proxy/route"
)

var OpsrampToken string

type App struct {
	Config         config.Config     `inject:""`
	Logger         logger.Logger     `inject:""`
	IncomingRouter route.Router      `inject:"inline"`
	PeerRouter     route.Router      `inject:"inline"`
	Collector      collect.Collector `inject:""`
	Metrics        metrics.Metrics   `inject:"metrics"`
	Client         http.Client
	// Version is the build ID for tracing-proxy so that the running process may answer
	// requests for the version
	Version string
}

// Start on the App obect should block until the proxy is shutting down. After
// Start exits, Stop will be called on all dependencies then on App then the
// program will exit.
func (a *App) Start() error {
	a.Logger.Debug().Logf("Starting up App...")
	a.IncomingRouter.SetVersion(a.Version)
	a.PeerRouter.SetVersion(a.Version)

	// launch our main routers to listen for incoming event traffic from both peers
	// and external sources
	a.IncomingRouter.LnS("incoming")
	a.PeerRouter.LnS("peer")
	a.Metrics.RegisterWithDescriptionLabels("collector_info", "gauge", "Version Of Tracing-Proxy Running", []string{"version"})
	a.Metrics.GaugeWithLabels("collector_info", map[string]string{
		"version": a.Version,
	}, 1)
	return nil
}

func (a *App) Stop() error {
	a.Logger.Debug().Logf("Shutting down App...")
	return nil
}
