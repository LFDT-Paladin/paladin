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
		t.queueEventForCoordinator(ctx, &StateTimeoutIntervalEvent{
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

func action_ScheduleStateTimeout(ctx context.Context, txn *CoordinatorTransaction, _ common.Event) error {
	txn.scheduleStateTimeout(ctx)
	return nil
}
