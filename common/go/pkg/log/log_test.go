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
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// captureRoot installs a buffer-backed simple logger as the root logger so tests
// can assert on emitted output. It marks init as done so EnsureInit won't reset it.
func captureRoot() *bytes.Buffer {
	initAtLeastOnce.Store(true)
	SetLevel("trace")
	buf := &bytes.Buffer{}
	rootLogger = &Entry{logger: newZapLogger(buf, &Formatting{TimestampFormat: defaultTimestampFormat})}
	return buf
}

func TestEntryPrintfMethods(t *testing.T) {
	buf := captureRoot()
	e := L(context.Background())
	e.Tracef("tracef %d", 1)
	e.Debugf("debugf %d", 2)
	e.Infof("infof %d", 3)
	e.Printf("printf %d", 4)
	e.Warnf("warnf %d", 5)
	e.Errorf("errorf %d", 6)
	out := buf.String()
	assert.Contains(t, out, "TRACE tracef 1")
	assert.Contains(t, out, "DEBUG debugf 2")
	assert.Contains(t, out, " INFO infof 3")
	assert.Contains(t, out, " INFO printf 4")
	assert.Contains(t, out, " WARN warnf 5")
	assert.Contains(t, out, "ERROR errorf 6")
}

func TestEntryPrintMethods(t *testing.T) {
	buf := captureRoot()
	e := L(context.Background())
	e.Trace("trace msg")
	e.Debug("debug msg")
	e.Info("info msg")
	e.Warn("warn msg")
	e.Error("error msg")
	out := buf.String()
	assert.Contains(t, out, "TRACE trace msg")
	assert.Contains(t, out, "DEBUG debug msg")
	assert.Contains(t, out, " INFO info msg")
	assert.Contains(t, out, " WARN warn msg")
	assert.Contains(t, out, "ERROR error msg")
}

func TestEntryLevelGating(t *testing.T) {
	buf := captureRoot()
	SetLevel("error")
	e := L(context.Background())
	// Both the printf and print helpers must short-circuit below the active level.
	e.Debugf("suppressed %d", 1)
	e.Debug("suppressed msg")
	assert.Empty(t, buf.String())
	// A line at/above the active level still emits.
	e.Errorf("emitted %d", 2)
	assert.Contains(t, buf.String(), "ERROR emitted 2")

	// Above every emitting level, all gated helpers must short-circuit.
	atomLevel.SetLevel(zapcore.FatalLevel)
	buf.Reset()
	e.Tracef("t %d", 1)
	e.Infof("i %d", 1)
	e.Printf("p %d", 1)
	e.Warnf("w %d", 1)
	e.Errorf("e %d", 1)
	e.Trace("t")
	e.Info("i")
	e.Warn("w")
	e.Error("e")
	assert.Empty(t, buf.String())
}

func TestEntryWithError(t *testing.T) {
	buf := captureRoot()
	L(context.Background()).WithError(fmt.Errorf("boom")).Error("failed")
	out := buf.String()
	assert.Contains(t, out, "error=boom")
	assert.Contains(t, out, "failed")
}

func TestEntryWithFields(t *testing.T) {
	buf := captureRoot()
	L(context.Background()).WithFields(map[string]any{"a": 1, "b": "two"}).Info("with fields")
	out := buf.String()
	assert.Contains(t, out, "a=1")
	assert.Contains(t, out, "b=two")
}

func TestEntryFatal(t *testing.T) {
	initAtLeastOnce.Store(true)
	SetLevel("trace")
	buf := &bytes.Buffer{}
	// zap's Fatal calls os.Exit; install a fatal hook so the terminal action is a
	// recoverable panic, letting us assert the FATAL line was still emitted.
	core := zapcore.NewCore(newSimpleEncoder(defaultTimestampFormat, false), zapcore.AddSync(buf), atomLevel)
	rootLogger = &Entry{logger: zap.New(core, zap.WithFatalHook(zapcore.WriteThenPanic)).Sugar()}

	assert.Panics(t, func() { L(context.Background()).Fatalf("fatalf %d", 1) })
	assert.Contains(t, buf.String(), "FATAL fatalf 1")

	buf.Reset()
	assert.Panics(t, func() { L(context.Background()).Fatal("fatal msg") })
	assert.Contains(t, buf.String(), "FATAL fatal msg")
}

func TestEntryPanic(t *testing.T) {
	buf := captureRoot()
	assert.PanicsWithValue(t, "panicf 1", func() {
		L(context.Background()).Panicf("panicf %d", 1)
	})
	assert.Contains(t, buf.String(), "PANIC panicf 1")

	assert.PanicsWithValue(t, "panic msg", func() {
		L(context.Background()).Panic("panic msg")
	})
	assert.Contains(t, buf.String(), "PANIC panic msg")
}

func TestBufferedOutputFlush(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstate defaults (buffering off) for other tests */ }()
	initAtLeastOnce.Store(true)
	SetLevel("info")
	buf := &bytes.Buffer{}
	// A large buffer and long flush interval mean a single line stays in memory until
	// we explicitly Sync — letting us prove the write is buffered, not passed through.
	setFormatting(buf, &Formatting{
		Format:              "simple",
		TimestampFormat:     defaultTimestampFormat,
		Buffered:            true,
		BufferSize:          256 * 1024,
		BufferFlushInterval: time.Hour,
	})
	L(context.Background()).Info("buffered line")
	assert.Empty(t, buf.String(), "line should be held in the buffer, not written yet")

	require.NoError(t, Sync())
	assert.Contains(t, buf.String(), "buffered line")
}

