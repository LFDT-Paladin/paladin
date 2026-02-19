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
package transaction

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

func (t *CoordinatorTransaction) scheduleRequestTimeout(ctx context.Context) {
	t.clearRequestTimeoutSchedule()
	t.cancelRequestTimeoutSchedule = t.clock.ScheduleTimer(ctx, t.requestTimeout, func() {
		t.queueEventForCoordinator(ctx, &RequestTimeoutIntervalEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: t.pt.ID,
			},
		})
	})
}

func (t *CoordinatorTransaction) scheduleStateTimeout(ctx context.Context) {
	t.clearStateTimeoutSchedule()
	t.cancelStateTimeoutSchedule = t.clock.ScheduleTimer(ctx, t.stateTimeout, func() {
		t.queueEventForCoordinator(ctx, &RequestTimeoutIntervalEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: t.pt.ID,
			},
		})
	})
}

func (t *CoordinatorTransaction) clearRequestTimeoutSchedule() {
	if t.cancelRequestTimeoutSchedule != nil {
		t.cancelRequestTimeoutSchedule()
		t.cancelRequestTimeoutSchedule = nil
	}
}

func (t *CoordinatorTransaction) clearStateTimeoutSchedule() {
	if t.cancelStateTimeoutSchedule != nil {
		t.cancelStateTimeoutSchedule()
		t.cancelStateTimeoutSchedule = nil
	}
}

func (t *CoordinatorTransaction) clearTimeoutSchedules() {
	t.clearRequestTimeoutSchedule()
	t.clearStateTimeoutSchedule()
}

func (t *CoordinatorTransaction) stateTimeoutExceeded(ctx context.Context, pendingRequest *common.IdempotentRequest, stateDescription string) bool {
	if pendingRequest == nil {
		log.L(ctx).Warnf("stateTimeoutExceeded called for %s on transaction %s with no pending request", stateDescription, t.pt.ID)
		return false
	}
	log.L(ctx).Debugf("checking state timeout exceeded for %s on transaction %s request idempotency key %s", stateDescription, t.pt.ID.String(), pendingRequest.IdempotencyKey())
	startTime := t.stateEntryTime
	if startTime == nil {
		log.L(ctx).Warnf("stateTimeoutExceeded called for %s on transaction %s with no start time", stateDescription, t.pt.ID)
		return false
	}
	timedOut := t.clock.HasExpired(startTime, t.stateTimeout)
	if timedOut {
		log.L(ctx).Debugf("%s of TX %s timed out", stateDescription, t.pt.ID)
	}
	return timedOut
}
