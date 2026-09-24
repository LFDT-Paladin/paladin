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
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

const defaultTimestampFormat = "2006-01-02T15:04:05.000"

// bufPool provides reusable buffers so the simple encoder never allocates a fresh
// buffer per log line.
var bufPool = buffer.NewPool()

// kvSlicePool reuses the working []kv slice used to gather and sort a line's fields,
// so encoding a line does not allocate a fresh slice each time.
var kvSlicePool = sync.Pool{New: func() any { s := make([]kv, 0, 16); return &s }}

type kv struct {
	k string
	v any
}

// simpleEncoder is a zapcore.Encoder producing Paladin's human-readable
// "[timestamp] LEVEL message key=value..." layout with fields sorted by key.
// zap's own console encoder is tab-separated, so this layout needs a custom encoder.
type simpleEncoder struct {
	tsFormat string
	utc      bool
	// fields holds the context fields accumulated via zap's .With(...) (baked in
	// by the core cloning the encoder and calling Field.AddTo), already decomposed
	// to key/value pairs.
	fields []kv
}

func newSimpleEncoder(tsFormat string, utc bool) *simpleEncoder {
	return &simpleEncoder{tsFormat: tsFormat, utc: utc}
}

func (e *simpleEncoder) Clone() zapcore.Encoder {
	clone := &simpleEncoder{tsFormat: e.tsFormat, utc: e.utc}
	if len(e.fields) > 0 {
		clone.fields = make([]kv, len(e.fields))
		copy(clone.fields, e.fields)
	}
	return clone
}

func (e *simpleEncoder) EncodeEntry(ent zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	// Decompose the per-line fields onto a working copy of the accumulated context
	// fields, routing them through Field.AddTo so every field type (strings, ints,
	// errors, ...) renders exactly as zap decomposes it. The working slice comes from
	// a pool so this does not allocate per line.
	sp := kvSlicePool.Get().(*[]kv)
	collector := &simpleEncoder{fields: append((*sp)[:0], e.fields...)}
	for _, f := range fields {
		f.AddTo(collector)
	}
	all := collector.fields

	// Field counts are tiny, so an in-place insertion sort beats sort.Slice (which
	// allocates a closure and reflects) and keeps the encode allocation-free.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].k < all[j-1].k; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}

	t := ent.Time
	if e.utc {
		t = t.UTC()
	}

	buf := bufPool.Get()
	buf.AppendByte('[')
	buf.AppendString(t.Format(e.tsFormat))
	buf.AppendString("] ")
	buf.AppendString(levelLabel(ent.Level))
	buf.AppendByte(' ')
	buf.AppendString(ent.Message)

	for i := range all {
		buf.AppendByte(' ')
		buf.AppendString(all[i].k)
		buf.AppendByte('=')
		appendValue(buf, all[i].v)
	}
	buf.AppendByte('\n')

	*sp = all[:0]
	kvSlicePool.Put(sp)
	return buf, nil
}

// appendValue renders a field value into buf. Common scalar types are written
// directly (no reflection); everything else falls back to fmt's "%+v", which is what
// the encoder used for all values previously — so output is byte-for-byte unchanged.
func appendValue(buf *buffer.Buffer, v any) {
	switch val := v.(type) {
	case string:
		buf.AppendString(val)
	case int:
		buf.AppendInt(int64(val))
	case int64:
		buf.AppendInt(val)
	case int32:
		buf.AppendInt(int64(val))
	case int16:
		buf.AppendInt(int64(val))
	case int8:
		buf.AppendInt(int64(val))
	case uint:
		buf.AppendUint(uint64(val))
	case uint64:
		buf.AppendUint(val)
	case uint32:
		buf.AppendUint(uint64(val))
	case uint16:
		buf.AppendUint(uint64(val))
	case uint8:
		buf.AppendUint(uint64(val))
	case bool:
		buf.AppendBool(val)
	default:
		// Floats, errors, durations, structs, Stringers, byte slices, ... — anything
		// whose "%+v" rendering differs from a plain strconv conversion falls through
		// here so output stays byte-for-byte identical to the previous encoder.
		_, _ = fmt.Fprintf(buf, "%+v", val)
	}
}

func (e *simpleEncoder) add(k string, v any) { e.fields = append(e.fields, kv{k, v}) }

