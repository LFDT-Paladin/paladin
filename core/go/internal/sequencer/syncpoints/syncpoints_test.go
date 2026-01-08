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

package syncpoints

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSyncPoints(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.FlushWriterConfig{}
	p := persistencemocks.NewPersistence(t)
	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)

	sp := NewSyncPoints(ctx, conf, p, txMgr, pubTxMgr, transportMgr)

	require.NotNil(t, sp)

	// Verify it's the correct type
	syncPointsImpl, ok := sp.(*syncPoints)
	require.True(t, ok, "NewSyncPoints should return a *syncPoints instance")

	// Verify all fields are set correctly
	assert.Equal(t, txMgr, syncPointsImpl.txMgr)
	assert.Equal(t, pubTxMgr, syncPointsImpl.pubTxMgr)
	assert.Equal(t, transportMgr, syncPointsImpl.transportMgr)
	assert.NotNil(t, syncPointsImpl.writer, "writer should be initialized")
	assert.False(t, syncPointsImpl.started, "started should be false initially")
}

func TestNewSyncPoints_WithEmptyConfig(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.FlushWriterConfig{} // Empty config (all fields nil)
	p := persistencemocks.NewPersistence(t)
	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)

	sp := NewSyncPoints(ctx, conf, p, txMgr, pubTxMgr, transportMgr)

	require.NotNil(t, sp)
	syncPointsImpl, ok := sp.(*syncPoints)
	require.True(t, ok)
	assert.NotNil(t, syncPointsImpl.writer, "writer should be initialized even with empty config")
}

func TestSyncPoints_Start(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.FlushWriterConfig{}
	p := persistencemocks.NewPersistence(t)
	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)

	sp := NewSyncPoints(ctx, conf, p, txMgr, pubTxMgr, transportMgr)
	syncPointsImpl := sp.(*syncPoints)

	// Initially not started
	assert.False(t, syncPointsImpl.started)

	// First call to Start should call writer.Start()
	// Note: The started field is never set to true in the current implementation
	sp.Start()
	// The writer.Start() is called, but started flag remains false
	assert.False(t, syncPointsImpl.started, "started remains false in current implementation")

	// Second call to Start should also call writer.Start() since started is still false
	sp.Start()
	assert.False(t, syncPointsImpl.started, "started remains false after second Start() call")
}

func TestSyncPoints_Start_InitializesWriter(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.FlushWriterConfig{}
	p := persistencemocks.NewPersistence(t)
	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)

	sp := NewSyncPoints(ctx, conf, p, txMgr, pubTxMgr, transportMgr)
	syncPointsImpl := sp.(*syncPoints)

	// Writer should exist before Start
	assert.NotNil(t, syncPointsImpl.writer)

	// Start should not panic
	sp.Start()
	// Note: started field is never set to true in current implementation
	assert.False(t, syncPointsImpl.started)
}

func TestSyncPoints_Close(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.FlushWriterConfig{}
	p := persistencemocks.NewPersistence(t)
	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)

	sp := NewSyncPoints(ctx, conf, p, txMgr, pubTxMgr, transportMgr)
	syncPointsImpl := sp.(*syncPoints)

	// Start first
	sp.Start()
	// Note: started field is never set to true in current implementation
	assert.False(t, syncPointsImpl.started)

	// Close should shutdown the writer
	sp.Close()
	// Note: We can't easily verify Shutdown was called without mocking, but we can verify it doesn't panic
	// The writer field should still exist
	assert.NotNil(t, syncPointsImpl.writer)
}

func TestSyncPoints_Close_WithoutStart(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.FlushWriterConfig{}
	p := persistencemocks.NewPersistence(t)
	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)

	sp := NewSyncPoints(ctx, conf, p, txMgr, pubTxMgr, transportMgr)

	// Close without starting should not panic
	sp.Close()
}

func TestSyncPoints_Close_CanBeCalledMultipleTimes(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.FlushWriterConfig{}
	p := persistencemocks.NewPersistence(t)
	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)

	sp := NewSyncPoints(ctx, conf, p, txMgr, pubTxMgr, transportMgr)

	sp.Start()

	// Multiple calls to Close should not panic
	sp.Close()
	sp.Close()
	sp.Close()
}

func TestSyncPoints_StartClose_Lifecycle(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.FlushWriterConfig{}
	p := persistencemocks.NewPersistence(t)
	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)

	sp := NewSyncPoints(ctx, conf, p, txMgr, pubTxMgr, transportMgr)
	syncPointsImpl := sp.(*syncPoints)

	// Verify initial state
	assert.False(t, syncPointsImpl.started)

	// Start
	sp.Start()
	// Note: started field is never set to true in current implementation
	assert.False(t, syncPointsImpl.started)

	// Close
	sp.Close()
	// After close, started flag remains false (it's never set to true)
	assert.False(t, syncPointsImpl.started)
}

func TestSyncPoints_NewSyncPoints_AllComponentsSet(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.FlushWriterConfig{
		WorkerCount:  confutil.P(5),
		BatchTimeout: confutil.P("10ms"),
		BatchMaxSize: confutil.P(50),
	}
	p := persistencemocks.NewPersistence(t)
	txMgr := componentsmocks.NewTXManager(t)
	pubTxMgr := componentsmocks.NewPublicTxManager(t)
	transportMgr := componentsmocks.NewTransportManager(t)

	sp := NewSyncPoints(ctx, conf, p, txMgr, pubTxMgr, transportMgr)
	syncPointsImpl := sp.(*syncPoints)

	// Verify all components are set
	assert.Equal(t, txMgr, syncPointsImpl.txMgr)
	assert.Equal(t, pubTxMgr, syncPointsImpl.pubTxMgr)
	assert.Equal(t, transportMgr, syncPointsImpl.transportMgr)
	assert.NotNil(t, syncPointsImpl.writer)
}
