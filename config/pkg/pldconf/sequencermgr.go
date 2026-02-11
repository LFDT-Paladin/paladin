/*
 * Copyright © 2025 Kaleido, Inc.
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
package pldconf

import (
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
)

type SequencerConfig struct {
	AssembleTimeout               *string           `json:"assembleTimeout"`
	RequestTimeout                *string           `json:"requestTimeout"`
	BlockHeightTolerance          *uint64           `json:"blockHeightTolerance"`
	BlockRange                    *uint64           `json:"blockRange"`
	ClosingGracePeriod            *int              `json:"closingGracePeriod"`
	DelegateTimeout               *string           `json:"delegateTimeout"`
	HeartbeatInterval             *string           `json:"heartbeatInterval"`
	HeartbeatThreshold            *int              `json:"heartbeatThreshold"`
	MaxInflightTransactions       *int              `json:"maxInflightTransactions"`
	MaxDispatchAhead              *int              `json:"maxDispatchAhead"`
	TargetActiveCoordinators      *int              `json:"targetActiveCoordinators"`
	TargetActiveSequencers        *int              `json:"targetActiveSequencers"`
	TransactionResumePollInterval *string           `json:"transactionResumePollInterval"`
	Writer                        FlushWriterConfig `json:"writer"`
}

type SequencerMinimumConfig struct {
	AssembleTimeout               time.Duration
	RequestTimeout                time.Duration
	BlockHeightTolerance          uint64
	BlockRange                    uint64
	ClosingGracePeriod            int
	DelegateTimeout               time.Duration
	HeartbeatInterval             time.Duration
	MaxInflightTransactions       int
	MaxDispatchAhead              int
	TargetActiveCoordinators      int
	TargetActiveSequencers        int
	TransactionResumePollInterval time.Duration
}

var SequencerDefaults = SequencerConfig{
	Writer: FlushWriterConfig{
		WorkerCount:  confutil.P(10),
		BatchTimeout: confutil.P("25ms"),
		BatchMaxSize: confutil.P(100),
	},
	AssembleTimeout:               confutil.P("10s"), // Time before giving up on assembly of the in progress transaction and re-pooling it
	RequestTimeout:                confutil.P("3s"),  // Time before sending 1 retry of an assemble request, endorsement request etc.
	BlockHeightTolerance:          confutil.P(uint64(5)),
	BlockRange:                    confutil.P(uint64(100)),
	ClosingGracePeriod:            confutil.P(4),
	DelegateTimeout:               confutil.P("5s"),
	HeartbeatInterval:             confutil.P("10s"),
	MaxInflightTransactions:       confutil.P(500),
	MaxDispatchAhead:              confutil.P(50),
	TargetActiveCoordinators:      confutil.P(50),
	TargetActiveSequencers:        confutil.P(50),
	TransactionResumePollInterval: confutil.P("5m"),
}

var SequencerMinimum = SequencerMinimumConfig{
	AssembleTimeout:               1 * time.Second,
	RequestTimeout:                1 * time.Second,
	BlockHeightTolerance:          1,
	BlockRange:                    10,
	ClosingGracePeriod:            1,
	DelegateTimeout:               100 * time.Millisecond,
	HeartbeatInterval:             1 * time.Second,
	MaxInflightTransactions:       1,
	MaxDispatchAhead:              1,
	TargetActiveCoordinators:      10,
	TargetActiveSequencers:        10,
	TransactionResumePollInterval: 10 * time.Second,
}
