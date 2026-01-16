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

package domain

import (
	"context"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func TestMockDomainCallbacksFindAvailableStates(t *testing.T) {
	expectedResponse := &prototk.FindAvailableStatesResponse{
		States: []*prototk.StoredState{
			{Id: "state1"},
			{Id: "state2"},
		},
	}

	mc := &MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return expectedResponse, nil
		},
	}

	result, err := mc.FindAvailableStates(context.Background(), &prototk.FindAvailableStatesRequest{})
	assert.NoError(t, err)
	assert.Equal(t, expectedResponse, result)
}

func TestMockDomainCallbacksFindAvailableStatesError(t *testing.T) {
	expectedError := fmt.Errorf("database error")

	mc := &MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return nil, expectedError
		},
	}

	result, err := mc.FindAvailableStates(context.Background(), &prototk.FindAvailableStatesRequest{})
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedError, err)
}

func TestMockDomainCallbacksEncodeData(t *testing.T) {
	mc := &MockDomainCallbacks{}

	result, err := mc.EncodeData(context.Background(), &prototk.EncodeDataRequest{})
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestMockDomainCallbacksRecoverSigner(t *testing.T) {
	mc := &MockDomainCallbacks{}

	result, err := mc.RecoverSigner(context.Background(), &prototk.RecoverSignerRequest{})
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestMockDomainCallbacksDecodeData(t *testing.T) {
	mc := &MockDomainCallbacks{}

	result, err := mc.DecodeData(context.Background(), &prototk.DecodeDataRequest{})
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestMockDomainCallbacksSendTransaction(t *testing.T) {
	mc := &MockDomainCallbacks{}

	result, err := mc.SendTransaction(context.Background(), &prototk.SendTransactionRequest{})
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestMockDomainCallbacksLocalNodeName(t *testing.T) {
	expectedResponse := &prototk.LocalNodeNameResponse{
		Name: "test-node",
	}

	mc := &MockDomainCallbacks{
		MockLocalNodeName: func() (*prototk.LocalNodeNameResponse, error) {
			return expectedResponse, nil
		},
	}

	result, err := mc.LocalNodeName(context.Background(), &prototk.LocalNodeNameRequest{})
	assert.NoError(t, err)
	assert.Equal(t, expectedResponse, result)
}

func TestMockDomainCallbacksLocalNodeNameError(t *testing.T) {
	expectedError := fmt.Errorf("node name unavailable")

	mc := &MockDomainCallbacks{
		MockLocalNodeName: func() (*prototk.LocalNodeNameResponse, error) {
			return nil, expectedError
		},
	}

	result, err := mc.LocalNodeName(context.Background(), &prototk.LocalNodeNameRequest{})
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedError, err)
}

func TestMockDomainCallbacksGetStatesByID(t *testing.T) {
	mc := &MockDomainCallbacks{}

	result, err := mc.GetStatesByID(context.Background(), &prototk.GetStatesByIDRequest{})
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestMockDomainCallbacksWithContext(t *testing.T) {
	// Verify that context is properly passed through
	ctx := context.WithValue(context.Background(), "key", "value")

	mc := &MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{}, nil
		},
		MockLocalNodeName: func() (*prototk.LocalNodeNameResponse, error) {
			return &prototk.LocalNodeNameResponse{}, nil
		},
	}

	// Should work with custom context
	_, err := mc.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{})
	assert.NoError(t, err)

	_, err = mc.LocalNodeName(ctx, &prototk.LocalNodeNameRequest{})
	assert.NoError(t, err)
}

func TestMockDomainCallbacksNilMocks(t *testing.T) {
	// Test that calling with nil mock functions causes panic (expected behavior)
	mc := &MockDomainCallbacks{
		MockFindAvailableStates: nil,
	}

	// This should panic when trying to call a nil function
	assert.Panics(t, func() {
		mc.FindAvailableStates(context.Background(), &prototk.FindAvailableStatesRequest{})
	})
}

func TestMockDomainCallbacksMultipleInstances(t *testing.T) {
	mc1 := &MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{Id: "mc1"}}}, nil
		},
	}

	mc2 := &MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{Id: "mc2"}}}, nil
		},
	}

	result1, _ := mc1.FindAvailableStates(context.Background(), &prototk.FindAvailableStatesRequest{})
	result2, _ := mc2.FindAvailableStates(context.Background(), &prototk.FindAvailableStatesRequest{})

	assert.Equal(t, "mc1", result1.States[0].Id)
	assert.Equal(t, "mc2", result2.States[0].Id)
}

func TestMockDomainCallbacksComplexResponse(t *testing.T) {
	// Test with more complex response structures
	expectedResponse := &prototk.FindAvailableStatesResponse{
		States: []*prototk.StoredState{
			{
				Id:       "state1",
				DataJson: "{\"field1\": \"value1\"}",
			},
			{
				Id:       "state2",
				DataJson: "{\"field2\": \"value2\"}",
			},
		},
	}

	mc := &MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return expectedResponse, nil
		},
	}

	result, err := mc.FindAvailableStates(context.Background(), &prototk.FindAvailableStatesRequest{})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.States, 2)
	assert.Equal(t, "state1", result.States[0].Id)
	assert.Equal(t, "{\"field1\": \"value1\"}", result.States[0].DataJson)
	assert.Equal(t, "state2", result.States[1].Id)
	assert.Equal(t, "{\"field2\": \"value2\"}", result.States[1].DataJson)
}
