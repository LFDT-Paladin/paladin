// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package log

import (
	"context"
	"io"
	"math"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// Paladin was originally built using logrus and later migrated to zap.
// Paladin logs through a logrus-style FieldLogger API (log.L(ctx).Infof(...)) at
// every call site. The engine underneath is Uber's zap; this package is a thin
// adaptation layer that exposes zap through that API so the call sites are unaffected.

// LevelTrace adds a level below zap's Debug(-1) to carry logrus's Trace level. The
// code base uses Tracef/Trace but zap has no Trace level of its own; every other
// level maps one-to-one onto zap (Debug/Info/Warn/Error, and the terminal Panic/Fatal).
const LevelTrace = zapcore.Level(-2) // just below Debug(-1)

var (
	// atomLevel is the single global level control, shared by every core so
	// SetLevel takes effect immediately across all loggers (matching logrus).
	atomLevel = zap.NewAtomicLevelAt(zapcore.InfoLevel)

	rootLogger = defaultRootEntry()

	// L accesses the current logger from the context
	L = loggerFromContext

	initAtLeastOnce atomic.Bool
)

type Component string

type (
	ctxLogKey struct{}
)

// Entry is a package-local wrapper around *zap.SugaredLogger that presents the
// logrus FieldLogger method set — Tracef/Infof/…, Trace/Info/…, WithField/WithError/
// WithFields — used throughout Paladin, so L(ctx) call sites need no changes. Each
// method forwards to its zap equivalent.
type Entry struct {
	logger *zap.SugaredLogger
}

func defaultRootEntry() *Entry {
	return &Entry{logger: newZapLogger(os.Stderr, &Formatting{TimestampFormat: defaultTimestampFormat})}
}

// Printf-style methods. Trace has no dedicated zap method, so it routes through the
// generic Logf at LevelTrace; the rest forward to the SugaredLogger's own methods.
func (e *Entry) Tracef(format string, args ...any) { e.logger.Logf(LevelTrace, format, args...) }
func (e *Entry) Debugf(format string, args ...any) { e.logger.Debugf(format, args...) }
func (e *Entry) Infof(format string, args ...any)  { e.logger.Infof(format, args...) }
func (e *Entry) Printf(format string, args ...any) { e.logger.Infof(format, args...) }
func (e *Entry) Warnf(format string, args ...any)  { e.logger.Warnf(format, args...) }
func (e *Entry) Errorf(format string, args ...any) { e.logger.Errorf(format, args...) }

// Fatalf/Panicf delegate to zap's native terminal methods, which log then
// os.Exit(1) / panic(message) exactly as logrus did.
func (e *Entry) Fatalf(format string, args ...any) { e.logger.Fatalf(format, args...) }
func (e *Entry) Panicf(format string, args ...any) { e.logger.Panicf(format, args...) }

// Print-style methods
func (e *Entry) Trace(args ...any) { e.logger.Log(LevelTrace, args...) }
func (e *Entry) Debug(args ...any) { e.logger.Debug(args...) }
func (e *Entry) Info(args ...any)  { e.logger.Info(args...) }
func (e *Entry) Warn(args ...any)  { e.logger.Warn(args...) }
func (e *Entry) Error(args ...any) { e.logger.Error(args...) }
func (e *Entry) Fatal(args ...any) { e.logger.Fatal(args...) }
func (e *Entry) Panic(args ...any) { e.logger.Panic(args...) }

// Field builders return a new *Entry (mirroring logrus's immutable chaining).
// SugaredLogger.With takes loosely-typed key, value pairs.
func (e *Entry) WithField(key string, value any) *Entry {
	return &Entry{logger: e.logger.With(key, value)}
}

func (e *Entry) WithError(err error) *Entry {
	return &Entry{logger: e.logger.With("error", err)}
}

func (e *Entry) WithFields(fields map[string]any) *Entry {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Entry{logger: e.logger.With(args...)}
}

func InitConfig(conf *pldconf.LogConfig) {
	initAtLeastOnce.Store(true) // must store before SetLevel

	level := confutil.StringNotEmpty(conf.Level, *pldconf.LogDefaults.Level)
	SetLevel(level)

	output := confutil.StringNotEmpty(conf.Output, *pldconf.LogDefaults.Output)
	var out io.Writer
	switch output {
	case "file":
		filename := confutil.StringNotEmpty(conf.File.Filename, *pldconf.LogDefaults.File.Filename)
		rootLogger.Infof("Logs diverted to %s", filename)
		maxSizeBytes := confutil.ByteSize(conf.File.MaxSize, 0, *pldconf.LogDefaults.File.MaxSize)
		maxAgeDuration := confutil.DurationMin(conf.File.MaxAge, 0, *pldconf.LogDefaults.File.MaxAge)
		out = &lumberjack.Logger{
			Filename:   filename,
			MaxSize:    int(math.Ceil(float64(maxSizeBytes) / 1024 / 1024)), /* round up in megabytes */
			MaxBackups: confutil.IntMin(conf.File.MaxBackups, 0, *pldconf.LogDefaults.File.MaxBackups),
			MaxAge:     int(math.Ceil(float64(maxAgeDuration) / float64(time.Hour) / 24)), /* round up in days */
			Compress:   confutil.Bool(conf.File.Compress, *pldconf.LogDefaults.File.Compress),
		}
	case "stdout":
		out = os.Stdout
	case "stderr":
		fallthrough
	default:
		out = os.Stderr
	}

	setFormatting(out, &Formatting{
		Format:              confutil.StringNotEmpty(conf.Format, *pldconf.LogDefaults.Format),
		DisableColor:        confutil.Bool(conf.DisableColor, *pldconf.LogDefaults.DisableColor),
		ForceColor:          confutil.Bool(conf.ForceColor, *pldconf.LogDefaults.ForceColor),
		TimestampFormat:     confutil.StringNotEmpty(conf.TimeFormat, *pldconf.LogDefaults.TimeFormat),
		UTC:                 confutil.Bool(conf.UTC, *pldconf.LogDefaults.UTC),
		JSONTimestampField:  confutil.StringNotEmpty(conf.JSON.TimestampField, *pldconf.LogDefaults.JSON.TimestampField),
		JSONLevelField:      confutil.StringNotEmpty(conf.JSON.LevelField, *pldconf.LogDefaults.JSON.LevelField),
		JSONMessageField:    confutil.StringNotEmpty(conf.JSON.MessageField, *pldconf.LogDefaults.JSON.MessageField),
		JSONFuncField:       confutil.StringNotEmpty(conf.JSON.FuncField, *pldconf.LogDefaults.JSON.FuncField),
		JSONFileField:       confutil.StringNotEmpty(conf.JSON.FileField, *pldconf.LogDefaults.JSON.FileField),
		Buffered:            confutil.Bool(conf.Buffer.Enabled, *pldconf.LogDefaults.Buffer.Enabled),
		BufferSize:          int(confutil.ByteSize(conf.Buffer.Size, 0, *pldconf.LogDefaults.Buffer.Size)),
		BufferFlushInterval: confutil.DurationMin(conf.Buffer.FlushInterval, 0, *pldconf.LogDefaults.Buffer.FlushInterval),
	})
}

func IsDebugEnabled() bool {
	return atomLevel.Enabled(zapcore.DebugLevel)
}

func IsTraceEnabled() bool {
	return atomLevel.Enabled(LevelTrace)
}

func EnsureInit() {
	// Called at a couple of strategic points to check we get log initialize in things like unit tests
	// However NOT guaranteed to be called because we can't afford to do atomic load on every log line
	if !initAtLeastOnce.Load() {
		InitConfig(&pldconf.LogConfig{})
	}
}

// WithLogger adds the specified logger to the context
func WithLogger(ctx context.Context, logger *Entry) context.Context {
	EnsureInit()
	return context.WithValue(ctx, ctxLogKey{}, logger)
}

// WithLogField adds the specified field to the logger in the context
func WithLogField(ctx context.Context, key, value string) context.Context {
	EnsureInit()
	if len(value) > 61 {
		value = value[0:61] + "..."
	}
	return WithLogger(ctx, loggerFromContext(ctx).WithField(key, value))
}

// WithComponent adds the specified component to the logger in the context
func WithComponent(ctx context.Context, component Component) context.Context {
	EnsureInit()
	if len(component) > 61 {
		component = component[0:61] + "..."
	}
	return WithLogger(ctx, loggerFromContext(ctx).WithField("component", component))
}

// loggerFromContext returns the logger for the current context, or the root logger if there is none
func loggerFromContext(ctx context.Context) *Entry {
	logger := ctx.Value(ctxLogKey{})
	if logger == nil {
		return rootLogger
	}
	return logger.(*Entry)
}

func GetLevel() string {
	switch atomLevel.Level() {
	case zapcore.ErrorLevel:
		return "error"
	case zapcore.WarnLevel:
		return "warn"
	case zapcore.DebugLevel:
		return "debug"
	case LevelTrace:
		return "trace"
	default:
		return "info"
	}
}

func SetLevel(level string) {
	var l zapcore.Level
	switch strings.ToLower(level) {
	case "error":
		l = zapcore.ErrorLevel
	case "warn", "warning":
		l = zapcore.WarnLevel
	case "debug":
		l = zapcore.DebugLevel
	case "trace":
		l = LevelTrace
	default:
		l = zapcore.InfoLevel
	}
	atomLevel.SetLevel(l)
}

type Formatting struct {
	Format              string
	DisableColor        bool
	ForceColor          bool
	TimestampFormat     string
	UTC                 bool
	JSONTimestampField  string
	JSONLevelField      string
	JSONMessageField    string
	JSONFuncField       string
	JSONFileField       string
	Buffered            bool
	BufferSize          int
	BufferFlushInterval time.Duration
}

// bufferedWS holds the buffered write syncer currently in use, or nil when buffering
// is disabled. Kept so Sync() can flush it and so a reconfigure can stop its flush
// goroutine before installing a replacement.
var bufferedWS *zapcore.BufferedWriteSyncer

// Sync flushes any buffered log output. It is a no-op when buffering is disabled.
// Call it on graceful shutdown so lines held in the buffer are not lost. (Lines at
// Error level and above, and the terminal Fatal/Panic paths, are flushed automatically
// by zap's core, so a crash still emits its final log line.)
func Sync() error {
	if bufferedWS != nil {
		return bufferedWS.Sync()
	}
	return nil
}

// newZapLogger builds a *zap.SugaredLogger for the given writer and formatting.
// The atomLevel is passed as the core's LevelEnabler so SetLevel takes effect live.
func newZapLogger(out io.Writer, format *Formatting) *zap.SugaredLogger {
	tsFormat := format.TimestampFormat
	if tsFormat == "" {
		tsFormat = defaultTimestampFormat
	}
	ws := zapcore.AddSync(out)
	if format.Buffered {
		// Batch log lines in memory to cut the write syscall per line. Size/FlushInterval
		// of 0 fall back to zap's defaults (256kB / 30s). The flush goroutine is stopped
		// by setFormatting when the logger is replaced.
		bufferedWS = &zapcore.BufferedWriteSyncer{
			WS:            ws,
			Size:          format.BufferSize,
			FlushInterval: format.BufferFlushInterval,
		}
		ws = bufferedWS
	}

	var enc zapcore.Encoder
	var opts []zap.Option
	switch format.Format {
	case "json":
		enc = zapcore.NewJSONEncoder(jsonEncoderConfig(format, tsFormat))
	case "detailed":
		enc = zapcore.NewConsoleEncoder(detailedEncoderConfig(format, tsFormat))
		// Sugar() already adds +2 caller skip for its two frames; our Entry method
		// layer adds one more, so the reported caller is the real call site.
		opts = append(opts, zap.AddCaller(), zap.AddCallerSkip(1))
	default: // "simple"
		enc = newSimpleEncoder(tsFormat, format.UTC)
	}

	core := zapcore.NewCore(enc, ws, atomLevel)
	return zap.New(core, opts...).Sugar()
}

func setFormatting(out io.Writer, format *Formatting) {
	// Stop the previous buffered syncer (if any) so its flush goroutine does not leak
	// when the logger is reconfigured. newZapLogger installs the replacement.
	if bufferedWS != nil {
		_ = bufferedWS.Stop()
		bufferedWS = nil
	}
	rootLogger = &Entry{logger: newZapLogger(out, format)}
}
