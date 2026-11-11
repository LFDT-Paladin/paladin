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

package fungible

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/require"
)

func TestMakeNewInfoState(t *testing.T) {
	ctx := context.Background()
	info := &types.TransactionData{
		Salt: pldtypes.MustParseHexUint256("0x01"),
		Data: pldtypes.HexBytes{0xab},
	}
	ns, err := makeNewInfoState(ctx, &prototk.StateSchema{Id: "data"}, info, []string{"sender@node"})
	require.NoError(t, err)
	require.NotNil(t, ns.Id)
	require.Equal(t, "data", ns.SchemaId)
}

func TestPrepareTransactionInfoStates(t *testing.T) {
	ctx := context.Background()
	states, err := prepareTransactionInfoStates(ctx, pldtypes.HexBytes{0x01}, []string{"sender@node"}, &prototk.StateSchema{Id: "data"})
	require.NoError(t, err)
	require.Len(t, states, 1)
	require.NotNil(t, states[0].Id)
}
