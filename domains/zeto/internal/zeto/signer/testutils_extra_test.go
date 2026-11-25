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

package signer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTestWitnessCalculator_Methods(t *testing.T) {
	calc := &testWitnessCalculator{}
	_, err := calc.CalculateWitness(map[string]interface{}{"x": 1}, true)
	require.NoError(t, err)
	_, err = calc.CalculateBinWitness(map[string]interface{}{"x": 1}, true)
	require.NoError(t, err)
	_, err = calc.CalculateWTNSBin(map[string]interface{}{"x": 1}, false)
	require.NoError(t, err)
}
