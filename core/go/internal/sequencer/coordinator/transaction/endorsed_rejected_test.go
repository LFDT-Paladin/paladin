package transaction

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Verifies that when an endorsement is rejected, the transaction resets pending endorsement requests
// and transitions back to the pooled state so it can be re-assembled immediately.
func TestEndorsedRejected_ImmediateRepool(t *testing.T) {
    // Build a transaction in the EndorsementGathering state with one pending endorsement request
    builder := NewTransactionBuilderForTesting(t, State_Endorsement_Gathering).
        UseMockTransportWriter().
        UseMockClock().
        NumberOfRequiredEndorsers(1).
        AddPendingEndorsementRequest(0)

    txn, _ := builder.Build()

    // Sanity: there should be pending endorsement requests
    require.NotNil(t, txn.pendingEndorsementRequests)

    // Build and queue an EndorsedRejectedEvent for the pending request
    event := builder.BuildEndorseRejectedEvent(0)

    err := txn.HandleEvent(context.Background(), event)
    require.NoError(t, err)

    // After handling rejection, the state should be pooled and pending requests cleared
    require.Equal(t, State_Pooled, txn.GetCurrentState())
    require.Nil(t, txn.pendingEndorsementRequests)
}
