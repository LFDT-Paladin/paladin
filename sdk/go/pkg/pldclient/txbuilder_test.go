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

package pldclient

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testABIJSON = ([]byte)(`[
	{
		"type": "constructor",
		"inputs": [
			{
				"name": "supplier",
				"type": "address"
			}
		]
	},
	{
		"name": "newWidget",
		"type": "function",
		"inputs": [
			{
				"name": "widget",
				"type": "tuple",
				"components": [
					{
						"name": "id",
						"type": "address"
					},
					{
						"name": "sku",
						"type": "uint256"
					},
					{
						"name": "features",
						"type": "string[]"
					}
				]
			}
		],
		"outputs": []
	},
	{
		"name": "getWidgets",
		"type": "function",
		"inputs": [
			{
				"name": "sku",
				"type": "uint256"
			}
		],
		"outputs": [
			{
				"name": "",
				"type": "tuple[]",
				"components": [
					{
						"name": "id",
						"type": "address"
					},
					{
						"name": "sku",
						"type": "uint256"
					},
					{
						"name": "features",
						"type": "string[]"
					}
				]
			}
		]
	},
	{
	  "type": "error",
	  "name": "WidgetError",
	  "inputs": [
	    {
	      "name": "sku",
	      "type": "uint256"
	    },
	    {
	      "name": "issue",
	      "type": "string"
	    }
	  ]
	}
]`)

var testABI = New().TxBuilder(context.Background()).ABIJSON(testABIJSON).GetABI()

func getTransaction(t *testing.T, txID uuid.UUID) testRPCMethod {
	return testRPCMethod{
		name: "ptx_getTransaction",
		handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
			var suppliedID uuid.UUID
			if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
				return errorResponse(rpcReq.ID, err)
			}
			require.Equal(t, txID, suppliedID)
			return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
				"id": "`+txID.String()+`"
			}`))
		},
	}
}

func TestBuildAndSubmitPublicTXHTTPOk(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()
	txHash := pldtypes.RandBytes32()

	methods := []testRPCMethod{
		{
			name: "ptx_sendTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var tx pldapi.TransactionInput
				err := json.Unmarshal(rpcReq.Params[0], &tx)
				if err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.JSONEq(t, `{
					"widget": {
						"id": "0x172ea50b3535721154ae5b368e850825615882bb",
						"sku": "12345",
						"features": ["blue", "round"]
					}
				}`, string(tx.Data))
				require.Equal(t, pldapi.TransactionTypePublic, tx.Type.V())
				require.Equal(t, "newWidget", tx.Function)
				require.Equal(t, contractAddr, tx.To)
				require.Equal(t, "tx.sender", tx.From)
				require.Equal(t, pldtypes.HexUint64(100000), *tx.PublicTxOptions.Gas)
				return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
			},
		},
		getTransaction(t, txID),
		{
			name: "ptx_getTransactionReceipt",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID uuid.UUID
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, txID, suppliedID)
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
					"id": "`+txID.String()+`",
					"transactionHash": "`+txHash.String()+`",
					"success": true
				}`))
			},
		},
	}
	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	sent := c.ForABI(ctx, testABI).
		Public().
		Function("newWidget").
		Inputs(map[string]any{
			"widget": map[string]any{
				"id":       "0x172EA50B3535721154ae5B368E850825615882BB",
				"sku":      12345,
				"features": []string{"blue", "round"},
			},
		}).
		From("tx.sender").
		To(contractAddr).
		PublicTxOptions(pldapi.PublicTxOptions{
			Gas: confutil.P(pldtypes.HexUint64(100000)),
		}).
		Send()

	res := sent.Wait(100 * time.Millisecond)
	require.NoError(t, res.Error())
	require.Equal(t, txHash, *res.TransactionHash())
	require.Equal(t, txHash, *res.Receipt().TransactionHash)
	require.Equal(t, txID, res.ID())

	// Check directly getting TX and receipt
	tx, err := sent.GetTransaction()
	require.NoError(t, err)
	require.Equal(t, txID, *tx.ID)

	// Check directly getting TX and receipt
	receipt, err := sent.GetReceipt()
	require.NoError(t, err)
	require.Equal(t, txID, receipt.ID)

}

