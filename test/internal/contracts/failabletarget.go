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

package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed abis/RevertableTarget.json
var revertableTargetBuildJSON []byte

func LoadRevertableTargetContract() (*Contract, error) {
	var contractData Contract
	if err := json.Unmarshal(revertableTargetBuildJSON, &contractData); err != nil {
		return nil, fmt.Errorf("failed to parse embedded RevertableTarget.json: %w", err)
	}
	return &contractData, nil
}
