package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
)

func Module() fx.Option {
	return fx.Options(
		fx.Provide(
			NewRegistry,
			func() buckets { return prometheus.ExponentialBuckets(0.1, 1.5, 5) },
			fx.Annotate(NewHTTPClientInstrumenter, fx.As(new(HTTPClientInstrumenter))),
			fx.Annotate(NewHTTPHandlerInstrumenter, fx.As(new(HTTPHandlerInstrumenter))),
		),
	)
}
