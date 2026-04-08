package noto

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildReceiptIncludesTransactionRequester(t *testing.T) {
	ctx := context.Background()

	n := &Noto{}
	// Minimal required schemas
	n.coinSchema = &prototk.StateSchema{Id: "coin-schema"}
	n.lockedCoinSchema = &prototk.StateSchema{Id: "locked-coin-schema"}
	n.dataSchema = &prototk.StateSchema{Id: "data-schema"}
	n.lockInfoSchema = &prototk.StateSchema{Id: "lock-info-schema"}

	// Prepare an info state with requester details
	requesterLookup := "requester@node"
	requesterAddr := pldtypes.MustEthAddress("0x0000000000000000000000000000000000000001")
	requesterType := "test-type"
	infoStates, err := n.prepareInfo(pldtypes.HexBytes{0x01, 0x02}, []string{"a"}, requesterLookup, requesterAddr, requesterType)
	require.NoError(t, err)
	require.Len(t, infoStates, 1)

	// Build a fake BuildReceiptRequest that includes the info state
	endorsable := &prototk.EndorsableState{
		SchemaId:      n.dataSchema.Id,
		StateDataJson: infoStates[0].StateDataJson,
	}

	req := &prototk.BuildReceiptRequest{
		InputStates:  []*prototk.EndorsableState{},
		OutputStates: []*prototk.EndorsableState{},
		ReadStates:   []*prototk.EndorsableState{},
		InfoStates:   []*prototk.EndorsableState{endorsable},
	}

	res, err := n.BuildReceipt(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)

	var receipt types.NotoDomainReceipt
	err = json.Unmarshal([]byte(res.ReceiptJson), &receipt)
	require.NoError(t, err)

	require.NotNil(t, receipt.TransactionRequester)
	assert.Equal(t, requesterLookup, receipt.TransactionRequester.Lookup)
	if receipt.TransactionRequester.ResolvedAddress != nil {
		assert.Equal(t, requesterAddr.String(), receipt.TransactionRequester.ResolvedAddress.String())
	}
	assert.Equal(t, requesterType, receipt.TransactionRequester.Type)
}
