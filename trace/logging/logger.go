package logging

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/dev-ofa/core-go/pass"
)

var defLog Logger = &StdoutLogger{
	logger: log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile),
}

// SetLogger for testing
func SetLogger(log Logger) {
	defLog = log
}

func buildTraceMsg(ctx context.Context) string {
	traceId, reqId := "-", "-"
	if v, ok := pass.CtxGetTraceID(ctx); ok {
		traceId = v
	}
	if v, ok := pass.CtxGetRequestID(ctx); ok {
		reqId = v
	}

	return fmt.Sprintf("trace_id: %s request_id: %s", traceId, reqId)
}

const (
	LogLevelDebug = "DEBUG"
	LogLevelInfo  = "INFO"
	LogLevelWarn  = "WARN"
	LogLevelError = "ERROR"
	LogLevelFatal = "FATAL"
)

// default logger
type StdoutLogger struct {
	logger *log.Logger
}

func (s *StdoutLogger) ctxPrintf(ctx context.Context, level string, format string, v ...any) {
	text := fmt.Sprintf("%s level: %s msg: %s", buildTraceMsg(ctx), level, fmt.Sprintf(format, v...))
	s.logger.Output(4, text)
}

func (s *StdoutLogger) printf(level string, format string, v ...any) {
	text := fmt.Sprintf("level: %s msg: %s", level, fmt.Sprintf(format, v...))
	s.logger.Output(4, text)
}

func (s *StdoutLogger) CtxDebugf(ctx context.Context, msg string, args ...interface{}) {
	s.ctxPrintf(ctx, LogLevelDebug, msg, args...)
}

func (s *StdoutLogger) CtxInfof(ctx context.Context, msg string, args ...interface{}) {
	s.ctxPrintf(ctx, LogLevelInfo, msg, args...)
}

func (s *StdoutLogger) CtxWarnf(ctx context.Context, msg string, args ...interface{}) {
	s.ctxPrintf(ctx, LogLevelError, msg, args...)
}

func (s *StdoutLogger) CtxErrorf(ctx context.Context, msg string, args ...interface{}) {
	s.ctxPrintf(ctx, LogLevelError, msg, args...)
}

func (s *StdoutLogger) CtxFatalf(ctx context.Context, msg string, args ...interface{}) {
	s.ctxPrintf(ctx, LogLevelFatal, msg, args...)
	os.Exit(1)
}

// Debugf
func (s *StdoutLogger) Debugf(msg string, args ...interface{}) {
	s.printf(LogLevelDebug, msg, args...)
}

// Infof
func (s *StdoutLogger) Infof(msg string, args ...interface{}) {
	s.printf(LogLevelInfo, msg, args...)
}

// Warnf
func (s *StdoutLogger) Warnf(msg string, args ...interface{}) {
	s.printf(LogLevelWarn, msg, args...)
}

// Errorf
func (s *StdoutLogger) Errorf(msg string, args ...interface{}) {
	s.printf(LogLevelError, msg, args...)
}

// Fatalf
func (s *StdoutLogger) Fatalf(msg string, args ...interface{}) {
	s.printf(LogLevelFatal, msg, args...)
	os.Exit(1)
}

// Logger
type Logger interface {
	CtxDebugf(ctx context.Context, msg string, args ...interface{})
	CtxInfof(ctx context.Context, msg string, args ...interface{})
	CtxWarnf(ctx context.Context, msg string, args ...interface{})
	CtxErrorf(ctx context.Context, msg string, args ...interface{})
	CtxFatalf(ctx context.Context, msg string, args ...interface{})

	Debugf(msg string, args ...interface{})
	Infof(msg string, args ...interface{})
	Warnf(msg string, args ...interface{})
	Errorf(msg string, args ...interface{})
	Fatalf(msg string, args ...interface{})
}

// Debugf
func CtxDebugf(ctx context.Context, msg string, args ...interface{}) {
	defLog.CtxDebugf(ctx, msg, args...)
}

// Infof
func CtxInfof(ctx context.Context, msg string, args ...interface{}) {
	defLog.CtxInfof(ctx, msg, args...)
}

// Warnf
func CtxWarnf(ctx context.Context, msg string, args ...interface{}) {
	defLog.CtxWarnf(ctx, msg, args...)
}

// Errorf
func CtxErrorf(ctx context.Context, msg string, args ...interface{}) {
	defLog.CtxErrorf(ctx, msg, args...)
}

// Fatalf
func CtxFatalf(ctx context.Context, msg string, args ...interface{}) {
	defLog.CtxFatalf(ctx, msg, args...)
}

// Debugf
func Debugf(msg string, args ...interface{}) {
	defLog.Debugf(msg, args...)
}

// Infof
func Infof(msg string, args ...interface{}) {
	defLog.Infof(msg, args...)
}

// Warnf
func Warnf(msg string, args ...interface{}) {
	defLog.Warnf(msg, args...)
}

// Errorf
func Errorf(msg string, args ...interface{}) {
	defLog.Errorf(msg, args...)
}

// Fatalf
func Fatalf(msg string, args ...interface{}) {
	defLog.Fatalf(msg, args...)
}
