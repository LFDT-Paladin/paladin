// Copyright © 2026 Kaleido, Inc.
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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// fixedTime is a deterministic, non-UTC timestamp so the UTC conversion is observable.
var fixedTime = time.Date(2026, 6, 18, 12, 18, 6, 123000000, time.FixedZone("test", 2*60*60))

func TestSimpleEncoderPlain(t *testing.T) {
	enc := newSimpleEncoder(defaultTimestampFormat, false)
	buf, err := enc.EncodeEntry(
		zapcore.Entry{Level: zapcore.InfoLevel, Time: fixedTime, Message: "hello"},
		[]zapcore.Field{zap.String("k2", "v2"), zap.String("k1", "v1")},
	)
	require.NoError(t, err)

	out := buf.String()
	// No ANSI escapes — colour has been dropped from the simple format.
	assert.NotContains(t, out, "\033[")
	assert.Contains(t, out, "[2026-06-18T12:18:06.123]  INFO hello")
	// Fields are sorted: k1 before k2.
	assert.Less(t, strings.Index(out, "k1=v1"), strings.Index(out, "k2=v2"))
	assert.True(t, strings.HasSuffix(out, "\n"))
}

func TestSimpleEncoderUTC(t *testing.T) {
	enc := newSimpleEncoder(defaultTimestampFormat, true)
	buf, err := enc.EncodeEntry(
		zapcore.Entry{Level: zapcore.InfoLevel, Time: fixedTime, Message: "utc message"},
		nil,
	)
	require.NoError(t, err)
	// fixedTime is +02:00, so its UTC hour is 10, not 12.
	assert.Contains(t, buf.String(), "[2026-06-18T10:18:06.123]")
}

func TestSimpleEncoderAccumulatedFields(t *testing.T) {
	enc := newSimpleEncoder(defaultTimestampFormat, false)
	// Simulate zap's .With(...): the core clones the encoder and calls Field.AddTo.
	clone := enc.Clone()
	zap.String("bound", "b").AddTo(clone)

	buf, err := clone.EncodeEntry(
		zapcore.Entry{Level: zapcore.InfoLevel, Time: fixedTime, Message: "msg"},
		[]zapcore.Field{zap.String("adhoc", "a")},
	)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "adhoc=a")
	assert.Contains(t, out, "bound=b")
	// adhoc sorts before bound; accumulated and per-line fields are merged and sorted together.
	assert.Less(t, strings.Index(out, "adhoc=a"), strings.Index(out, "bound=b"))

	// Clone must not mutate the original encoder's accumulated fields.
	buf2, err := enc.EncodeEntry(zapcore.Entry{Level: zapcore.InfoLevel, Time: fixedTime, Message: "msg"}, nil)
	require.NoError(t, err)
	assert.NotContains(t, buf2.String(), "bound=b")
}

func TestSimpleEncoderError(t *testing.T) {
	enc := newSimpleEncoder(defaultTimestampFormat, false)
	buf, err := enc.EncodeEntry(
		zapcore.Entry{Level: zapcore.ErrorLevel, Time: fixedTime, Message: "failed"},
		[]zapcore.Field{zap.Error(assertError("boom"))},
	)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "error=boom")
}

type assertError string

func (e assertError) Error() string { return string(e) }

func TestSimpleEncoderCloneCopiesFields(t *testing.T) {
	e := newSimpleEncoder(defaultTimestampFormat, false)
	e.AddString("bound", "b")

	clone := e.Clone().(*simpleEncoder)
	require.Len(t, clone.fields, 1)

	// Mutating the clone must not affect the original's accumulated fields.
	clone.AddString("extra", "x")
	assert.Len(t, e.fields, 1)
	assert.Len(t, clone.fields, 2)
}