func TestBuildAndSubmitPrivateTXHTTPRevert(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	methods := []testRPCMethod{
		{
			name: "ptx_sendTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var tx pldapi.TransactionInput
				err := json.Unmarshal(rpcReq.Params[0], &tx)
				if err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.JSONEq(t, `{
				"widget": {
					"id": "0x172ea50b3535721154ae5b368e850825615882bb",
					"sku": "12345",
					"features": ["blue", "round"]
				}
			}`, string(tx.Data))
				require.Equal(t, pldapi.TransactionTypePrivate, tx.Type.V())
				require.Equal(t, "neeto", tx.Domain)
				require.Equal(t, "newWidget", tx.Function)
				require.Equal(t, contractAddr, tx.To)
				require.Equal(t, "tx.sender", tx.From)
				require.Equal(t, pldtypes.HexUint64(100000), *tx.PublicTxOptions.Gas)
				return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
			},
		},
		getTransaction(t, txID),
		{
			name: "ptx_getTransactionReceipt",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID uuid.UUID
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, txID, suppliedID)
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
				"id": "`+txID.String()+`",
				"success": false,
				"failureMessage": "something went wrong"
			}`))
			},
		}}

	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	sent := c.ForABI(ctx, testABI).
		Private().Domain("neeto").
		Function("newWidget").
		Inputs(map[string]any{
			"widget": map[string]any{
				"id":       "0x172EA50B3535721154ae5B368E850825615882BB",
				"sku":      12345,
				"features": []string{"blue", "round"},
			},
		}).
		From("tx.sender").
		To(contractAddr).
		PublicTxOptions(pldapi.PublicTxOptions{
			Gas: confutil.P(pldtypes.HexUint64(100000)),
		}).
		Send()

	res := sent.Wait(100 * time.Millisecond)
	assert.EqualError(t, res.Error(), "something went wrong")
	assert.Nil(t, res.TransactionHash())
}

func TestBuildAndPreparePrivateTXHTTPOk(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	methods := []testRPCMethod{
		{
			name: "ptx_prepareTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var tx pldapi.TransactionInput
				err := json.Unmarshal(rpcReq.Params[0], &tx)
				if err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.JSONEq(t, `{
					"widget": {
						"id": "0x172ea50b3535721154ae5b368e850825615882bb",
						"sku": "12345",
						"features": ["blue", "round"]
					}
				}`, string(tx.Data))
				require.Equal(t, pldapi.TransactionTypePrivate, tx.Type.V())
				require.Equal(t, "neeto", tx.Domain)
				require.Equal(t, "newWidget", tx.Function)
				require.Equal(t, contractAddr, tx.To)
				require.Equal(t, "tx.sender", tx.From)
				require.Equal(t, pldtypes.HexUint64(100000), *tx.PublicTxOptions.Gas)
				return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
			},
		},
		getTransaction(t, txID),
		{
			name: "ptx_getPreparedTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID uuid.UUID
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, txID, suppliedID)
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
					"id": "`+txID.String()+`",
					"transaction": {
						"idempotencyKey": "tx1"
					}
				}`))
			},
		},
	}

	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	prepare := c.ForABI(ctx, testABI).
		Private().Domain("neeto").
		Function("newWidget").
		Inputs(map[string]any{
			"widget": map[string]any{
				"id":       "0x172EA50B3535721154ae5B368E850825615882BB",
				"sku":      12345,
				"features": []string{"blue", "round"},
			},
		}).
		From("tx.sender").
		To(contractAddr).
		PublicTxOptions(pldapi.PublicTxOptions{
			Gas: confutil.P(pldtypes.HexUint64(100000)),
		}).
		Prepare()

	res := prepare.Wait(100 * time.Millisecond)
	require.NoError(t, res.Error())
	assert.Equal(t, txID, *prepare.ID())
	assert.Equal(t, txID, res.ID())
	assert.Equal(t, "tx1", res.PreparedTransaction().Transaction.IdempotencyKey)

	// Check directly getting TX and receipt
	tx, err := prepare.GetTransaction()
	require.NoError(t, err)
	require.Equal(t, txID, *tx.ID)

	// Check directly getting TX and receipt
	prepared, err := prepare.GetPreparedTransaction()
	require.NoError(t, err)
	require.Equal(t, txID, prepared.ID)

}

func TestBuildAndSubmitPublicCallHTTPOk(t *testing.T) {
	contractAddr := pldtypes.RandAddress()

	ctx, c, done := newTestClientAndServerHTTP(t, testRPCMethod{
		name: "ptx_call",
		handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
			var tx pldapi.TransactionCall
			err := json.Unmarshal(rpcReq.Params[0], &tx)
			if err != nil {
				return errorResponse(rpcReq.ID, err)
			}
			require.JSONEq(t, `["12345"]`, string(tx.Data))
			require.Equal(t, pldapi.TransactionTypePublic, tx.Type.V())
			require.Equal(t, "getWidgets", tx.Function)
			require.Equal(t, contractAddr, tx.To)
			require.Equal(t, "latest", tx.Block.String())
			return successResponse(rpcReq.ID, pldtypes.RawJSON(`[[
				"0x172ea50b3535721154ae5b368e850825615882bb",
				"12345",
				["blue", "round"]
			]]`))
		},
	})
	defer done()

	var widgets []any
	called := c.ForABI(ctx, testABI).
		Public().
		Function("getWidgets").
		To(contractAddr).
		Inputs([]int{12345}).
		Outputs(&widgets).
		DataFormat("mode=array").
		PublicCallOptions(pldapi.PublicCallOptions{
			Block: "latest",
		}).
		Call()
	require.NoError(t, called)
	assert.Equal(t, []any{
		[]any{
			"0x172ea50b3535721154ae5b368e850825615882bb",
			"12345",
			[]any{"blue", "round"},
		},
	}, widgets)

}

