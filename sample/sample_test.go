//go:build all || race
// +build all race

package sample

import (
	"testing"

	"github.com/facebookgo/inject"
	"github.com/jirs5/tracing-proxy/config"
	"github.com/jirs5/tracing-proxy/logger"
	"github.com/jirs5/tracing-proxy/metrics"
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