func TestSimpleEncoderObjectEncoderAllTypes(t *testing.T) {
	e := newSimpleEncoder(defaultTimestampFormat, false)

	require.NoError(t, e.AddArray("arr", zapcore.ArrayMarshalerFunc(func(zapcore.ArrayEncoder) error { return nil })))
	require.NoError(t, e.AddObject("obj", zapcore.ObjectMarshalerFunc(func(zapcore.ObjectEncoder) error { return nil })))
	require.NoError(t, e.AddReflected("refl", struct{ X int }{X: 7}))
	e.AddBinary("bin", []byte{0x01, 0x02})
	e.AddByteString("bytestr", []byte("bs"))
	e.AddBool("bool", true)
	e.AddComplex128("c128", complex(1, 2))
	e.AddComplex64("c64", complex64(complex(3, 4)))
	e.AddDuration("dur", 5*time.Second)
	e.AddFloat64("f64", 1.5)
	e.AddFloat32("f32", 2.5)
	e.AddInt("int", 1)
	e.AddInt64("int64", 2)
	e.AddInt32("int32", 3)
	e.AddInt16("int16", 4)
	e.AddInt8("int8", 5)
	e.AddString("str", "s")
	e.AddTime("time", fixedTime)
	e.AddUint("uint", 6)
	e.AddUint64("uint64", 7)
	e.AddUint32("uint32", 8)
	e.AddUint16("uint16", 9)
	e.AddUint8("uint8", 10)
	e.AddUintptr("uintptr", 11)
	e.OpenNamespace("ns") // no-op, must not panic

	buf, err := e.EncodeEntry(zapcore.Entry{Level: zapcore.InfoLevel, Time: fixedTime, Message: "types"}, nil)
	require.NoError(t, err)
	out := buf.String()

	// Scalars rendered directly by appendValue.
	assert.Contains(t, out, "bool=true")
	assert.Contains(t, out, "int=1")
	assert.Contains(t, out, "int64=2")
	assert.Contains(t, out, "int32=3")
	assert.Contains(t, out, "int16=4")
	assert.Contains(t, out, "int8=5")
	assert.Contains(t, out, "str=s")
	assert.Contains(t, out, "bytestr=bs")
	assert.Contains(t, out, "uint=6")
	assert.Contains(t, out, "uint64=7")
	assert.Contains(t, out, "uint32=8")
	assert.Contains(t, out, "uint16=9")
	assert.Contains(t, out, "uint8=10")
	// Types that fall through to the "%+v" default branch.
	assert.Contains(t, out, "f64=1.5")
	assert.Contains(t, out, "f32=2.5")
	assert.Contains(t, out, "uintptr=11")
	assert.Contains(t, out, "dur=5s")
	assert.Contains(t, out, "refl={X:7}")
	assert.Contains(t, out, "bin=[1 2]")
}

func TestLevelNames(t *testing.T) {
	for _, tc := range []struct {
		level zapcore.Level
		name  string
		label string
	}{
		{LevelTrace, "trace", "TRACE"},
		{zapcore.DebugLevel, "debug", "DEBUG"},
		{zapcore.InfoLevel, "info", " INFO"},
		{zapcore.WarnLevel, "warn", " WARN"},
		{zapcore.ErrorLevel, "error", "ERROR"},
		{zapcore.FatalLevel, "fatal", "FATAL"},
		{zapcore.PanicLevel, "panic", "PANIC"},
		{zapcore.Level(99), "info", " INFO"}, // default fall-through
	} {
		assert.Equal(t, tc.name, levelName(tc.level), "levelName(%d)", tc.level)
		assert.Equal(t, tc.label, levelLabel(tc.level), "levelLabel(%d)", tc.level)
	}
}

func TestJSONEncoderConfig(t *testing.T) {
	format := &Formatting{
		UTC:                true,
		JSONTimestampField: "@timestamp",
		JSONLevelField:     "level",
		JSONMessageField:   "message",
	}
	enc := zapcore.NewJSONEncoder(jsonEncoderConfig(format, defaultTimestampFormat))
	buf, err := enc.EncodeEntry(
		zapcore.Entry{Level: zapcore.WarnLevel, Time: fixedTime, Message: "hello"},
		[]zapcore.Field{zap.String("custom", "x")},
	)
	require.NoError(t, err)
	out := buf.String()
	// Field names are renamed, level is lowercased, timestamp formatted and UTC-converted.
	assert.Contains(t, out, `"@timestamp":"2026-06-18T10:18:06.123"`)
	assert.Contains(t, out, `"level":"warn"`)
	assert.Contains(t, out, `"message":"hello"`)
	assert.Contains(t, out, `"custom":"x"`)
}

func TestJSONEncoderConfigLocalTime(t *testing.T) {
	// UTC disabled: the timestamp keeps its original zone (+02:00 → hour 12).
	format := &Formatting{JSONTimestampField: "@timestamp", JSONLevelField: "level", JSONMessageField: "message"}
	enc := zapcore.NewJSONEncoder(jsonEncoderConfig(format, defaultTimestampFormat))
	buf, err := enc.EncodeEntry(zapcore.Entry{Level: zapcore.InfoLevel, Time: fixedTime, Message: "hi"}, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"@timestamp":"2026-06-18T12:18:06.123"`)
}

func TestJSONEncoderCustomLevel(t *testing.T) {
	// Paladin's custom trace level must render by name, not as zap's default "Level(-2)".
	format := &Formatting{JSONTimestampField: "@timestamp", JSONLevelField: "level", JSONMessageField: "message"}
	enc := zapcore.NewJSONEncoder(jsonEncoderConfig(format, defaultTimestampFormat))
	buf, err := enc.EncodeEntry(zapcore.Entry{Level: LevelTrace, Time: fixedTime, Message: "trace line"}, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"level":"trace"`)
}

func TestDetailedEncoderConfig(t *testing.T) {
	format := &Formatting{UTC: true}
	enc := zapcore.NewConsoleEncoder(detailedEncoderConfig(format, defaultTimestampFormat))
	buf, err := enc.EncodeEntry(
		zapcore.Entry{Level: zapcore.ErrorLevel, Time: fixedTime, Message: "detailed", Caller: zapcore.NewEntryCaller(0, "file.go", 42, true)},
		nil,
	)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "2026-06-18T10:18:06.123")
	assert.Contains(t, out, "error")
	assert.Contains(t, out, "detailed")
	assert.Contains(t, out, "file.go:42")
}