func TestBuildAndSubmitPublicDeployWSFail(t *testing.T) {
	bytecode := pldtypes.HexBytes(pldtypes.RandBytes(64))
	txID := uuid.New()

	methods := []testRPCMethod{
		{
			name: "ptx_sendTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var tx pldapi.TransactionInput
				err := json.Unmarshal(rpcReq.Params[0], &tx)
				if err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.JSONEq(t, `{"supplier": "0x172ea50b3535721154ae5b368e850825615882bb"}`, string(tx.Data))
				require.Equal(t, bytecode, tx.Bytecode)
				return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
			},
		}, {
			name: "ptx_getTransactionReceipt",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID uuid.UUID
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, txID, suppliedID)
				return errorResponse(rpcReq.ID, fmt.Errorf("server throws an error"))
			},
		},
	}

	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	cancellable, cancelCtx := context.WithCancel(ctx)

	sent := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(cancellable).
		Public().
		SolidityBuild(&solutils.SolidityBuild{
			ABI:      testABI,
			Bytecode: bytecode,
		}).
		From("tx.sender").
		Inputs(`{"supplier": "0x172EA50B3535721154ae5B368E850825615882BB"}`).
		Send()

	res := sent.Wait(25 * time.Millisecond)
	require.Regexp(t, "PD020216.*timed out.*server throws an error", res.Error())
	require.Nil(t, res.TransactionHash())

	cancelCtx()
	res = sent.Wait(1 * time.Minute)
	require.Regexp(t, "PD020000", res.Error())
	require.Nil(t, res.TransactionHash())

}

func TestBuildAndPreparePrivateHTTPFail(t *testing.T) {
	bytecode := pldtypes.HexBytes(pldtypes.RandBytes(64))
	txID := uuid.New()
	methods := []testRPCMethod{
		{
			name: "ptx_prepareTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var tx pldapi.TransactionInput
				err := json.Unmarshal(rpcReq.Params[0], &tx)
				if err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.JSONEq(t, `{"supplier": "0x172ea50b3535721154ae5b368e850825615882bb"}`, string(tx.Data))
				require.Equal(t, bytecode, tx.Bytecode)
				return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
			},
		},
		{
			name: "ptx_getPreparedTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID uuid.UUID
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, txID, suppliedID)
				return errorResponse(rpcReq.ID, fmt.Errorf("server throws an error"))
			},
		},
	}
	ctx, c, done := newTestClientAndServerWebSockets(t, methods...)
	defer done()

	cancellable, cancelCtx := context.WithCancel(ctx)

	sent := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(cancellable).
		Public().
		SolidityBuild(&solutils.SolidityBuild{
			ABI:      testABI,
			Bytecode: bytecode,
		}).
		From("tx.sender").
		Inputs(`{"supplier": "0x172EA50B3535721154ae5B368E850825615882BB"}`).
		Prepare()

	res := sent.Wait(25 * time.Millisecond)
	require.Regexp(t, "PD020216.*timed out.*server throws an error", res.Error())
	require.Nil(t, res.PreparedTransaction())

	cancelCtx()
	res = sent.Wait(1 * time.Minute)
	require.Regexp(t, "PD020000", res.Error())
	require.Nil(t, res.PreparedTransaction())

}

func TestSendUnconnectedFail(t *testing.T) {

	res := New().TxBuilder(context.Background()).
		Public().
		From("tx.sender").
		Function("someFunc").
		To(pldtypes.RandAddress()).
		Inputs(`{"supplier": "0x172EA50B3535721154ae5B368E850825615882BB"}`).
		Send()
	require.Regexp(t, "PD020210", res.Error())

}

func TestIdempotentSubmit(t *testing.T) {
	txID := uuid.New()
	methods := []testRPCMethod{
		{
			name: "ptx_sendTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				return errorResponse(rpcReq.ID, fmt.Errorf("PD012220: key clash" /* note important error code in Paladin */))
			},
		},
		{
			name: "ptx_prepareTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				return errorResponse(rpcReq.ID, fmt.Errorf("PD012220: key clash" /* note important error code in Paladin */))
			},
		},
		{
			name: "ptx_getTransactionByIdempotencyKey",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID string
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, "tx.12345", suppliedID)
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
					"id": "`+txID.String()+`"
				}`))
			},
		},
	}
	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	txb := c.TxBuilder(ctx).
		Private().
		Domain("domain1").
		IdempotencyKey("tx.12345").
		ABIJSON(testABIJSON).
		From("tx.sender").
		Inputs(`{"supplier": "0x172EA50B3535721154ae5B368E850825615882BB"}`)

	send := txb.Send()
	require.NoError(t, send.Error())
	assert.Equal(t, txID, *send.ID())

	prepare := txb.Prepare()
	require.NoError(t, prepare.Error())
	assert.Equal(t, txID, *prepare.ID())

}

func TestIdempotentSubmitRPCCodeConflict(t *testing.T) {
	txID := uuid.New()
	// Simulate new Paladin behaviour: HTTP 200 with RPCCodeConflict (-32001) in the error body
	conflictResponse := func(id pldtypes.RawJSON) (int, *rpcclient.RPCResponse) {
		return 200, &rpcclient.RPCResponse{
			JSONRpc: "2.0",
			ID:      id,
			Error: &rpcclient.RPCError{
				Code:    int64(RPCCodeConflict),
				Message: "PD012220: key clash",
			},
		}
	}
	methods := []testRPCMethod{
		{
			name: "ptx_sendTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				return conflictResponse(rpcReq.ID)
			},
		},
		{
			name: "ptx_prepareTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				return conflictResponse(rpcReq.ID)
			},
		},
		{
			name: "ptx_getTransactionByIdempotencyKey",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{"id": "`+txID.String()+`"}`))
			},
		},
	}
	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	txb := c.TxBuilder(ctx).
		Private().
		Domain("domain1").
		IdempotencyKey("tx.12345").
		ABIJSON(testABIJSON).
		From("tx.sender").
		Inputs(`{"supplier": "0x172EA50B3535721154ae5B368E850825615882BB"}`)

	send := txb.Send()
	require.NoError(t, send.Error())
	assert.Equal(t, txID, *send.ID())

	prepare := txb.Prepare()
	require.NoError(t, prepare.Error())
	assert.Equal(t, txID, *prepare.ID())
}

