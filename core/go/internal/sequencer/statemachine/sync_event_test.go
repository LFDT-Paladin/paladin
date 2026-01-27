/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
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

package statemachine

import (
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/stretchr/testify/assert"
)

func TestSyncEvent_Type(t *testing.T) {
	se := &SyncEvent{}
	assert.Equal(t, common.EventType(-1), se.Type())
}

func TestSyncEvent_TypeString(t *testing.T) {
	se := &SyncEvent{}
	assert.Equal(t, "SyncEvent", se.TypeString())
}

func TestNewSyncEvent(t *testing.T) {
	se := NewSyncEvent()
	assert.NotNil(t, se.Done)
}

func TestIsSyncEvent_WithSyncEvent(t *testing.T) {
	se := NewSyncEvent()
	out, ok := IsSyncEvent(se)
	assert.True(t, ok)
	assert.Same(t, se, out)
}

func TestIsSyncEvent_WithNonSyncEvent(t *testing.T) {
	ev := newTestEvent(Event_Start)
	out, ok := IsSyncEvent(ev)
	assert.False(t, ok)
	assert.Nil(t, out)
}
