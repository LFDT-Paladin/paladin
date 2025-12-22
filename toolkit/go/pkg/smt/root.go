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

package smt

import (
	"crypto/sha256"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

type MerkleTreeRoot struct {
	SmtName   string           `json:"smtName"`
	RootIndex pldtypes.Bytes32 `json:"rootIndex"`
}

func (m *MerkleTreeRoot) Hash() (string, error) {
	h := sha256.New()
	h.Write([]byte(m.SmtName))
	h.Write(m.RootIndex.Bytes())
	return pldtypes.Bytes32(h.Sum(nil)).HexString(), nil
}