// ObjectEncoder implementation — every Add* method decomposes to a key/value pair.
func (e *simpleEncoder) AddArray(k string, v zapcore.ArrayMarshaler) error   { e.add(k, v); return nil }
func (e *simpleEncoder) AddObject(k string, v zapcore.ObjectMarshaler) error { e.add(k, v); return nil }
func (e *simpleEncoder) AddReflected(k string, v any) error                  { e.add(k, v); return nil }
func (e *simpleEncoder) AddBinary(k string, v []byte)                        { e.add(k, v) }
func (e *simpleEncoder) AddByteString(k string, v []byte)                    { e.add(k, string(v)) }
func (e *simpleEncoder) AddBool(k string, v bool)                            { e.add(k, v) }
func (e *simpleEncoder) AddComplex128(k string, v complex128)                { e.add(k, v) }
func (e *simpleEncoder) AddComplex64(k string, v complex64)                  { e.add(k, v) }
func (e *simpleEncoder) AddDuration(k string, v time.Duration)               { e.add(k, v) }
func (e *simpleEncoder) AddFloat64(k string, v float64)                      { e.add(k, v) }
func (e *simpleEncoder) AddFloat32(k string, v float32)                      { e.add(k, v) }
func (e *simpleEncoder) AddInt(k string, v int)                              { e.add(k, v) }
func (e *simpleEncoder) AddInt64(k string, v int64)                          { e.add(k, v) }
func (e *simpleEncoder) AddInt32(k string, v int32)                          { e.add(k, v) }
func (e *simpleEncoder) AddInt16(k string, v int16)                          { e.add(k, v) }
func (e *simpleEncoder) AddInt8(k string, v int8)                            { e.add(k, v) }
func (e *simpleEncoder) AddString(k, v string)                               { e.add(k, v) }
func (e *simpleEncoder) AddTime(k string, v time.Time)                       { e.add(k, v) }
func (e *simpleEncoder) AddUint(k string, v uint)                            { e.add(k, v) }
func (e *simpleEncoder) AddUint64(k string, v uint64)                        { e.add(k, v) }
func (e *simpleEncoder) AddUint32(k string, v uint32)                        { e.add(k, v) }
func (e *simpleEncoder) AddUint16(k string, v uint16)                        { e.add(k, v) }
func (e *simpleEncoder) AddUint8(k string, v uint8)                          { e.add(k, v) }
func (e *simpleEncoder) AddUintptr(k string, v uintptr)                      { e.add(k, v) }

// OpenNamespace is a no-op: Paladin does not use zap namespaces in the simple format.
func (e *simpleEncoder) OpenNamespace(_ string) {}

// jsonEncoderConfig tunes zap's JSON encoder to match Paladin's configured field
// names, lowercase level strings and custom timestamp format. No caller key is set
// (matching the previous json handler, which emitted no source).
func jsonEncoderConfig(format *Formatting, tsFormat string) zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        format.JSONTimestampField,
		LevelKey:       format.JSONLevelField,
		MessageKey:     format.JSONMessageField,
		NameKey:        "logger",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    levelNameEncoder,
		EncodeTime:     timeEncoder(tsFormat, format.UTC),
		EncodeDuration: zapcore.StringDurationEncoder,
	}
}

// detailedEncoderConfig configures zap's console encoder for the "detailed" format,
// including the caller (source) location.
func detailedEncoderConfig(format *Formatting, tsFormat string) zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		MessageKey:     "msg",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    levelNameEncoder,
		EncodeTime:     timeEncoder(tsFormat, format.UTC),
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}

// timeEncoder reproduces the custom timestamp format and optional UTC conversion
// used by the json/detailed encoders.
func timeEncoder(tsFormat string, utc bool) zapcore.TimeEncoder {
	return func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		if utc {
			t = t.UTC()
		}
		enc.AppendString(t.Format(tsFormat))
	}
}

// levelNameEncoder writes the lowercase level name for the json/detailed formats.
// It is required because zap renders Paladin's custom levels (-2/6/7) as
// "Level(-2)" etc. by default.
func levelNameEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(levelName(l))
}

// levelName renders the lowercase level name used by the json/detailed formats.
func levelName(level zapcore.Level) string {
	switch level {
	case LevelTrace:
		return "trace"
	case zapcore.DebugLevel:
		return "debug"
	case zapcore.InfoLevel:
		return "info"
	case zapcore.WarnLevel:
		return "warn"
	case zapcore.ErrorLevel:
		return "error"
	case zapcore.PanicLevel, zapcore.DPanicLevel:
		return "panic"
	case zapcore.FatalLevel:
		return "fatal"
	default:
		return "info"
	}
}

// levelLabel renders the 5-char, uppercase level label used by the simple format,
// matching the previous formatter output exactly.
func levelLabel(level zapcore.Level) string {
	switch level {
	case zapcore.DebugLevel:
		return "DEBUG"
	case zapcore.InfoLevel:
		return " INFO"
	case zapcore.WarnLevel:
		return " WARN"
	case zapcore.ErrorLevel:
		return "ERROR"
	case zapcore.PanicLevel, zapcore.DPanicLevel:
		return "PANIC"
	case zapcore.FatalLevel:
		return "FATAL"
	case LevelTrace:
		return "TRACE"
	default:
		return " INFO"
	}
}
