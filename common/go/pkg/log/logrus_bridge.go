/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the License); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package log

import (
	"io"
	"sync"

	"github.com/sirupsen/logrus"
)

// Paladin moved from logrus to zap, but several dependencies still write through the global logrus
// logger — notably the zeto go-sdk's sparse merkle tree, whose per-node "Upserting root node index"
// lines are useful when debugging SMT behaviour. Nothing in Paladin configures logrus any more, so
// those records kept logrus's stock defaults and went straight to stderr in logrus's own
// `time=... level=info msg=...` format, bypassing the configured destination entirely: on a node
// logging to a file they never reached the file, and in tests they cluttered the console.
//
// The bridge below attaches a logrus hook that re-emits each record through Paladin's logger, so
// dependency logs share the destination, format, level control and context fields of everything
// else. logrus's own output is discarded to avoid emitting each line twice.

var installLogrusBridgeOnce sync.Once

// InstallLogrusBridge routes the global logrus logger into Paladin's logger. It is called by
// InitConfig, so any process that configures Paladin logging picks it up; calling it directly is
// only needed by a process that does not. Safe to call more than once — only the first call binds.
func InstallLogrusBridge() {
	installLogrusBridgeOnce.Do(func() {
		// Let every record reach the hook and gate on Paladin's level instead, so SetLevel("debug")
		// at runtime starts including dependency debug lines without reconfiguring logrus.
		logrus.SetLevel(logrus.TraceLevel)
		// The hook does the emitting; leaving logrus's own writer attached would duplicate every line.
		logrus.SetOutput(io.Discard)
		logrus.AddHook(&logrusBridgeHook{})
	})
}

type logrusBridgeHook struct{}

func (h *logrusBridgeHook) Levels() []logrus.Level { return logrus.AllLevels }

// Fire re-emits one logrus record through Paladin's logger. Levels map one-to-one except Fatal and
// Panic: logrus performs its own os.Exit/panic once hooks have run, so this must not call Paladin's
// Fatal/Panic as well — that would exit from inside the hook and lose the remaining log lines. Those
// records are emitted at Error with the original level preserved in a field.
func (h *logrusBridgeHook) Fire(entry *logrus.Entry) error {
	e := rootLogger
	if entry.Context != nil {
		e = L(entry.Context)
	}
	for k, v := range entry.Data {
		e = e.WithField(k, v)
	}
	// Only set when a dependency has enabled logrus.SetReportCaller; Paladin's own caller field would
	// otherwise point at this hook rather than the code that logged.
	if entry.Caller != nil {
		e = e.WithField("logrusCaller", entry.Caller.Function)
	}

	switch entry.Level {
	case logrus.TraceLevel:
		e.Trace(entry.Message)
	case logrus.DebugLevel:
		e.Debug(entry.Message)
	case logrus.InfoLevel:
		e.Info(entry.Message)
	case logrus.WarnLevel:
		e.Warn(entry.Message)
	case logrus.ErrorLevel:
		e.Error(entry.Message)
	case logrus.FatalLevel, logrus.PanicLevel:
		e.WithField("logrusLevel", entry.Level.String()).Error(entry.Message)
	}
	return nil
}
