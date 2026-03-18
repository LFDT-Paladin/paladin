// Copyright © 2026 Kaleido, Inc.
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

package pldapi

import (
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

type Dispatch struct {
	ID                       string              `docstruct:"Dispatch" json:"id"`
	PrivateTransactionID     string              `docstruct:"Dispatch" json:"privateTransactionID"`
	PublicTransactionAddress pldtypes.EthAddress `docstruct:"Dispatch" json:"publicTransactionAddress"`
	PublicTransactionID      uint64              `docstruct:"Dispatch" json:"publicTransactionID"`
}
