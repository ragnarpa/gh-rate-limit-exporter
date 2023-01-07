package exporter

import (
	"context"
	"time"

	"github.com/ragnarpa/gh-rate-limit-exporter/metrics"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/github"
	"github.com/spf13/afero"
	"go.uber.org/fx"
)

func Module() fx.Option {
	i := Interval(30 * time.Second)
	fs := afero.Afero{Fs: afero.NewOsFs()}

	return fx.Options(
		fx.Supply(&i, &fs),
		fx.Provide(
			func(s CredentialSource) []*Credential { return s.Credentials() },
			func(i metrics.HTTPClientInstrumenter) Instrumenter { return i },
			func() HttpClientWithAppFactory { return github.NewHTTPClientForApp },
			func() HttpClientWithPATFactory { return github.NewHTTPClientForPAT },
			NewCollector,
			NewMetricsHandler,
			NewRateLimitsServiceFactory,
			fx.Annotate(NewFileCredentialSource, fx.As(new(CredentialSource))),
		),
		fx.Invoke(
			func(collector *Collector, lc fx.Lifecycle) {
				lc.Append(fx.Hook{
					OnStop: func(context.Context) error {
						collector.Shutdown()

						return nil
					},
				})
			},
		),
	)
}
