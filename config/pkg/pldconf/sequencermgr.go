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
	AssembleErrorRetryThreshold       *int              `json:"assembleErrorRetryThreshold"`
	BaseLedgerRevertRetryThreshold    *int              `json:"baseLedgerRevertRetryThreshold"`
	BlockHeightTolerance              *uint64           `json:"blockHeightTolerance"`
	BlockRange                        *uint64           `json:"blockRange"`
	ClosingGracePeriod                *int              `json:"closingGracePeriod"`
	CoordinatorEventQueueSize         *int              `json:"coordinatorEventQueueSize"`
	CoordinatorPriorityEventQueueSize *int              `json:"coordinatorPriorityEventQueueSize"`
	HeartbeatInterval                 *string           `json:"heartbeatInterval"`
	IdleSequencerCleanupInterval      *string           `json:"idleSequencerCleanupInterval"`
	InactiveGracePeriod               *int              `json:"inactiveGracePeriod"`
	MaxDispatchAhead                  *int              `json:"maxDispatchAhead"`
	MaxDispatchBatchSize              *int              `json:"maxDispatchBatchSize"`
	MaxInflightTransactions           *int              `json:"maxInflightTransactions"`
	OriginatorEventQueueSize          *int              `json:"originatorEventQueueSize"`
	OriginatorPriorityEventQueueSize  *int              `json:"originatorPriorityEventQueueSize"`
	RequestTimeout                    *string           `json:"requestTimeout"`
	SignErrorRetryThreshold           *int              `json:"signErrorRetryThreshold"`
	StateTimeout                      *string           `json:"stateTimeout"`
	TargetActiveSequencers            *int              `json:"targetActiveSequencers"`
	TransactionResumeMaxTransactions  *int              `json:"transactionResumeMaxTransactions"`
	TransactionResumePageSize         *int              `json:"transactionResumePageSize"`
	TransactionResumePollInterval     *string           `json:"transactionResumePollInterval"`
	Writer                            FlushWriterConfig `json:"writer"`
}

type SequencerMinimumConfig struct {
	AssembleErrorRetryThreshold       int
	BaseLedgerRevertRetryThreshold    int
	BlockHeightTolerance              uint64
	BlockRange                        uint64
	ClosingGracePeriod                int
	CoordinatorEventQueueSize         int
	CoordinatorPriorityEventQueueSize int
	HeartbeatInterval                 time.Duration
	IdleSequencerCleanupInterval      time.Duration
	InactiveGracePeriod               int
	MaxDispatchAhead                  int
	MaxDispatchBatchSize              int
	MaxInflightTransactions           int
	OriginatorEventQueueSize          int
	OriginatorPriorityEventQueueSize  int
	RequestTimeout                    time.Duration
	SignErrorRetryThreshold           int
	StateTimeout                      time.Duration
	TargetActiveSequencers            int
	TransactionResumeMaxTransactions  int
	TransactionResumePageSize         int
	TransactionResumePollInterval     time.Duration
}

var SequencerDefaults = SequencerConfig{
	AssembleErrorRetryThreshold:       confutil.P(3),
	BaseLedgerRevertRetryThreshold:    confutil.P(3),
	BlockHeightTolerance:              confutil.P(uint64(5)),
	BlockRange:                        confutil.P(uint64(100)),
	ClosingGracePeriod:                confutil.P(2),
	CoordinatorEventQueueSize:         confutil.P(100),
	CoordinatorPriorityEventQueueSize: confutil.P(500),
	HeartbeatInterval:                 confutil.P("10s"),
	IdleSequencerCleanupInterval:      confutil.P("1m"),
	InactiveGracePeriod:               confutil.P(2),
	MaxDispatchAhead:                  confutil.P(50),
	MaxDispatchBatchSize:              confutil.P(20),
	MaxInflightTransactions:           confutil.P(500),
	OriginatorEventQueueSize:          confutil.P(50),
	OriginatorPriorityEventQueueSize:  confutil.P(500),
	RequestTimeout:                    confutil.P("3s"), // Time before sending 1 retry of an assemble request, endorsement request etc
	SignErrorRetryThreshold:           confutil.P(3),
	StateTimeout:                      confutil.P("10s"), // Time before giving up on request-driven transaction state progress and re-pooling
	TargetActiveSequencers:            confutil.P(50),
	TransactionResumeMaxTransactions:  confutil.P(100000),
	TransactionResumePageSize:         confutil.P(1000),
	TransactionResumePollInterval:     confutil.P("5m"),
	Writer: FlushWriterConfig{
		WorkerCount:  confutil.P(10),
		BatchTimeout: confutil.P("25ms"),
		BatchMaxSize: confutil.P(100),
	},
}

var SequencerMinimum = SequencerMinimumConfig{
	AssembleErrorRetryThreshold:       0,
	BaseLedgerRevertRetryThreshold:    0,
	BlockHeightTolerance:              1,
	BlockRange:                        10,
	ClosingGracePeriod:                1,
	CoordinatorEventQueueSize:         1,
	CoordinatorPriorityEventQueueSize: 1,
	HeartbeatInterval:                 1 * time.Second,
	IdleSequencerCleanupInterval:      10 * time.Second,
	InactiveGracePeriod:               1,
	MaxDispatchAhead:                  1,
	MaxDispatchBatchSize:              1,
	MaxInflightTransactions:           1,
	OriginatorEventQueueSize:          1,
	OriginatorPriorityEventQueueSize:  1,
	RequestTimeout:                    1 * time.Second,
	SignErrorRetryThreshold:           0,
	StateTimeout:                      1 * time.Second,
	TargetActiveSequencers:            10,
	TransactionResumeMaxTransactions:  0,
	TransactionResumePageSize:         1,
	TransactionResumePollInterval:     10 * time.Second,
}