func TestDeferFunctionSelectError(t *testing.T) {
	ctx, c, done := newTestClientAndServerHTTP(t)
	defer done()

	res := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(ctx).
		Public().
		ABIJSON(testABIJSON).
		Function("wrong").
		To(pldtypes.RandAddress()).
		Send().
		Wait(25 * time.Millisecond)
	require.Regexp(t, "PD020208", res.Error()) // function not found

}

func TestBuildABIDataJSONArray(t *testing.T) {
	ctx, c, done := newTestClientAndServerHTTP(t)
	defer done()

	data, err := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(ctx).
		Public().
		ABIJSON(testABIJSON).
		Function("getWidgets(uint256)").
		To(pldtypes.RandAddress()).
		Inputs(`{"sku": 73588229205}`).
		DataFormat("mode=array&number=hex").
		BuildInputDataJSON()
	require.NoError(t, err)
	require.JSONEq(t, `["0x1122334455"]`, string(data))

}

func TestSendNoABI(t *testing.T) {
	txID := uuid.New()
	expectNil := false

	ctx, c, done := newTestClientAndServerHTTP(t, testRPCMethod{
		name: "ptx_sendTransaction",
		handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
			var tx pldapi.TransactionInput
			err := json.Unmarshal(rpcReq.Params[0], &tx)
			if err != nil {
				return errorResponse(rpcReq.ID, err)
			}
			if expectNil {
				require.Nil(t, tx.Data)
			} else {
				require.JSONEq(t, `{"sku": 73588229205}`, string(tx.Data))
			}
			return successResponse(rpcReq.ID, pldtypes.JSONString(txID))
		},
	})
	defer done()

	builder := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(ctx).
		Public().
		ABIReference((*pldtypes.Bytes32)(pldtypes.RandBytes(32))).
		Function("getWidgets(uint256)").
		From("tx.sender").
		To(pldtypes.RandAddress())

	res := builder.Inputs(`{"sku": 73588229205}`).Send()
	require.NoError(t, res.Error())
	require.Equal(t, txID, *res.ID())

	res = builder.Inputs([]byte(`{"sku": 73588229205}`)).Send()
	require.NoError(t, res.Error())
	require.Equal(t, txID, *res.ID())

	res = builder.Inputs(map[string]any{"sku": 73588229205}).Send()
	require.NoError(t, res.Error())
	require.Equal(t, txID, *res.ID())

	expectNil = true
	res = builder.Inputs(nil).Send()
	require.NoError(t, res.Error())
	require.Equal(t, txID, *res.ID())
}

func TestBuildBadABIFunction(t *testing.T) {
	ctx, c, done := newTestClientAndServerHTTP(t)
	defer done()

	res := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(ctx).
		ABIFunction(&abi.Entry{Type: abi.Function, Inputs: abi.ParameterArray{{Type: "wrongness"}}}).
		Public().
		ABIReference((*pldtypes.Bytes32)(pldtypes.RandBytes(32))).
		Function("getWidgets(uint256)").
		From("tx.sender").
		To(pldtypes.RandAddress()).
		Inputs(`{"sku": 73588229205}`).
		Send()
	assert.Regexp(t, "FF22025", res.Error())
}

func TestErrChainingTXAndReceipt(t *testing.T) {

	builder := New().ForABI(context.Background(), abi.ABI{})

	send := builder.Send()
	require.Regexp(t, "PD020211", send.Error()) // missing public or private

	err := builder.Call()
	require.Regexp(t, "PD020211", err)

	_, err = send.GetTransaction()
	require.Regexp(t, "PD020211", err)

	_, err = send.GetReceipt()
	require.Regexp(t, "PD020211", err)

	_, err = send.GetReceiptFull()
	require.Regexp(t, "PD020211", err)

	prepare := builder.Prepare()
	require.Regexp(t, "PD020211", prepare.Error())
	require.Regexp(t, "PD020211", prepare.Wait(100*time.Microsecond).Error())

	_, err = prepare.GetTransaction()
	require.Regexp(t, "PD020211", err)

	_, err = prepare.GetPreparedTransaction()
	require.Regexp(t, "PD020211", err)

}

