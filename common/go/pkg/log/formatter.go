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
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"

	"github.com/mattn/go-isatty"
	"github.com/sirupsen/logrus"
)

const defaultTimestampFormat = "2006-01-02T15:04:05.000"

// ANSI color codes matching the defaults of logrus-prefixed-formatter
const (
	ansiReset  = "\033[0m"
	ansiGray   = "\033[37m"
	ansiBlue   = "\033[34m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
)

// simpleFormatter is a drop-in replacement for logrus-prefixed-formatter's TextFormatter.
// It produces identical output but pre-allocates no regex — the extractPrefix feature of
// the prefixed formatter (which called regexp.MustCompile on every log write) is not used
// by Paladin, so it is omitted entirely.
type simpleFormatter struct {
	DisableColors   bool
	ForceColors     bool
	TimestampFormat string
	DisableSorting  bool

	once       sync.Once
	isTerminal bool
}

func (f *simpleFormatter) initOnce(w io.Writer) {
	if file, ok := w.(*os.File); ok {
		f.isTerminal = isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
	}
}

func (f *simpleFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	f.once.Do(func() {
		if entry.Logger != nil {
			f.initOnce(entry.Logger.Out)
		}
	})

	useColor := (f.ForceColors || f.isTerminal) && !f.DisableColors

	b := entry.Buffer
	if b == nil {
		b = &bytes.Buffer{}
	}

	tsFormat := f.TimestampFormat
	if tsFormat == "" {
		tsFormat = defaultTimestampFormat
	}

	// Timestamp  e.g. [2026-06-18T12:18:06.123]
	ts := fmt.Sprintf("[%s]", entry.Time.Format(tsFormat))
	if useColor {
		ts = ansiGray + ts + ansiReset
	}

	// Level — 5 chars wide, uppercase, matching prefixed formatter output exactly
	levelText := levelLabel(entry.Level)
	if useColor {
		levelText = levelAnsi(entry.Level) + levelText + ansiReset
	}

	fmt.Fprintf(b, "%s %s %s", ts, levelText, entry.Message)

	// Fields — sorted (matching prefixed formatter default), printed as key=value
	keys := make([]string, 0, len(entry.Data))
	for k := range entry.Data {
		keys = append(keys, k)
	}
	if !f.DisableSorting {
		sort.Strings(keys)
	}
	for _, k := range keys {
		if useColor {
			fmt.Fprintf(b, " %s%s%s=%+v", levelAnsi(entry.Level), k, ansiReset, entry.Data[k])
		} else {
			fmt.Fprintf(b, " %s=%+v", k, entry.Data[k])
		}
	}

	b.WriteByte('\n')
	return b.Bytes(), nil
}

func levelLabel(level logrus.Level) string {
	switch level {
	case logrus.DebugLevel:
		return "DEBUG"
	case logrus.InfoLevel:
		return " INFO"
	case logrus.WarnLevel:
		return " WARN"
	case logrus.ErrorLevel:
		return "ERROR"
	case logrus.FatalLevel:
		return "FATAL"
	case logrus.PanicLevel:
		return "PANIC"
	case logrus.TraceLevel:
		return "TRACE"
	default:
		return " INFO"
	}
}

func levelAnsi(level logrus.Level) string {
	switch level {
	case logrus.DebugLevel, logrus.TraceLevel:
		return ansiBlue
	case logrus.InfoLevel:
		return ansiGreen
	case logrus.WarnLevel:
		return ansiYellow
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		return ansiRed
	default:
		return ansiCyan
	}
}
