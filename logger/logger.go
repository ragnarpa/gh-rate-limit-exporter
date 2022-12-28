package logger

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Info(args ...any)
	Warn(args ...any)
	Error(args ...any)
}

func NewLogger() (Logger, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}

	return logger.Sugar(), nil
}

func Module() fx.Option {
	return fx.Provide(NewLogger)
}
