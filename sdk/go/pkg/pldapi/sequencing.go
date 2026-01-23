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

package pldapi

import (
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

type SequencerActivityType string

const (
	SequencerActivityType_Dispatched      SequencerActivityType = "dispatched"
	SequencerActivityType_ChainedDispatch SequencerActivityType = "chained_dispatch"
)

type SequencerActivity struct {
	LocalID        *uint64            `docstruct:"SequencerActivity" json:"localId,omitempty"`  // only a local DB identifier for the sequencing activity
	RemoteID       string             `docstruct:"SequencerActivity" json:"remoteId,omitempty"` // Optional depending on the activity type. It may have a remote ID that correlates with something on another node
	Timestamp      pldtypes.Timestamp `docstruct:"SequencerActivity" json:"timestamp,omitempty"`
	ActivityType   string             `docstruct:"SequencerActivity" json:"activityType,omitempty"`
	SubmittingNode string             `docstruct:"SequencerActivity" json:"submittingNode,omitempty"` // The node where this activity took place
	TransactionID  uuid.UUID          `docstruct:"SequencerActivity" json:"transactionId,omitempty"`
}
