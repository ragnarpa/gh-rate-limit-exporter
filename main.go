package main

import (
	"github.com/ragnarpa/gh-rate-limit-exporter/logger"
	"github.com/ragnarpa/gh-rate-limit-exporter/metrics"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/exporter"
	"github.com/ragnarpa/gh-rate-limit-exporter/server"
	"go.uber.org/fx"
)

func module() fx.Option {
	return fx.Options(
		logger.Module(),
		metrics.Module(),
		exporter.Module(),
		server.Module(),
		fx.NopLogger,
	)
}

func main() {
	fx.New(module()).Run()
}
