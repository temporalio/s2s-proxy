package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
)

var Module = fx.Provide(func() (*Registry, error) {
	reg := prometheus.NewRegistry()
	return NewRegistry(reg, reg)
})
