package mocks

import (
	"github.com/ragnarpa/gh-rate-limit-exporter/logger"
)

type Logger struct{}

func (*Logger) Infof(format string, args ...any) {}
func (*Logger) Warnf(format string, args ...any) {}
func (*Logger) Info(args ...any)                 {}
func (*Logger) Warn(args ...any)                 {}
func (*Logger) Error(args ...any)                {}

var _ logger.Logger = (*Logger)(nil)
