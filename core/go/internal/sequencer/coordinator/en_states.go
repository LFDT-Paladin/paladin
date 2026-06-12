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

package coordinator

import (
	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"golang.org/x/text/language"
)

var pdm = func(key, translation string) i18n.MessageKey {
	return i18n.PDM(language.AmericanEnglish, key, translation)
}

var (
	MsgStateInitial      = pdm("coordinator.State_Initial", "Coordinator state machine created")
	MsgStateIdle         = pdm("coordinator.State_Idle", "Not actively coordinating and not aware of any other active coordinators")
	MsgStateObserving    = pdm("coordinator.State_Observing", "Not actively coordinating but aware of another node actively coordinating")
	MsgStateElect        = pdm("coordinator.State_Elect", "Has sent a handover request to an active coordinator and is waiting for that node to stop coordinating")
	MsgStatePrepared     = pdm("coordinator.State_Prepared", "Has seen the previous active coordinator begin to flush and is waiting for the flush to complete")
	MsgStateActive       = pdm("coordinator.State_Active", "Actively coordinating transactions for this domain instance")
	MsgStateActiveFlush  = pdm("coordinator.State_Active_Flush", "Draining dispatched transactions while still the active coordinator (key-rotation)")
	MsgStateClosingFlush = pdm("coordinator.State_Closing_Flush", "Draining dispatched transactions after stepping down (preemption)")
	MsgStateClosing      = pdm("coordinator.State_Closing", "Has flushed and is continuing to send closing status for configured number of heartbeats")
)
