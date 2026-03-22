package audit

import (
	"context"
	"log/slog"
	"time"
)

// Entry holds the data for a single audit log record.
type Entry struct {
	Tool        string
	User        string
	Method      string
	Path        string
	Query       string
	StatusCode  *int
	Blocked     bool
	Error       string
	Duration    time.Duration
	ResultCount int
}

// IntPtr returns a pointer to the given int, for use with Entry.StatusCode.
func IntPtr(v int) *int {
	return &v
}

// Logger emits structured audit log entries.
type Logger struct {
	logger *slog.Logger
}

// New creates an audit Logger backed by the given slog.Logger.
func New(logger *slog.Logger) *Logger {
	return &Logger{logger: logger}
}

// LogExecute records an execute_operation audit entry.
func (l *Logger) LogExecute(ctx context.Context, e Entry) {
	attrs := []slog.Attr{
		slog.String("tool", e.Tool),
		slog.String("method", e.Method),
		slog.String("path", e.Path),
		slog.Bool("blocked", e.Blocked),
		slog.Float64("duration_ms", float64(e.Duration.Milliseconds())),
	}

	if e.User != "" {
		attrs = append(attrs, slog.String("user", e.User))
	}
	if e.StatusCode != nil {
		attrs = append(attrs, slog.Int("status_code", *e.StatusCode))
	}

	l.emit(ctx, e.Error, attrs)
}

// LogSearch records a search_operations audit entry.
func (l *Logger) LogSearch(ctx context.Context, e Entry) {
	attrs := []slog.Attr{
		slog.String("tool", e.Tool),
		slog.String("query", e.Query),
		slog.Int("result_count", e.ResultCount),
		slog.Float64("duration_ms", float64(e.Duration.Milliseconds())),
	}

	if e.User != "" {
		attrs = append(attrs, slog.String("user", e.User))
	}

	l.emit(ctx, e.Error, attrs)
}

func (l *Logger) emit(ctx context.Context, errMsg string, attrs []slog.Attr) {
	level := slog.LevelInfo
	if errMsg != "" {
		level = slog.LevelError
		attrs = append(attrs, slog.String("error", errMsg))
	}

	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	l.logger.Log(ctx, level, "audit", args...)
}
