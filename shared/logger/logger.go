package logger

import (
	"context"
	"os"
	"ride-sharing/shared/env"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var base *zap.Logger

func Init(serviceName string) *zap.Logger {
	var core zapcore.Core

	if env.IsDev {
		cfg := zap.NewDevelopmentEncoderConfig()
		cfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006/01/02 15:04:05")
		cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder

		core = zapcore.NewCore(
			zapcore.NewConsoleEncoder(cfg),
			zapcore.AddSync(os.Stdout),
			zapcore.InfoLevel,
		)
	} else {
		cfg := zap.NewProductionEncoderConfig()
		cfg.EncodeTime = zapcore.ISO8601TimeEncoder

		core = zapcore.NewCore(
			zapcore.NewJSONEncoder(cfg),
			zapcore.AddSync(os.Stdout),
			zapcore.InfoLevel,
		)
	}

	base = zap.New(core, zap.AddCaller()).With(zap.String("service", serviceName))
	return base
}

func WithTrace(ctx context.Context) *zap.Logger {
	if base == nil {
		return zap.NewNop()
	}

	spanCtx := trace.SpanContextFromContext(ctx)

	if !spanCtx.IsValid() {
		return base
	}

	return base.With(
		zap.String("trace_id", spanCtx.TraceID().String()),
		zap.String("span_id", spanCtx.SpanID().String()),
	)
}

func WithTraceNoCaller(ctx context.Context) *zap.Logger {
	return WithTrace(ctx).WithOptions(zap.WithCaller(false))
}
