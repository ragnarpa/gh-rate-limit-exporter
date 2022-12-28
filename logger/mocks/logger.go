package mocks

import (
	"github.com/ragnarpa/gh-rate-limit-exporter/logger"
)

type NopLogger struct{}

func (*NopLogger) Infof(format string, args ...any) {}
func (*NopLogger) Warnf(format string, args ...any) {}
func (*NopLogger) Info(args ...any)                 {}
func (*NopLogger) Warn(args ...any)                 {}
func (*NopLogger) Error(args ...any)                {}

var _ logger.Logger = (*NopLogger)(nil)
