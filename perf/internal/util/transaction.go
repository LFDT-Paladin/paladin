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

package util

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
)

// WaitForTransactionReceipt polls for a transaction receipt until it is available, the context is cancelled, or the timeout is reached.
func WaitForTransactionReceipt(ctx context.Context, client pldclient.PaladinClient, txID uuid.UUID, timeout time.Duration) (*pldapi.TransactionReceipt, error) {
	deadline := time.Now().Add(timeout)
	checkInterval := 1 * time.Second

	for time.Now().Before(deadline) {
		receipt, err := client.PTX().GetTransactionReceipt(ctx, txID)
		if err == nil && receipt != nil {
			return receipt, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(checkInterval):
			// Continue polling
		}
	}

	return nil, fmt.Errorf("timeout waiting for transaction receipt: %s", txID)
}

// WaitForTransactionReceiptFull polls for a full transaction receipt until it is available, the context is cancelled, or the timeout is reached.
func WaitForTransactionReceiptFull(ctx context.Context, client pldclient.PaladinClient, txID uuid.UUID, timeout time.Duration) (*pldapi.TransactionReceiptFull, error) {
	deadline := time.Now().Add(timeout)
	checkInterval := 1 * time.Second

	for time.Now().Before(deadline) {
		receipt, err := client.PTX().GetTransactionReceiptFull(ctx, txID)
		if err == nil && receipt != nil {
			return receipt, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(checkInterval):
			// Continue polling
		}
	}

	return nil, fmt.Errorf("timeout waiting for transaction receipt: %s", txID)
}

// GetIdempotencyKey returns a unique key for a worker action, used to avoid duplicate transaction submission.
func GetIdempotencyKey(startTime int64, workerID, iteration int) string {
	workerIDStr := fmt.Sprintf("%05d", workerID)
	iterationIDStr := fmt.Sprintf("%09d", iteration)
	return fmt.Sprintf("%v-%s-%s-%s", startTime, workerIDStr, iterationIDStr, uuid.New())
}
