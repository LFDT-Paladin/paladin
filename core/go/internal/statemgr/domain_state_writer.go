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
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
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

func (sw *domainStateWriter) ResolveStates(ctx context.Context, dbTX persistence.DBTX, states ...*prototk.EndorsableState) ([]*components.StateWithLabels, error) {
	return sw.ss.validateStates(ctx, sw.domainName, sw.contractAddress, sw.customHashFunction, dbTX, states...)
}

// StageWrites validates the nullifiers against the supplied states and, only once the whole batch is
// consistent, appends the states and their nullifiers to the unFlushed buffer under a single lock.
// Each nullified state must be present in the states passed to this call, which holds because nullifiers
// only ever target the same transaction's own output/info states.
func (sw *domainStateWriter) StageWrites(ctx context.Context, states []*components.StateWithLabels, nullifiers ...*components.NullifierUpsert) error {
	stateNullifiers := make([]*pldapi.StateNullifier, 0, len(nullifiers))
	for _, nullifierInput := range nullifiers {
		nullifier := &pldapi.StateNullifier{
			DomainName: sw.domainName,
			ID:         nullifierInput.ID,
			State:      nullifierInput.State,
		}
		var creatingState *components.StateWithLabels
		for _, s := range states {
			if s.ID.Equals(nullifier.State) {
				creatingState = s
				break
			}
		}
		if creatingState == nil {
			return i18n.NewError(ctx, msgs.MsgStateNullifierStateNotInCtx, nullifier.State, nullifier.ID)
		} else if creatingState.Nullifier != nil && !creatingState.Nullifier.ID.Equals(nullifier.ID) {
			return i18n.NewError(ctx, msgs.MsgStateNullifierConflict, nullifier.State, creatingState.Nullifier.ID)
		}
		creatingState.Nullifier = nullifier
		stateNullifiers = append(stateNullifiers, nullifier)
	}

	sw.stateLock.Lock()
	defer sw.stateLock.Unlock()
	if flushErr := sw.checkResetInitUnFlushed(ctx); flushErr != nil {
		return flushErr
	}
	sw.unFlushed.states = append(sw.unFlushed.states, states...)
	sw.unFlushed.stateNullifiers = append(sw.unFlushed.stateNullifiers, stateNullifiers...)
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

	flushing, err := sw.rotateForFlush(ctx)
	if err != nil || flushing == nil {
		return err
	}

	// The DB write runs lock-free. rotateForFlush already swapped this buffer into flushing
	// status under the lock, after which it is stable for exec's reads, so the coordinator's
	// StageWrites append no longer waits behind DB I/O.
	if syncFlushError := flushing.exec(ctx, dbTX); syncFlushError != nil {
		sw.stateLock.Lock()
		flushing.setError(syncFlushError)
		sw.stateLock.Unlock()
		return syncFlushError
	}

	dbTX.AddFinalizer(sw.finalizer)
	return nil
}

// rotateForFlush runs the single-flush guard and the buffer swap under stateLock, returning
// the buffer to flush. The returned buffer is safe to write to the DB lock-free. A nil buffer
// with nil error means there was nothing pending to flush.
func (sw *domainStateWriter) rotateForFlush(ctx context.Context) (*pendingStateWrites, error) {
	sw.stateLock.Lock()
	defer sw.stateLock.Unlock()

	if sw.flushing != nil {
		if sw.flushing.flushResult != nil {
			return nil, sw.flushing.flushResult
		}
		return nil, i18n.NewError(ctx, msgs.MsgStateFlushInProgress)
	}

	sw.flushing = sw.unFlushed
	sw.unFlushed = nil

	if sw.flushing == nil {
		log.L(ctx).Debugf("nothing pending to flush in domain state writer")
		return nil, nil
	}

	return sw.flushing, nil
}
