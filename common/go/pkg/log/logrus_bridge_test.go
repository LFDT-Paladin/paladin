/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package log

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureViaBridge installs the bridge, points Paladin's root logger at a buffer (mirroring
// captureRoot in log_test.go), and returns what a logrus call produced through it.
func captureViaBridge(t *testing.T, level string, emit func()) string {
	t.Helper()
	InstallLogrusBridge()

	initAtLeastOnce.Store(true)
	SetLevel(level)
	buf := &bytes.Buffer{}
	saved := rootLogger
	rootLogger = &Entry{logger: newZapLogger(buf, &Formatting{TimestampFormat: defaultTimestampFormat})}
	t.Cleanup(func() {
		rootLogger = saved
		SetLevel("info")
	})

	emit()
	return buf.String()
}

// The case this bridge exists for: the zeto go-sdk's sparse merkle tree logs through the global
// logrus logger, and those lines previously bypassed Paladin's configured destination entirely.
func TestLogrusBridgeRoutesThroughPaladinLogger(t *testing.T) {
	out := captureViaBridge(t, "info", func() {
		logrus.Infof("Upserting root node index to %s", "eec3d50e")
	})
	assert.Contains(t, out, "Upserting root node index to eec3d50e")
	// Paladin's format, not logrus's `time=... level=info msg=...`
	assert.NotContains(t, out, "level=info")
	assert.NotContains(t, out, "msg=")
}

// logrus's own writer is discarded, so a record must not also appear on stderr.
func TestLogrusBridgeDoesNotDuplicate(t *testing.T) {
	out := captureViaBridge(t, "info", func() {
		logrus.Info("only-once-marker")
	})
	assert.Equal(t, 1, strings.Count(out, "only-once-marker"))
	// logrus's own writer is discarded, so the record cannot also reach stderr
	assert.Equal(t, io.Discard, logrus.StandardLogger().Out)
}

func TestLogrusBridgeCarriesFields(t *testing.T) {
	out := captureViaBridge(t, "info", func() {
		logrus.WithField("nodeIndex", "abc123").Info("leaf committed")
	})
	assert.Contains(t, out, "leaf committed")
	assert.Contains(t, out, "nodeIndex")
	assert.Contains(t, out, "abc123")
}

// Paladin's level gates dependency logs too, so raising the level quiets the SMT chatter.
func TestLogrusBridgeRespectsPaladinLevel(t *testing.T) {
	out := captureViaBridge(t, "error", func() {
		logrus.Info("suppressed-at-error-level")
		logrus.Error("kept-at-error-level")
	})
	assert.NotContains(t, out, "suppressed-at-error-level")
	assert.Contains(t, out, "kept-at-error-level")
}

// Debug records must still reach Paladin when its level allows, which means logrus itself must not
// be filtering them out before the hook runs.
func TestLogrusBridgeDeliversDebugWhenPaladinLevelAllows(t *testing.T) {
	out := captureViaBridge(t, "debug", func() {
		logrus.Debug("dependency-debug-line")
	})
	assert.Contains(t, out, "dependency-debug-line")
}

// A context-bound Paladin logger is honoured, so dependency logs inherit component/correlation fields.
func TestLogrusBridgeUsesEntryContext(t *testing.T) {
	out := captureViaBridge(t, "info", func() {
		ctx := WithLogField(context.Background(), "smtContext", "ctx-marker")
		logrus.WithContext(ctx).Info("contextual line")
	})
	assert.Contains(t, out, "contextual line")
	assert.Contains(t, out, "ctx-marker")
}

// Fire must never call Paladin's Fatal/Panic: logrus does its own exit after hooks run, and exiting
// from inside the hook would terminate the process early and lose the remaining records.
func TestLogrusBridgeFatalDoesNotExit(t *testing.T) {
	hook := &logrusBridgeHook{}
	out := captureViaBridge(t, "info", func() {
		require.NotPanics(t, func() {
			require.NoError(t, hook.Fire(&logrus.Entry{Level: logrus.FatalLevel, Message: "fatal-from-dependency"}))
			require.NoError(t, hook.Fire(&logrus.Entry{Level: logrus.PanicLevel, Message: "panic-from-dependency"}))
		})
	})
	assert.Contains(t, out, "fatal-from-dependency")
	assert.Contains(t, out, "panic-from-dependency")
	assert.Contains(t, out, "logrusLevel")
}

func TestLogrusBridgeHookCoversAllLevels(t *testing.T) {
	assert.ElementsMatch(t, logrus.AllLevels, (&logrusBridgeHook{}).Levels())
}
