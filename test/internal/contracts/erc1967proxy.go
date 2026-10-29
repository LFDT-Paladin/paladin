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

package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed abis/ERC1967Proxy.json
var erc1967ProxyBuildJSON []byte

func LoadERC1967ProxyContract() (*Contract, error) {
	var contractData Contract
	if err := json.Unmarshal(erc1967ProxyBuildJSON, &contractData); err != nil {
		return nil, fmt.Errorf("failed to parse embedded ERC1967Proxy.json: %w", err)
	}
	return &contractData, nil
}
