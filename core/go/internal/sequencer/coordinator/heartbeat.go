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

package coordinator

import (
	"context"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

type heatbeatLoop struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	contractAddress string
	interval        time.Duration
	queueEvent      func(ctx context.Context, event common.Event)
}

func newHeartbeatLoop(contractAddress string, interval time.Duration, queueEvent func(ctx context.Context, event common.Event)) *heatbeatLoop {
	return &heatbeatLoop{
		contractAddress: contractAddress,
		interval:        interval,
		queueEvent:      queueEvent,
	}
}

func (h *heatbeatLoop) start(ctx context.Context) {
	if h.ctx == nil {
		h.ctx, h.cancelCtx = context.WithCancel(ctx)
		go h.run()
	}
}

func (h *heatbeatLoop) stop() {
	if h.cancelCtx != nil {
		h.cancelCtx()
	}
}

func (h *heatbeatLoop) run() {
	log.L(log.WithLogField(h.ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf("coord    | %s   | Starting heartbeat loop", h.contractAddress[0:8])

	// Send an initial heartbeat interval event to be handled immediately
	h.queueEvent(h.ctx, &common.HeartbeatIntervalEvent{})

	// Then every N seconds
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			h.queueEvent(h.ctx, &common.HeartbeatIntervalEvent{})
		case <-h.ctx.Done():
			log.L(h.ctx).Infof("Ending heartbeat loop for %s", h.contractAddress)
			h.ctx = nil
			h.cancelCtx = nil
			return
		}
	}
}
