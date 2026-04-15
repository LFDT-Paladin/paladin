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
	"sync"

	"github.com/google/uuid"
)

// ChainedChildStore is a thread-safe store that maps parent transaction IDs to
// their chained child transaction IDs. It has its own mutex, separate from any
// individual transaction lock, so the dispatch loop can read/write without
// risking deadlocks with the event loop's notification cascades.
type ChainedChildStore interface {
	SetChainedChild(parentID, childID uuid.UUID)
	GetChainedChild(parentID uuid.UUID) *uuid.UUID
	ForgetChainedChild(parentID uuid.UUID)
}

type chainedChildStore struct {
	mu       sync.RWMutex
	children map[uuid.UUID]uuid.UUID
}

func NewChainedChildStore() ChainedChildStore {
	return &chainedChildStore{children: make(map[uuid.UUID]uuid.UUID)}
}

func (s *chainedChildStore) SetChainedChild(parentID, childID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.children[parentID] = childID
}

func (s *chainedChildStore) GetChainedChild(parentID uuid.UUID) *uuid.UUID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id, ok := s.children[parentID]; ok {
		return &id
	}
	return nil
}

func (s *chainedChildStore) ForgetChainedChild(parentID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.children, parentID)
}
