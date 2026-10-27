// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package msgs

var (
	MsgOriginatorStateInitial   = pdm("originator.State_Initial", "Waiting for initial coordinator selection")
	MsgOriginatorStateIdle      = pdm("originator.State_Idle", "Not acting as an originator and not aware of any active coordinators")
	MsgOriginatorStateObserving = pdm("originator.State_Observing", "Not acting as an originator but aware of a node (which may be the same node) acting as a coordinator")
	MsgOriginatorStateSending   = pdm("originator.State_Sending", "Has some transactions that have been delegated to a coordinator but not yet confirmed")
)
