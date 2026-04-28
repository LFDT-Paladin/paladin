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
	StateTimeout                      *string           `json:"stateTimeout"`
	RequestTimeout                    *string           `json:"requestTimeout"`
	AssembleErrorRetryThreshold       *int              `json:"assembleErrorRetryThreshold"`
	BlockHeightTolerance              *uint64           `json:"blockHeightTolerance"`
	BlockRange                        *uint64           `json:"blockRange"`
	CoordinatorEventQueueSize         *int              `json:"coordinatorEventQueueSize"`
	CoordinatorPriorityEventQueueSize *int              `json:"coordinatorPriorityEventQueueSize"`
	OriginatorEventQueueSize          *int              `json:"originatorEventQueueSize"`
	OriginatorPriorityEventQueueSize  *int              `json:"originatorPriorityEventQueueSize"`
	ClosingGracePeriod                *int              `json:"closingGracePeriod"`
	ConfirmedLockRetentionGracePeriod *int              `json:"confirmedLockRetentionGracePeriod"`
	BaseLedgerRevertRetryThreshold    *int              `json:"baseLedgerRevertRetryThreshold"`
	HeartbeatInterval                 *string           `json:"heartbeatInterval"`
	HeartbeatGracePeriod              *int              `json:"heartbeatGracePeriod"`
	MaxInflightTransactions           *int              `json:"maxInflightTransactions"`
	MaxDispatchAhead                  *int              `json:"maxDispatchAhead"`
	RedelegateGracePeriod             *int              `json:"redelegateGracePeriod"`
	TargetActiveSequencers            *int              `json:"targetActiveSequencers"`
	TransactionResumePollInterval     *string           `json:"transactionResumePollInterval"`
	TransactionResumePageSize         *int              `json:"transactionResumePageSize"`
	TransactionResumeMaxTransactions  *int              `json:"transactionResumeMaxTransactions"`
	IdleSequencerCleanupInterval      *string           `json:"idleSequencerCleanupInterval"`
	Writer                            FlushWriterConfig `json:"writer"`
}

type SequencerMinimumConfig struct {
	StateTimeout                      time.Duration
	RequestTimeout                    time.Duration
	AssembleErrorRetryThreshold       int
	BlockHeightTolerance              uint64
	BlockRange                        uint64
	CoordinatorEventQueueSize         int
	CoordinatorPriorityEventQueueSize int
	OriginatorEventQueueSize          int
	OriginatorPriorityEventQueueSize  int
	ClosingGracePeriod                int
	ConfirmedLockRetentionGracePeriod int
	BaseLedgerRevertRetryThreshold    int
	HeartbeatInterval                 time.Duration
	HeartbeatGracePeriod              int
	MaxInflightTransactions           int
	MaxDispatchAhead                  int
	RedelegateGracePeriod             int
	TargetActiveSequencers            int
	TransactionResumePollInterval     time.Duration
	TransactionResumePageSize         int
	TransactionResumeMaxTransactions  int
	IdleSequencerCleanupInterval      time.Duration
}

var SequencerDefaults = SequencerConfig{
	Writer: FlushWriterConfig{
		WorkerCount:  confutil.P(10),
		BatchTimeout: confutil.P("25ms"),
		BatchMaxSize: confutil.P(100),
	},
	StateTimeout:                      confutil.P("10s"), // Time before giving up on request-driven transaction state progress and re-pooling
	RequestTimeout:                    confutil.P("3s"),  // Time before sending 1 retry of an assemble request, endorsement request etc
	AssembleErrorRetryThreshold:       confutil.P(3),
	BlockHeightTolerance:              confutil.P(uint64(5)),
	BlockRange:                        confutil.P(uint64(100)),
	CoordinatorEventQueueSize:         confutil.P(100),
	CoordinatorPriorityEventQueueSize: confutil.P(500),
	OriginatorEventQueueSize:          confutil.P(50),
	OriginatorPriorityEventQueueSize:  confutil.P(500),
	ClosingGracePeriod:                confutil.P(2),
	ConfirmedLockRetentionGracePeriod: confutil.P(2),
	BaseLedgerRevertRetryThreshold:    confutil.P(3),
	HeartbeatInterval:                 confutil.P("10s"),
	HeartbeatGracePeriod:              confutil.P(10),
	MaxInflightTransactions:           confutil.P(500),
	MaxDispatchAhead:                  confutil.P(50),
	RedelegateGracePeriod:             confutil.P(2),
	TargetActiveSequencers:            confutil.P(50),
	TransactionResumePollInterval:     confutil.P("5m"),
	TransactionResumePageSize:         confutil.P(1000),
	TransactionResumeMaxTransactions:  confutil.P(100000),
	IdleSequencerCleanupInterval:      confutil.P("1m"),
}

var SequencerMinimum = SequencerMinimumConfig{
	StateTimeout:                      1 * time.Second,
	RequestTimeout:                    1 * time.Second,
	AssembleErrorRetryThreshold:       0,
	BlockHeightTolerance:              1,
	BlockRange:                        10,
	CoordinatorEventQueueSize:         1,
	CoordinatorPriorityEventQueueSize: 1,
	OriginatorEventQueueSize:          1,
	OriginatorPriorityEventQueueSize:  1,
	ClosingGracePeriod:                1,
	ConfirmedLockRetentionGracePeriod: 0,
	BaseLedgerRevertRetryThreshold:    0,
	HeartbeatInterval:                 1 * time.Second,
	HeartbeatGracePeriod:              1,
	MaxInflightTransactions:           1,
	MaxDispatchAhead:                  1,
	RedelegateGracePeriod:             1,
	TargetActiveSequencers:            10,
	TransactionResumePollInterval:     10 * time.Second,
	TransactionResumePageSize:         1,
	TransactionResumeMaxTransactions:  0,
	IdleSequencerCleanupInterval:      10 * time.Second,
}
