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

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

var _ plugintk.DomainCallbacks = &MockDomainCallbacks{}

type MockDomainCallbacks struct {
	MockFindAvailableStates func() (*prototk.FindAvailableStatesResponse, error)
	MockLocalNodeName       func() (*prototk.LocalNodeNameResponse, error)
	MockValidateStates      func(ctx context.Context, req *prototk.ValidateStatesRequest) (*prototk.ValidateStatesResponse, error)
	MockReverseKeyLookup    func(ctx context.Context, req *prototk.ReverseKeyLookupRequest) (*prototk.ReverseKeyLookupResponse, error)
}

func (dc *MockDomainCallbacks) FindAvailableStates(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	return dc.MockFindAvailableStates()
}

func (dc *MockDomainCallbacks) EncodeData(ctx context.Context, req *prototk.EncodeDataRequest) (*prototk.EncodeDataResponse, error) {
	return nil, nil
}
func (dc *MockDomainCallbacks) RecoverSigner(ctx context.Context, req *prototk.RecoverSignerRequest) (*prototk.RecoverSignerResponse, error) {
	return nil, nil
}

func (dc *MockDomainCallbacks) DecodeData(context.Context, *prototk.DecodeDataRequest) (*prototk.DecodeDataResponse, error) {
	return nil, nil
}

func (dc *MockDomainCallbacks) SendTransaction(context.Context, *prototk.SendTransactionRequest) (*prototk.SendTransactionResponse, error) {
	return nil, nil
}

func (dc *MockDomainCallbacks) LocalNodeName(context.Context, *prototk.LocalNodeNameRequest) (*prototk.LocalNodeNameResponse, error) {
	return dc.MockLocalNodeName()
}

func (dc *MockDomainCallbacks) GetStatesByID(context.Context, *prototk.GetStatesByIDRequest) (*prototk.GetStatesByIDResponse, error) {
	return nil, nil
}

func (dc *MockDomainCallbacks) ReverseKeyLookup(ctx context.Context, req *prototk.ReverseKeyLookupRequest) (*prototk.ReverseKeyLookupResponse, error) {
	return dc.MockReverseKeyLookup(ctx, req)
}

func (dc *MockDomainCallbacks) ValidateStates(ctx context.Context, req *prototk.ValidateStatesRequest) (*prototk.ValidateStatesResponse, error) {
	return dc.MockValidateStates(ctx, req)
}
