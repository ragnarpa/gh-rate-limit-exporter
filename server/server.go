package server

import (
	"context"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ragnarpa/gh-rate-limit-exporter/logger"
	"github.com/ragnarpa/gh-rate-limit-exporter/metrics"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/exporter"
	"go.uber.org/fx"
)

const (
	Host = "localhost"
	Port = "8080"
)

func NewHTTPServer(lc fx.Lifecycle, mux *http.ServeMux, log logger.Logger) *http.Server {
	srv := &http.Server{Addr: Host + ":" + Port, Handler: mux}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}

			log.Infof("Starting HTTP server at %v", srv.Addr)
			go srv.Serve(ln)

			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})

	return srv
}

type ServerMuxParams struct {
	fx.In

	Handler      *exporter.MetricsHandler
	Registry     *prometheus.Registry
	Instrumenter metrics.HTTPHandlerInstrumenter
}

func NewServeMux(p ServerMuxParams) *http.ServeMux {
	instrumentedHandler := p.Instrumenter.Instrument("/metrics", p.Handler)
	mux := http.NewServeMux()
	mux.Handle("/metrics", instrumentedHandler)

	return mux
}

func Module() fx.Option {
	return fx.Options(
		fx.Provide(
			NewServeMux,
			NewHTTPServer,
		),
		fx.Invoke(func(*http.Server) {}),
	)
}