func TestSyncNoOpWhenUnbuffered(t *testing.T) {
	captureRoot() // simple logger, buffering disabled
	assert.NoError(t, Sync())
}

func TestEnsureInitInitialisesWhenUnset(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstate defaults for other tests */ }()
	initAtLeastOnce.Store(false)
	EnsureInit()
	assert.True(t, initAtLeastOnce.Load())
}

func TestSetFormattingDefaultTimestamp(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstate defaults for other tests */ }()
	SetLevel("info")
	// An empty TimestampFormat must fall back to the default rather than emit a blank timestamp.
	buf := &bytes.Buffer{}
	setFormatting(buf, &Formatting{Format: "simple"})
	L(context.Background()).Info("defaulted timestamp")
	// The default format yields e.g. [2026-06-18T12:18:06.123] — assert it is applied.
	assert.Regexp(t, `\[\d{4}-\d{2}-\d{2}T`, buf.String())
}

func TestLogContext(t *testing.T) {
	buf := captureRoot()
	ctx := WithLogField(context.Background(), "myfield", "myvalue")
	L(ctx).Info("test message")
	assert.Contains(t, buf.String(), "myfield=myvalue")
}

func TestLogContextLimited(t *testing.T) {
	buf := captureRoot()
	ctx := WithLogField(context.Background(), "myfield", "0123456789012345678901234567890123456789012345678901234567890123456789")
	L(ctx).Info("test message")
	assert.Contains(t, buf.String(), "myfield=0123456789012345678901234567890123456789012345678901234567890...")
}

func TestSettingErrorLevel(t *testing.T) {
	SetLevel("eRrOr")
	assert.Equal(t, "error", GetLevel())
}

func TestSettingWarnLevel(t *testing.T) {
	SetLevel("WARNING")
	assert.Equal(t, "warn", GetLevel())
}

func TestSettingDebugLevel(t *testing.T) {
	SetLevel("DEBUG")
	assert.True(t, IsDebugEnabled())
	assert.Equal(t, "debug", GetLevel())
}

func TestSettingTraceLevel(t *testing.T) {
	SetLevel("trace")
	assert.True(t, IsTraceEnabled())
	assert.Equal(t, "trace", GetLevel())
}

func TestSettingInfoLevel(t *testing.T) {
	SetLevel("info")
	assert.False(t, IsDebugEnabled())
	assert.Equal(t, "info", GetLevel())
}

func TestSettingDefaultLevel(t *testing.T) {
	SetLevel("something else")
	assert.Equal(t, "info", GetLevel())
}

func TestSetFormatting(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstate defaults for other tests */ }()
	InitConfig(&pldconf.LogConfig{
		DisableColor: confutil.P(true),
		UTC:          confutil.P(true),
	})
	L(context.Background()).Infof("time in UTC")
}

func TestSetFormattingStderr(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstate defaults for other tests */ }()
	InitConfig(&pldconf.LogConfig{
		Output: confutil.P("stderr"),
	})
	L(context.Background()).Infof("code info included")
}

func TestSetFormattingStdout(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstate defaults for other tests */ }()
	InitConfig(&pldconf.LogConfig{
		Output: confutil.P("stdout"),
	})
	L(context.Background()).Infof("code info included")
}

func TestSetFormattingIncludeCodeInfo(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstate defaults for other tests */ }()
	InitConfig(&pldconf.LogConfig{
		Format: confutil.P("detailed"),
	})
	L(context.Background()).Infof("code info included")
}

func TestSetFormattingJSONEnabled(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstate defaults for other tests */ }()
	InitConfig(&pldconf.LogConfig{
		Format: confutil.P("json"),
	})
	L(context.Background()).Infof("JSON logs")
}

func TestSetFormattingFile(t *testing.T) {
	defer func() { InitConfig(&pldconf.LogConfig{}) /* reinstate defaults for other tests */ }()
	logFile := path.Join(t.TempDir(), "paladin.log")
	InitConfig(&pldconf.LogConfig{
		Output: confutil.P("file"),
		File: pldconf.LogFileConfig{
			Filename: confutil.P(logFile),
		},
	})
	L(context.Background()).Infof("File logs")

	fileExists, err := os.Stat(logFile)
	require.NoError(t, err)
	assert.False(t, fileExists.IsDir())
}

func TestLogComponent(t *testing.T) {
	buf := captureRoot()
	ctx := WithComponent(context.Background(), "mycomponent")
	L(ctx).Info("test message")
	assert.Contains(t, buf.String(), "component=mycomponent")
}

func TestLogVeryLongComponent(t *testing.T) {
	buf := captureRoot()
	ctx := WithComponent(context.Background(), "very-long-coomponent-name-0123456789012345678901234567890123456789012345678901234567890123456789")
	L(ctx).Info("test message")
	assert.Contains(t, buf.String(), "component=very-long-coomponent-name-01234567890123456789012345678901234...")
}
