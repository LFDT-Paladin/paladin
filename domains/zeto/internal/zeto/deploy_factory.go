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

package zeto

import (
	"fmt"

	"github.com/hyperledger/firefly-signer/pkg/abi"
)

// pickZetoFactoryDeploy7Arg selects deploy(bytes32,string,string,string,address,bytes,bool) from a factory ABI.
func pickZetoFactoryDeploy7Arg(a abi.ABI) (*abi.Entry, error) {
	var found *abi.Entry
	for i := range a {
		e := a[i]
		if e.Type == abi.Function && e.Name == "deploy" && len(e.Inputs) == 7 {
			if found != nil {
				return nil, fmt.Errorf("multiple 7-argument deploy overloads in factory ABI")
			}
			entry := *e
			found = &entry
		}
	}
	if found == nil {
		return nil, fmt.Errorf("7-argument deploy(bytes32,string,string,string,address,bytes,bool) not found in factory ABI")
	}
	return found, nil
}