func TestBuildBadABIJSON(t *testing.T) {
	ctx, c, done := newTestClientAndServerHTTP(t)
	defer done()

	res := c.ReceiptPollingInterval(1 * time.Millisecond).
		TxBuilder(ctx).
		ABIJSON([]byte(`{!!!! wrong`)).
		Public().
		ABIReference((*pldtypes.Bytes32)(pldtypes.RandBytes(32))).
		Function("getWidgets(uint256)").
		From("tx.sender").
		To(pldtypes.RandAddress()).
		Inputs(`{"sku": 73588229205}`).
		Send()
	assert.Regexp(t, "PD020207", res.Error())
}

func TestGetters(t *testing.T) {

	dep1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	dep2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	tx := &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			IdempotencyKey: "tx1",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			ABIReference:   confutil.P(pldtypes.RandBytes32()),
			From:           "tx.sender",
			To:             pldtypes.RandAddress(),
			Function:       "function1",
			PublicTxOptions: pldapi.PublicTxOptions{
				Gas: confutil.P(pldtypes.HexUint64(100000)),
			},
		},
		DependsOn: []uuid.UUID{dep1, dep2},
		ABI:       abi.ABI{{Type: abi.Constructor}},
		Bytecode:  pldtypes.HexBytes(pldtypes.RandBytes(64)),
	}

	// This isn't a valid TX, but we're just testing getters
	b := New().TxBuilder(context.Background()).Wrap(tx).Clone()
	assert.Equal(t, tx.ABI, b.GetABI())
	assert.Equal(t, tx.IdempotencyKey, b.GetIdempotencyKey())
	assert.Equal(t, pldapi.TransactionTypePrivate, b.GetType())
	assert.Equal(t, "domain1", b.GetDomain())
	assert.Same(t, tx.ABIReference, b.GetABIReference())
	assert.Equal(t, "tx.sender", b.GetFrom())
	assert.Equal(t, tx.To, b.GetTo())
	assert.Equal(t, tx.Data, b.GetInputs())
	assert.Equal(t, "function1", b.GetFunction())
	assert.Equal(t, tx.Bytecode, b.GetBytecode())
	assert.Equal(t, tx.PublicTxOptions, b.GetPublicTxOptions())
	assert.Equal(t, tx.DependsOn, b.GetDependsOn())

	require.NotNil(t, b.Client())

	// Check it doesn't change in the round trip
	tx2 := b.BuildTX().TX()
	require.Equal(t, tx, tx2)

	callTX := &pldapi.TransactionCall{
		TransactionInput: *b.BuildTX().TX(),
		DataFormat:       "mode=array",
		PublicCallOptions: pldapi.PublicCallOptions{
			Block: "latest",
		},
	}
	b = b.WrapCall(callTX)
	require.Equal(t, callTX.PublicCallOptions, b.GetPublicCallOptions())
	require.Equal(t, pldtypes.JSONFormatOptions("mode=array"), b.GetDataFormat())
	var result pldtypes.RawJSON
	b.Outputs(&result)
	require.Equal(t, &result, b.GetOutputs())

	tx3 := b.BuildTX().CallTX()
	require.Equal(t, callTX, tx3)
}

func TestDependsOn(t *testing.T) {
	ctx := context.Background()
	b := New().TxBuilder(ctx)

	// DependsOn(nil) sets nil and allows chaining
	chained := b.DependsOn(nil)
	require.Same(t, b, chained)
	assert.Nil(t, b.GetDependsOn())
	built := b.BuildTX().TX()
	assert.Nil(t, built.DependsOn)

	// DependsOn(empty slice)
	b = New().TxBuilder(ctx).DependsOn([]uuid.UUID{})
	assert.Empty(t, b.GetDependsOn())
	built = b.BuildTX().TX()
	assert.Empty(t, built.DependsOn)

	// DependsOn with one or more UUIDs
	dep1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	dep2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	deps := []uuid.UUID{dep1, dep2}
	b = New().TxBuilder(ctx).DependsOn(deps)
	require.Equal(t, deps, b.GetDependsOn())
	built = b.BuildTX().TX()
	require.Equal(t, deps, built.DependsOn)

	// Overwrite: second DependsOn call replaces
	dep3 := uuid.New()
	b = b.DependsOn([]uuid.UUID{dep3})
	require.Equal(t, []uuid.UUID{dep3}, b.GetDependsOn())
}

