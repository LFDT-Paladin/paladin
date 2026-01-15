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

package smt

import (
	pldsmt "github.com/LFDT-Paladin/paladin/toolkit/pkg/smt"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/core"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/smt"
	utxocore "github.com/LFDT-Paladin/smt/pkg/utxo/core"
)

const SMT_HEIGHT_UTXO = 64

func NewSmt(storage pldsmt.StatesStorage, levels int) (core.SparseMerkleTree, error) {
	mt, err := smt.NewMerkleTree(storage, levels)
	return mt, err
}

func MerkleTreeName(domainInstanceContract string) string {
	return "smt_noto_" + domainInstanceContract
}

func GetHasher() utxocore.Hasher {
	return &pldsmt.Keccak256Hasher{}
}
