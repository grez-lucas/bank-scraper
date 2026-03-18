package debug

import (
	"log/slog"
	"time"
)

// OpLogger emits structured logs scoped to one operation, with automatic
// duration tracking. All entries include "operation" and "duration_ms" keys.
type OpLogger struct {
	logger    *slog.Logger
	operation string
	start     time.Time
}

// StartOp creates an OpLogger and emits an Info log marking operation start.
func StartOp(logger *slog.Logger, operation string, attrs ...slog.Attr) *OpLogger {
	o := &OpLogger{
		logger:    logger,
		operation: operation,
		start:     time.Now(),
	}
	logger.Info("starting "+operation, o.buildArgs(false, attrs...)...)
	return o
}

// Success logs operation completion at Info level with duration.
func (o *OpLogger) Success(attrs ...slog.Attr) {
	o.logger.Info(o.operation+" completed", o.buildArgs(true, attrs...)...)
}

// Error logs operation failure at Error level with duration.
func (o *OpLogger) Error(msg string, err error, attrs ...slog.Attr) {
	args := o.buildArgs(true, attrs...)
	args = append(args, slog.Any("error", err))
	o.logger.Error(o.operation+": "+msg, args...)
}

// Warn logs a warning at Warn level with duration.
func (o *OpLogger) Warn(msg string, attrs ...slog.Attr) {
	o.logger.Warn(o.operation+": "+msg, o.buildArgs(true, attrs...)...)
}

// Info logs a mid-operation progress message at Info level.
func (o *OpLogger) Info(msg string, attrs ...slog.Attr) {
	o.logger.Info(o.operation+": "+msg, o.buildArgs(false, attrs...)...)
}

// buildArgs constructs the log args slice with operation key, optional duration,
// and any caller-provided attrs.
func (o *OpLogger) buildArgs(withDuration bool, attrs ...slog.Attr) []any {
	args := make([]any, 0, 2+len(attrs))
	args = append(args, slog.String("operation", o.operation))
	if withDuration {
		args = append(args, slog.Int64("duration_ms", time.Since(o.start).Milliseconds()))
	}
	for _, a := range attrs {
		args = append(args, any(a))
	}
	return args
}