func TestBuildCallDataFunction(t *testing.T) {

	builder := New().ForABI(context.Background(), testABI).Function("getWidgets(uint256)")

	type skuInput struct {
		SKU pldtypes.HexUint64 `json:"sku"`
	}

	// A JSON serializable structure
	callData, err := builder.Inputs(&skuInput{SKU: 0x1122334455}).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// A generic structure serializable to an array
	callData, err = builder.Inputs([]string{"0x1122334455"}).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// A generic structure serializable to an object
	callData, err = builder.Inputs(map[string]any{"sku": 0x1122334455}).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// A string JSON array
	callData, err = builder.Inputs(`["0x1122334455"]`).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// A bytes JSON object
	callData, err = builder.Inputs([]byte(`{"sku": "0x1122334455"}`)).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// A pre-parsed component value tree ready to go
	cv, err := testABI.Functions()["getWidgets"].Inputs.ParseJSON([]byte(`{"sku": "0x1122334455"}`))
	require.NoError(t, err)
	callData, err = builder.Inputs(cv).BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0x4f8989ff0000000000000000000000000000000000000000000000000000001122334455", callData.String())

	// Nil when no value is required (default constructor)
	callData, err = New().ForABI(context.Background(), abi.ABI{}).
		Constructor().Inputs(nil).
		Bytecode(pldtypes.MustParseHexBytes("0xfeedbeef")).
		BuildCallData()
	require.NoError(t, err)
	require.Equal(t, "0xfeedbeef", callData.String())

	// Nil when a value is required
	_, err = builder.Inputs(nil).BuildCallData()
	assert.Regexp(t, "PD020203", err)

	// Some broken JSON
	_, err = builder.Inputs(pldtypes.RawJSON(`{!!!! bad json`)).BuildCallData()
	assert.Regexp(t, "PD020200", err)

}

func TestResolveDefinitionNoABI(t *testing.T) {
	_, err := New().TxBuilder(context.Background()).ResolveDefinition()
	assert.Regexp(t, "PD020213", err)
}

func TestNoDomain(t *testing.T) {
	res := New().TxBuilder(context.Background()).Private().Send()
	assert.Regexp(t, "PD020214", res.Error())
}

func TestMissingFunction(t *testing.T) {
	res := New().TxBuilder(context.Background()).
		Private().
		Domain("noto").
		To(pldtypes.RandAddress()).
		Send()
	assert.Regexp(t, "PD020215", res.Error())
}

func TestMissingTo(t *testing.T) {
	res := New().TxBuilder(context.Background()).
		Private().
		Domain("noto").
		Function("someFunc").
		Send()
	assert.Regexp(t, "PD020202", res.Error())
}

func TestIncorrectlyAddingBytecode(t *testing.T) {
	res := New().TxBuilder(context.Background()).
		Private().
		Domain("noto").
		Constructor().
		Bytecode(pldtypes.MustParseHexBytes("0xfeedbeef")).
		Send()
	assert.Regexp(t, "PD020205", res.Error())
}

func TestMissingBytecode(t *testing.T) {
	res := New().TxBuilder(context.Background()).
		Public().
		Constructor().
		Send()
	assert.Regexp(t, "PD020206", res.Error())
}

func TestPrivacyGroupDeployWithConstructor(t *testing.T) {
	groupID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	txID := uuid.New()
	deployedAddr := pldtypes.RandAddress()
	bytecode := pldtypes.HexBytes(pldtypes.RandBytes(64))

	methods := []testRPCMethod{
		{
			name: "pgroup_sendTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var tx pldapi.PrivacyGroupEVMTXInput
				err := json.Unmarshal(rpcReq.Params[0], &tx)
				if err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, "pente", tx.Domain)
				require.Equal(t, groupID, tx.Group)
				require.Equal(t, "member@node1", tx.From)
				require.Nil(t, tx.To)
				require.Equal(t, bytecode, tx.Bytecode)
				require.NotNil(t, tx.Function)
				require.Equal(t, abi.Constructor, tx.Function.Type)
				require.Len(t, tx.Function.Inputs, 1)
				require.JSONEq(t, `{"supplier": "0x172ea50b3535721154ae5b368e850825615882bb"}`, string(tx.Input))
				require.Equal(t, "deploy1", tx.IdempotencyKey)
				require.Equal(t, pldtypes.HexUint64(100000), *tx.PublicTxOptions.Gas)
				return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
			},
		},
		{
			name: "ptx_getTransactionReceipt",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
					"id": "`+txID.String()+`",
					"success": true
				}`))
			},
		},
		{
			name: "ptx_getTransactionReceiptFull",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID uuid.UUID
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, txID, suppliedID)
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
					"id": "`+txID.String()+`",
					"success": true,
					"domainReceipt": {"receipt": {"contractAddress": "`+deployedAddr.String()+`"}}
				}`))
			},
		},
	}
	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	sent := c.TxBuilder(ctx).
		Domain("pente").
		PrivacyGroupID(groupID).
		ABIJSON(testABIJSON).
		Bytecode(bytecode).
		Constructor().
		Inputs(map[string]any{"supplier": "0x172EA50B3535721154ae5B368E850825615882BB"}).
		From("member@node1").
		IdempotencyKey("deploy1").
		PublicTxOptions(pldapi.PublicTxOptions{
			Gas: confutil.P(pldtypes.HexUint64(100000)),
		}).
		Send()

	res := sent.Wait(100 * time.Millisecond)
	require.NoError(t, res.Error())
	require.Equal(t, txID, res.ID())

	full, err := sent.GetReceiptFull()
	require.NoError(t, err)
	var penteReceipt pldapi.PenteDomainReceipt
	require.NoError(t, json.Unmarshal(full.DomainReceipt, &penteReceipt))
	require.Equal(t, deployedAddr, penteReceipt.Receipt.ContractAddress)
}

