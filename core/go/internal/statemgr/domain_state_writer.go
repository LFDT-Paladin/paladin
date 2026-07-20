// Copyright contributors to Paladin, an LFDT project
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

package statemgr

import (
	"context"
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// domainStateWriter implements components.DomainStateWriter.
// It is the sequencer's long-lived write buffer for a single private contract.
type domainStateWriter struct {
	ss                 *stateManager
	domainName         string
	customHashFunction bool
	contractAddress    pldtypes.EthAddress
	stateLock          sync.Mutex
	unFlushed          *pendingStateWrites
	flushing           *pendingStateWrites
}

// NewDomainStateWriter creates a coordinator-owned write buffer for a single contract.
func (ss *stateManager) NewDomainStateWriter(ctx context.Context, domain components.Domain, contractAddress pldtypes.EthAddress) components.DomainStateWriter {
	log.L(ctx).Debugf("Domain state writer for domain %s contract %s created", domain.Name(), contractAddress)

	return &domainStateWriter{
		ss:                 ss,
		domainName:         domain.Name(),
		customHashFunction: domain.CustomHashFunction(),
		contractAddress:    contractAddress,
	}
}

// MUST hold the stateLock to call this function.
// Checks there is no un-cleared flush error, and inits unFlushed if nil.
func (sw *domainStateWriter) checkResetInitUnFlushed(ctx context.Context) error {
	if sw.flushing != nil {
		select {
		case <-sw.flushing.flushed:
			if sw.flushing.flushResult != nil {
				log.L(ctx).Errorf("flush failed - domain state writer must be reset")
				return i18n.WrapError(ctx, sw.flushing.flushResult, msgs.MsgStateFlushFailedDomainReset, sw.domainName, sw.contractAddress)
			}
		default:
		}
	}
	if sw.unFlushed == nil {
		sw.unFlushed = newPendingStateWrites(sw.ss)
	}
	return nil
}

func (sw *domainStateWriter) upsertStates(ctx context.Context, dbTX persistence.DBTX, holdingLock bool, stateUpserts ...*components.StateUpsert) ([]*pldapi.State, error) {
	vss, err := sw.ss.validateStateUpserts(ctx, sw.domainName, sw.contractAddress, sw.customHashFunction, dbTX, stateUpserts...)
	if err != nil {
		return nil, err
	}

	if !holdingLock {
		sw.stateLock.Lock()
		defer sw.stateLock.Unlock()
	}
	if flushErr := sw.checkResetInitUnFlushed(ctx); flushErr != nil {
		return nil, flushErr
	}

	sw.unFlushed.states = append(sw.unFlushed.states, vss.withValues...)
	return vss.states, nil
}

// StageStateUpserts creates or updates states in the in-memory write buffer.
func (sw *domainStateWriter) StageStateUpserts(ctx context.Context, dbTX persistence.DBTX, stateUpserts ...*components.StateUpsert) (states []*pldapi.State, err error) {
	return sw.upsertStates(ctx, dbTX, false, stateUpserts...)
}

// StageNullifierUpserts creates nullifier records to be written on the next flush.
// The state being nullified must already be staged in this writer's unFlushed or flushing buffer.
func (sw *domainStateWriter) StageNullifierUpserts(ctx context.Context, nullifiers ...*components.NullifierUpsert) error {
	sw.stateLock.Lock()
	defer sw.stateLock.Unlock()
	if flushErr := sw.checkResetInitUnFlushed(ctx); flushErr != nil {
		return flushErr
	}

	for _, nullifierInput := range nullifiers {
		nullifier := &pldapi.StateNullifier{
			DomainName: sw.domainName,
			ID:         nullifierInput.ID,
			State:      nullifierInput.State,
		}
		// Locate the state in the pending write buffers to validate it exists and detect conflicts.
		var creatingState *components.StateWithLabels
		for _, s := range sw.unFlushed.states {
			if s.ID.Equals(nullifier.State) {
				creatingState = s
				break
			}
		}
		if creatingState == nil && sw.flushing != nil {
			for _, s := range sw.flushing.states {
				if s.ID.Equals(nullifier.State) {
					creatingState = s
					break
				}
			}
		}
		if creatingState == nil {
			return i18n.NewError(ctx, msgs.MsgStateNullifierStateNotInCtx, nullifier.State, nullifier.ID)
		} else if creatingState.Nullifier != nil && !creatingState.Nullifier.ID.Equals(nullifier.ID) {
			return i18n.NewError(ctx, msgs.MsgStateNullifierConflict, nullifier.State, creatingState.Nullifier.ID)
		}
		creatingState.Nullifier = nullifier
		sw.unFlushed.stateNullifiers = append(sw.unFlushed.stateNullifiers, nullifier)
	}

	return nil
}

func (sw *domainStateWriter) finalizer(ctx context.Context, commitError error) {
	sw.stateLock.Lock()
	defer sw.stateLock.Unlock()
	if sw.flushing != nil && commitError != nil {
		sw.flushing.setError(commitError)
	} else {
		sw.flushing = nil
	}
}

// Reset puts the world back to fresh.
//
// Must be called after a flush error before the writer can be used, as on a flush
// error the caller must reset their processing to the last point of consistency
// as they cannot trust in-memory state
//
// Note it does not cancel or check the status of any in-progress flush, as the
// things that are flushed are insert records in isolation.
// Reset instead is intended to be a boundary where the calling code knows explicitly
// that any states that haven't reached a confirmed flush must be re-written into the
// DomainStateWriter
func (sw *domainStateWriter) Reset() {
	sw.stateLock.Lock()
	defer sw.stateLock.Unlock()

	sw.flushing = nil
	sw.unFlushed = nil
}

// Flush moves the un-flushed set into flushing status and queues a batch DB write.
func (sw *domainStateWriter) Flush(ctx context.Context, dbTX persistence.DBTX) error {
	log.L(ctx).Infof("Flushing domain state writer domain=%s", sw.domainName)

	sw.stateLock.Lock()
	defer sw.stateLock.Unlock()

	if sw.flushing != nil {
		if sw.flushing.flushResult != nil {
			return sw.flushing.flushResult
		}
		return i18n.NewError(ctx, msgs.MsgStateFlushInProgress)
	}

	sw.flushing = sw.unFlushed
	sw.unFlushed = nil

	if sw.flushing == nil {
		log.L(ctx).Debugf("nothing pending to flush in domain state writer")
		return nil
	}

	var syncFlushError error
	defer func() {
		if syncFlushError != nil {
			sw.flushing.setError(syncFlushError)
		}
	}()
	syncFlushError = sw.flushing.exec(ctx, dbTX)
	if syncFlushError != nil {
		return syncFlushError
	}

	dbTX.AddFinalizer(sw.finalizer)
	return nil
}
