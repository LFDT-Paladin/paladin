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

package noto

import (
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndorsableStateIDs(t *testing.T) {
	owner1 := pldtypes.MustEthAddress("0xbb2b99dde4ca2d4c99f149d13cd55a9edada69eb")
	inputStates := []*prototk.EndorsableState{
		{
			Id:       "1",
			SchemaId: "coin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner1),
		},
	}

	ids := endorsableStateIDs(inputStates, true)
	require.Len(t, ids, 1)
	assert.Equal(t, "ada5013122d395ba3c54772283fb069b10426056ef8ca54750cb9bb552a59e7d", ids[0])
}