func TestPrivacyGroupDeployDefaultConstructor(t *testing.T) {
	group := &pldapi.PrivacyGroup{
		Domain: "pente",
		ID:     pldtypes.HexBytes(pldtypes.RandBytes(32)),
	}
	txID := uuid.New()
	bytecode := pldtypes.HexBytes(pldtypes.RandBytes(64))

	ctx, c, done := newTestClientAndServerHTTP(t, testRPCMethod{
		name: "pgroup_sendTransaction",
		handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
			var tx pldapi.PrivacyGroupEVMTXInput
			err := json.Unmarshal(rpcReq.Params[0], &tx)
			if err != nil {
				return errorResponse(rpcReq.ID, err)
			}
			require.Equal(t, group.ID, tx.Group)
			require.Equal(t, bytecode, tx.Bytecode)
			require.NotNil(t, tx.Function)
			require.Equal(t, abi.Constructor, tx.Function.Type)
			require.Empty(t, tx.Function.Inputs)
			require.JSONEq(t, `{}`, string(tx.Input))
			return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
		},
	})
	defer done()

	// The ABI has no constructor entry - a default constructor is supplied
	sent := c.TxBuilder(ctx).
		PrivacyGroup(group).
		ABIJSON([]byte(`[{"name": "get", "type": "function", "inputs": [], "outputs": []}]`)).
		Bytecode(bytecode).
		Constructor().
		From("member@node1").
		Send()
	require.NoError(t, sent.Error())
	require.Equal(t, txID, *sent.ID())
}

func TestPrivacyGroupDeployNoABI(t *testing.T) {
	group := &pldapi.PrivacyGroup{
		Domain: "pente",
		ID:     pldtypes.HexBytes(pldtypes.RandBytes(32)),
	}
	txID := uuid.New()
	bytecode := pldtypes.HexBytes(pldtypes.RandBytes(64))

	ctx, c, done := newTestClientAndServerHTTP(t, testRPCMethod{
		name: "pgroup_sendTransaction",
		handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
			var tx pldapi.PrivacyGroupEVMTXInput
			err := json.Unmarshal(rpcReq.Params[0], &tx)
			if err != nil {
				return errorResponse(rpcReq.ID, err)
			}
			require.Equal(t, group.ID, tx.Group)
			require.Equal(t, bytecode, tx.Bytecode)
			require.Nil(t, tx.Function)
			require.Empty(t, tx.Input)
			return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
		},
	})
	defer done()

	// No ABI supplied - the bytecode-only deploy passes through without a function definition
	sent := c.TxBuilder(ctx).
		PrivacyGroup(group).
		Bytecode(bytecode).
		From("member@node1").
		Send()
	require.NoError(t, sent.Error())
	require.Equal(t, txID, *sent.ID())
}

func TestPrivacyGroupInvoke(t *testing.T) {
	group := &pldapi.PrivacyGroup{
		Domain: "pente",
		ID:     pldtypes.HexBytes(pldtypes.RandBytes(32)),
	}
	contractAddr := pldtypes.RandAddress()
	txID := uuid.New()

	ctx, c, done := newTestClientAndServerHTTP(t, testRPCMethod{
		name: "pgroup_sendTransaction",
		handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
			var tx pldapi.PrivacyGroupEVMTXInput
			err := json.Unmarshal(rpcReq.Params[0], &tx)
			if err != nil {
				return errorResponse(rpcReq.ID, err)
			}
			require.Equal(t, "pente", tx.Domain)
			require.Equal(t, group.ID, tx.Group)
			require.Equal(t, contractAddr, tx.To)
			require.NotNil(t, tx.Function)
			require.Equal(t, "newWidget", tx.Function.Name)
			require.JSONEq(t, `{
				"widget": {
					"id": "0x172ea50b3535721154ae5b368e850825615882bb",
					"sku": "12345",
					"features": ["blue", "round"]
				}
			}`, string(tx.Input))
			return successResponse(rpcReq.ID, pldtypes.JSONString(txID.String()))
		},
	})
	defer done()

	sent := c.ForABI(ctx, testABI).
		PrivacyGroup(group).
		Function("newWidget").
		To(contractAddr).
		Inputs(map[string]any{
			"widget": map[string]any{
				"id":       "0x172EA50B3535721154ae5B368E850825615882BB",
				"sku":      12345,
				"features": []string{"blue", "round"},
			},
		}).
		From("member@node1").
		Send()
	require.NoError(t, sent.Error())
	require.Equal(t, txID, *sent.ID())
}

