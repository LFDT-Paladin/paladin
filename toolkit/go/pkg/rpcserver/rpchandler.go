// Copyright © 2024 Kaleido, Inc.
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

package rpcserver

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"
	"unicode"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/pldmsgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
)

type handlerResult struct {
	sendRes      bool
	res          any
	postSend     func()
	unauthorized bool
}

func (s *rpcServer) rpcHandler(ctx context.Context, r io.Reader, wsc *webSocketConnection) handlerResult {

	b, err := io.ReadAll(r)
	if err != nil {
		return s.replyRPCParseError(ctx, b, err)
	}

	if log.IsTraceEnabled() {
		log.L(ctx).Tracef("RPC[Server] --> %s", b)
	}

	if s.sniffFirstByte(b) == '[' {
		var rpcArray []*rpcclient.RPCRequest
		err := json.Unmarshal(b, &rpcArray)
		if err != nil || len(rpcArray) == 0 {
			log.L(ctx).Errorf("Bad RPC array received %s", b)
			return s.replyRPCParseError(ctx, b, err)
		}
		batchRes, unauthorized, postSend := s.handleRPCBatch(ctx, rpcArray, wsc)
		return handlerResult{sendRes: true, res: batchRes, postSend: postSend, unauthorized: unauthorized}
	}

	var rpcRequest rpcclient.RPCRequest
	err = json.Unmarshal(b, &rpcRequest)
	if err != nil {
		return s.replyRPCParseError(ctx, b, err)
	}
	startTime := time.Now()
	log.L(ctx).Debugf("RPC-server[%s] --> %s", rpcRequest.ID, rpcRequest.Method)
	res, unauthorized, postSend := s.processRPC(ctx, &rpcRequest, wsc)
	durationMS := float64(time.Since(startTime)) / float64(time.Millisecond)
	if res != nil && res.Error != nil {
		log.L(ctx).Errorf("RPC-server[%s] <-- %s [%.2fms]: %s", rpcRequest.ID.StringValue(), rpcRequest.Method, durationMS, res.Error.Message)
	} else {
		log.L(ctx).Debugf("RPC-server[%s] <-- %s [%.2fms]", rpcRequest.ID.StringValue(), rpcRequest.Method, durationMS)
	}
	if log.IsTraceEnabled() {
		log.L(ctx).Tracef("RPC-server[%s] <-- %s", rpcRequest.ID.StringValue(), pldtypes.JSONString(res))
	}
	return handlerResult{sendRes: res != nil, res: res, postSend: postSend, unauthorized: unauthorized}

}

func (s *rpcServer) replyRPCParseError(ctx context.Context, b []byte, err error) handlerResult {
	log.L(ctx).Errorf("Request could not be parsed (err=%v): %s", err, b)
	return handlerResult{
		sendRes: true,
		res: rpcclient.NewRPCErrorResponse(
			i18n.NewError(ctx, pldmsgs.MsgJSONRPCInvalidRequest),
			pldtypes.RawJSON(`"1"`),
			rpcclient.RPCCodeInvalidRequest,
		),
	}
}

func (s *rpcServer) sniffFirstByte(data []byte) byte {
	sniffLen := len(data)
	sniffLen = min(sniffLen, 100)
	for _, b := range data[0:sniffLen] {
		if !unicode.IsSpace(rune(b)) {
			return b
		}
	}
	return 0x00
}

func (s *rpcServer) handleRPCBatch(ctx context.Context, rpcArray []*rpcclient.RPCRequest, wsc *webSocketConnection) ([]*rpcclient.RPCResponse, bool, func()) {
	// Kick off a routine to fill in each
	rpcResponses := make([]*rpcclient.RPCResponse, len(rpcArray))
	postSends := make([]func(), len(rpcArray))
	var wg sync.WaitGroup
	var unauthorizedMu sync.Mutex
	anyUnauthorized := false
	for i, r := range rpcArray {
		responseNumber := i
		rpcRequest := r
		wg.Add(1)
		go func() {
			defer wg.Done()
			startTime := time.Now()
			log.L(ctx).Debugf("RPC-server[%v] (b=%d) --> %s", rpcRequest.ID.StringValue(), i, rpcRequest.Method)
			res, unauthorized, postSend := s.processRPC(ctx, rpcRequest, wsc)
			if unauthorized {
				unauthorizedMu.Lock()
				anyUnauthorized = true
				unauthorizedMu.Unlock()
			}
			durationMS := float64(time.Since(startTime)) / float64(time.Millisecond)
			if res != nil && res.Error != nil {
				log.L(ctx).Errorf("RPC-server[%s] (b=%d) <-- %s [%.2fms]: %s", rpcRequest.ID.StringValue(), i, rpcRequest.Method, durationMS, res.Error.Message)
			} else {
				log.L(ctx).Debugf("RPC-server[%s] (b=%d) <-- %s [%.2fms]", rpcRequest.ID.StringValue(), i, rpcRequest.Method, durationMS)
			}
			if log.IsTraceEnabled() {
				log.L(ctx).Tracef("RPC-server[%s] (b=%d) <-- %s", rpcRequest.ID.StringValue(), i, pldtypes.JSONString(res))
			}
			rpcResponses[responseNumber] = res
			postSends[responseNumber] = postSend
		}()
	}
	wg.Wait()
	return rpcResponses, anyUnauthorized, func() {
		for _, postSend := range postSends {
			if postSend != nil {
				postSend()
			}
		}
	}
}
