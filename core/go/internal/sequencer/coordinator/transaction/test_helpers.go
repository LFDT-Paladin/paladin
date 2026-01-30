/*
 * Copyright © 2024 Kaleido, Inc.
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
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/google/uuid"
)

// NewMinimalCoordinatorTransactionForTest returns a minimal Transaction with only ID and originatorNode set,
// for use in handler tests that need a coordinator transaction (e.g. sequencer HandleNonceAssigned).
func NewMinimalCoordinatorTransactionForTest(originatorNode string, id uuid.UUID) *Transaction {
	return &Transaction{
		PrivateTransaction: &components.PrivateTransaction{ID: id},
		originatorNode:     originatorNode,
	}
}