func TestPrivacyGroupCall(t *testing.T) {
	group := &pldapi.PrivacyGroup{
		Domain: "pente",
		ID:     pldtypes.HexBytes(pldtypes.RandBytes(32)),
	}
	contractAddr := pldtypes.RandAddress()

	ctx, c, done := newTestClientAndServerHTTP(t, testRPCMethod{
		name: "pgroup_call",
		handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
			var call pldapi.PrivacyGroupEVMCall
			err := json.Unmarshal(rpcReq.Params[0], &call)
			if err != nil {
				return errorResponse(rpcReq.ID, err)
			}
			require.Equal(t, "pente", call.Domain)
			require.Equal(t, group.ID, call.Group)
			require.Equal(t, contractAddr, call.To)
			require.Equal(t, "member@node1", call.From)
			require.NotNil(t, call.Function)
			require.Equal(t, "getWidgets", call.Function.Name)
			require.JSONEq(t, `["12345"]`, string(call.Input))
			require.Equal(t, pldtypes.JSONFormatOptions("mode=array"), call.DataFormat)
			require.Equal(t, "latest", call.Block.String())
			return successResponse(rpcReq.ID, pldtypes.RawJSON(`[[
				"0x172ea50b3535721154ae5b368e850825615882bb",
				"12345",
				["blue", "round"]
			]]`))
		},
	})
	defer done()

	var widgets []any
	err := c.ForABI(ctx, testABI).
		PrivacyGroup(group).
		Function("getWidgets").
		To(contractAddr).
		Inputs([]int{12345}).
		Outputs(&widgets).
		DataFormat("mode=array").
		PublicCallOptions(pldapi.PublicCallOptions{
			Block: "latest",
		}).
		From("member@node1").
		Call()
	require.NoError(t, err)
	require.Len(t, widgets, 1)
}

func TestPrivacyGroupIdempotentSubmit(t *testing.T) {
	groupID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	txID := uuid.New()
	bytecode := pldtypes.HexBytes(pldtypes.RandBytes(64))

	methods := []testRPCMethod{
		{
			name: "pgroup_sendTransaction",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				return errorResponse(rpcReq.ID, fmt.Errorf("PD012220: key clash"))
			},
		},
		{
			name: "ptx_getTransactionByIdempotencyKey",
			handler: func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse) {
				var suppliedID string
				if err := json.Unmarshal(rpcReq.Params[0], &suppliedID); err != nil {
					return errorResponse(rpcReq.ID, err)
				}
				require.Equal(t, "deploy1", suppliedID)
				return successResponse(rpcReq.ID, pldtypes.RawJSON(`{
					"id": "`+txID.String()+`"
				}`))
			},
		},
	}
	ctx, c, done := newTestClientAndServerHTTP(t, methods...)
	defer done()

	sent := c.TxBuilder(ctx).
		Domain("pente").
		PrivacyGroupID(groupID).
		ABIJSON([]byte(`[]`)).
		Bytecode(bytecode).
		Constructor().
		IdempotencyKey("deploy1").
		From("member@node1").
		Send()
	require.NoError(t, sent.Error())
	require.Equal(t, txID, *sent.ID())
}

func TestPrivacyGroupValidation(t *testing.T) {
	ctx := context.Background()
	groupID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	bytecode := pldtypes.MustParseHexBytes("0xfeedbeef")

	// An explicit Public() conflicts with a privacy group target
	res := New().TxBuilder(ctx).
		Domain("pente").
		PrivacyGroupID(groupID).
		Public().
		ABIJSON(testABIJSON).
		Constructor().
		Bytecode(bytecode).
		From("member@node1").
		Send()
	assert.Regexp(t, "PD020218", res.Error())

	// Domain still required
	res = New().TxBuilder(ctx).
		PrivacyGroupID(groupID).
		ABIJSON(testABIJSON).
		Constructor().
		Bytecode(bytecode).
		From("member@node1").
		Send()
	assert.Regexp(t, "PD020214", res.Error())

	// An invoke still requires a function
	res = New().TxBuilder(ctx).
		Domain("pente").
		PrivacyGroupID(groupID).
		ABIJSON(testABIJSON).
		To(pldtypes.RandAddress()).
		From("member@node1").
		Send()
	assert.Regexp(t, "PD020215", res.Error())

	// A deploy requires bytecode
	res = New().TxBuilder(ctx).
		Domain("pente").
		PrivacyGroupID(groupID).
		ABIJSON(testABIJSON).
		Constructor().
		From("member@node1").
		Send()
	assert.Regexp(t, "PD020220", res.Error())

	// A function still requires a to address
	res = New().TxBuilder(ctx).
		Domain("pente").
		PrivacyGroupID(groupID).
		ABIJSON(testABIJSON).
		Function("newWidget").
		From("member@node1").
		Send()
	assert.Regexp(t, "PD020202", res.Error())

	// A named function requires an ABI to resolve the full definition to send
	res = New().TxBuilder(ctx).
		Domain("pente").
		PrivacyGroupID(groupID).
		Function("someFunc").
		To(pldtypes.RandAddress()).
		Inputs(`[]`).
		From("member@node1").
		Send()
	assert.Regexp(t, "PD020213", res.Error())

	// Prepare is not supported
	prepare := New().TxBuilder(ctx).
		Domain("pente").
		PrivacyGroupID(groupID).
		ABIJSON([]byte(`[]`)).
		Constructor().
		Bytecode(bytecode).
		From("member@node1").
		Prepare()
	assert.Regexp(t, "PD020219", prepare.Error())
}

func TestPrivacyGroupGetters(t *testing.T) {
	group := &pldapi.PrivacyGroup{
		Domain: "pente",
		ID:     pldtypes.HexBytes(pldtypes.RandBytes(32)),
	}
	b := New().TxBuilder(context.Background()).PrivacyGroup(group).Clone()
	assert.Equal(t, group.ID, b.GetPrivacyGroupID())
	assert.Equal(t, "pente", b.GetDomain())
}
