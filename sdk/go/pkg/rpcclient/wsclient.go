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

package rpcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/pldmsgs"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/retry"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/wsclient"
	"github.com/google/uuid"
)

func NewWSClient(ctx context.Context, conf *pldconf.WSClientConfig) (WSClient, error) {
	err := ValidateWSClientConfig(ctx, conf)
	if err != nil {
		return nil, err
	}

	return WrapWSConfig(conf), nil
}

func WrapWSConfig(conf *pldconf.WSClientConfig) WSClient {
	return &wsRPCClient{
		wsConf:                      *conf,
		requestTimeout:              confutil.DurationMin(conf.WSRequestTimeout, 1*time.Second, *pldconf.DefaultWSConfig.WSRequestTimeout),
		subscriptionDeliveryTimeout: confutil.DurationMin(conf.SubscriptionDeliveryTimeout, 0, *pldconf.DefaultWSConfig.SubscriptionDeliveryTimeout),
		calls:                       make(map[string]chan *RPCResponse),
		configuredSubs:              make(map[uuid.UUID]*sub),
		pendingSubsByReqID:          make(map[string]*sub),
		activeSubsBySubID:           make(map[string]*sub),
		notificationMethods:         make(map[string]bool),
		closing:                     make(chan struct{}),
	}
}

func ValidateWSClientConfig(ctx context.Context, conf *pldconf.WSClientConfig) error {
	_, _, err := wsclient.ValidateConfig(ctx, conf)
	return err
}

type Subscription interface {
	LocalID() uuid.UUID // does not change through reconnects
	Notifications() chan RPCSubscriptionNotification
	Unsubscribe(ctx context.Context) ErrorRPC
	Err() ErrorRPC // set if the client fails the subscription and closes Notifications() itself
}

type RPCSubscriptionNotification interface {
	Ack(ctx context.Context) ErrorRPC
	Nack(ctx context.Context) ErrorRPC
	GetCurrentSubID() string
	GetResult() pldtypes.RawJSON
}

type rpcSubscriptionNotification struct {
	wsc          *wsRPCClient
	sub          *sub
	CurrentSubID string // will change on each reconnect
	Result       pldtypes.RawJSON
}

func (n *rpcSubscriptionNotification) Ack(ctx context.Context) ErrorRPC {
	id, req := n.wsc.newAsyncReq(n.sub.AckMethod, pldtypes.JSONString(n.CurrentSubID))
	return n.wsc.sendRPC(ctx, id, req)
}

func (n *rpcSubscriptionNotification) Nack(ctx context.Context) ErrorRPC {
	id, req := n.wsc.newAsyncReq(n.sub.NackMethod, pldtypes.JSONString(n.CurrentSubID))
	return n.wsc.sendRPC(ctx, id, req)
}

func (n *rpcSubscriptionNotification) GetCurrentSubID() string {
	return n.CurrentSubID
}

func (n *rpcSubscriptionNotification) GetResult() pldtypes.RawJSON {
	return n.Result
}

type wsRPCClient struct {
	mux                         sync.Mutex
	wsConf                      pldconf.WSClientConfig
	requestTimeout              time.Duration
	subscriptionDeliveryTimeout time.Duration
	client                      wsclient.WSClient
	requestCounter              int64
	connected                   chan struct{}
	calls                       map[string]chan *RPCResponse
	configuredSubs              map[uuid.UUID]*sub
	pendingSubsByReqID          map[string]*sub
	activeSubsBySubID           map[string]*sub
	notificationMethods         map[string]bool
	closed                      bool
	closing                     chan struct{}
	connectMux                  sync.Mutex // serialises Connect, which waits on the reconnect hook and so cannot hold mux
	established                 bool
}

type sub struct {
	SubscriptionConfig
	localID        uuid.UUID
	rc             *wsRPCClient
	ctx            context.Context
	cancelCtx      context.CancelFunc
	params         []interface{}
	pendingReqID   string
	currentSubID   string
	newSubResponse chan ErrorRPC
	notifications  chan RPCSubscriptionNotification
	err            ErrorRPC
	deliverMux     sync.Mutex
	closed         bool
	resubscribing  bool
}

// Connecting an already connected client keeps its transport. A failed attempt leaves nothing running, so it can be retried
func (rc *wsRPCClient) Connect(ctx context.Context) error {
	rc.connectMux.Lock()
	defer rc.connectMux.Unlock()
	if rc.isClosed() {
		return NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientClosed)
	}
	if rc.established {
		return nil
	}
	client, err := wsclient.New(ctx, &rc.wsConf, nil, rc.handleReconnect)
	if err != nil {
		return err
	}
	if !rc.setClient(client) {
		client.Close()
		return NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientClosed)
	}

	// Wait until the afterConnect hook has been driven
	connected := make(chan struct{})
	rc.connected = connected
	if err = client.Connect(); err == nil {
		go rc.receiveLoop(log.WithLogField(ctx, "role", "rpc_websocket"))
		err = rc.waitConnected(ctx, connected)
	}
	if err != nil {
		client.Close()
		return err
	}
	rc.established = true
	return nil
}

func (rc *wsRPCClient) isClosed() bool {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	return rc.closed
}

func (rc *wsRPCClient) setClient(client wsclient.WSClient) bool {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	if rc.closed {
		return false
	}
	rc.client = client
	return true
}

func (rc *wsRPCClient) waitConnected(ctx context.Context, connected chan struct{}) error {
	select {
	case <-connected:
	case <-ctx.Done():
		return i18n.NewError(ctx, pldmsgs.MsgContextCanceled)
	case <-rc.closing:
		return NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientClosed)
	}
	return nil
}

// Closing is final: every subscription fails with an error from Err, and the client cannot be connected again
func (rc *wsRPCClient) Close() {
	subs, client := rc.markClosed()
	for _, s := range subs {
		rc.removeSubscription(s)
		s.closeNotifications()
	}
	if client != nil {
		client.Close()
	}
}

func (rc *wsRPCClient) markClosed() ([]*sub, wsclient.WSClient) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	if rc.closed {
		return nil, nil
	}
	rc.closed = true
	close(rc.closing)
	closedErr := NewRPCError(context.Background(), RPCCodeInternalError, pldmsgs.MsgRPCClientClosed)
	subs := make([]*sub, 0, len(rc.configuredSubs))
	for _, s := range rc.configuredSubs {
		s.err = closedErr
		subs = append(subs, s)
	}
	return subs, rc.client
}

func (rc *wsRPCClient) handleReconnect(ctx context.Context, w wsclient.WSClient) error {
	calls, subs := rc.clearActiveReturnConfiguredSubs()
	for rpcID, c := range calls {
		rc.deliverCallResponse(c, &RPCResponse{
			ID:    pldtypes.RawJSON(`"` + rpcID + `"`),
			Error: NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientWebSocketReconnected),
		})
	}
	for _, s := range subs {
		log.L(ctx).Infof("Resubscribing %s after WebSocket reconnect", s.localID)
		_, rpcErr := s.sendSubscribe(ctx)
		if rpcErr != nil {
			log.L(ctx).Errorf("Failed to send resubscribe: %s", rpcErr)
			return rpcErr
		}
	}

	if rc.connected != nil {
		close(rc.connected)
		rc.connected = nil
	}
	return nil
}

func (rc *wsRPCClient) newAsyncReq(method string, params ...pldtypes.RawJSON) (string, *RPCRequest) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	rc.requestCounter++
	reqID := fmt.Sprintf(`"%.9d"`, rc.requestCounter)
	return reqID, &RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(reqID),
		Method:  method,
		Params:  params,
	}
}

func (rc *wsRPCClient) addInflightRequest(req *RPCRequest) (string, chan *RPCResponse) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	rc.requestCounter++
	reqID := fmt.Sprintf("%.9d", rc.requestCounter)
	req.ID = pldtypes.RawJSON(`"` + reqID + `"`)
	resChl := make(chan *RPCResponse, 1)
	rc.calls[reqID] = resChl
	return reqID, resChl
}

func (rc *wsRPCClient) addInflightSub(s *sub) string {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	return rc.addInflightSubLocked(s)
}

func (rc *wsRPCClient) addInflightSubLocked(s *sub) string {
	rc.requestCounter++
	s.pendingReqID = fmt.Sprintf("%.9d", rc.requestCounter)
	s.currentSubID = ""
	rc.pendingSubsByReqID[s.pendingReqID] = s
	return s.pendingReqID
}

func (rc *wsRPCClient) popInflight(rpcID string) (*sub, chan *RPCResponse) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	s, ok := rc.pendingSubsByReqID[rpcID]
	if ok {
		s.pendingReqID = ""
		delete(rc.pendingSubsByReqID, rpcID)
		return s, nil
	}
	inflightCall, ok := rc.calls[rpcID]
	if ok {
		delete(rc.calls, rpcID)
		return nil, inflightCall
	}
	return nil, nil
}

func (rc *wsRPCClient) addActiveSub(s *sub, subscriptionID string) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	s.currentSubID = subscriptionID
	rc.activeSubsBySubID[s.currentSubID] = s
}

func (rc *wsRPCClient) getActiveSub(subID string) *sub {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	return rc.activeSubsBySubID[subID]
}

func (rc *wsRPCClient) removeSubscription(s *sub) string {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	// Removes from configured, pending and active lists
	delete(rc.configuredSubs, s.localID)
	if s.currentSubID != "" {
		delete(rc.activeSubsBySubID, s.currentSubID)
	}
	if s.pendingReqID != "" {
		delete(rc.pendingSubsByReqID, s.pendingReqID)
	}
	s.cancelCtx() // we unblock the receiver if it was previously trying to dispatch to this subscription, before invoking unsubscribe
	currentSubID := s.currentSubID
	s.currentSubID = "" // cleared so that removing an already-removed subscription does not unsubscribe on the server again
	return currentSubID
}

// Returns no subscription once the client is closed
func (rc *wsRPCClient) addConfiguredSub(ctx context.Context, conf SubscriptionConfig, params []interface{}) (*sub, chan ErrorRPC) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	if rc.closed {
		return nil, nil
	}
	s := &sub{
		SubscriptionConfig: conf,
		rc:                 rc,
		localID:            uuid.New(),
		params:             params,
		newSubResponse:     make(chan ErrorRPC, 1),
		notifications:      make(chan RPCSubscriptionNotification), // blocking channel for these, but Unsubscribe will unblock by cancelling ctx
	}
	s.ctx, s.cancelCtx = context.WithCancel(ctx)
	rc.configuredSubs[s.localID] = s
	rc.notificationMethods[conf.NotificationMethod] = true // trigger notification handling for this method type
	// need to return newSubResponse because it's a use-once thing (not on reconnect)
	// and will be nilled out (under lock) when the first creation response comes in
	return s, s.newSubResponse
}

func (rc *wsRPCClient) clearActiveReturnConfiguredSubs() (map[string]chan *RPCResponse, map[uuid.UUID]*sub) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	// Return a copy of all the in-flight RPC calls, before we clear those (as they will all be defunct now)
	calls := rc.calls
	rc.calls = make(map[string]chan *RPCResponse)
	// Clear the active state as considered now invalid after a reconnect
	rc.activeSubsBySubID = make(map[string]*sub)
	rc.pendingSubsByReqID = make(map[string]*sub)
	// Return all the configured ones so we can re-establish them on the new connecti
	subs := make(map[uuid.UUID]*sub)
	for id, s := range rc.configuredSubs {
		s.currentSubID = ""
		s.pendingReqID = ""
		subs[id] = s
	}
	return calls, subs
}

func (rc *wsRPCClient) getAllSubs() []*sub {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	subs := make([]*sub, 0)
	for _, s := range rc.configuredSubs {
		subs = append(subs, s)
	}
	return subs
}

func (rc *wsRPCClient) removeInflightRequest(reqID string) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	delete(rc.calls, reqID)
}

func (rc *wsRPCClient) Subscribe(ctx context.Context, conf SubscriptionConfig, params ...interface{}) (sub Subscription, error ErrorRPC) {

	s, newSubResponse := rc.addConfiguredSub(ctx, conf, params)
	if s == nil {
		return nil, NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientClosed)
	}

	reqID, rpcErr := s.sendSubscribe(ctx)
	if rpcErr != nil {
		rc.removeSubscription(s)
		return nil, rpcErr
	}

	select {
	case rpcErr := <-newSubResponse:
		if rpcErr != nil {
			rc.removeSubscription(s)
			return nil, rpcErr
		}
		return s, nil
	case <-s.ctx.Done():
		// Cancelled by the caller, or by Close which sets the error first
		rc.removeSubscription(s)
		if closedErr := s.Err(); closedErr != nil {
			return nil, closedErr
		}
		return nil, NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgContextCanceled, reqID)
	}
}

func (s *sub) sendSubscribe(ctx context.Context) (string, ErrorRPC) {
	rpcReq, rpcErr := buildRequest(ctx, s.SubscribeMethod, s.params)
	if rpcErr != nil {
		return "", rpcErr
	}
	reqID := s.rc.addInflightSub(s)
	rpcReq.ID = pldtypes.RawJSON(`"` + reqID + `"`)

	return reqID, s.rc.sendRPC(ctx, s.pendingReqID, rpcReq)
}

func (s *sub) LocalID() uuid.UUID {
	return s.localID
}

func (s *sub) Notifications() chan RPCSubscriptionNotification {
	return s.notifications
}

func (s *sub) Unsubscribe(ctx context.Context) ErrorRPC {
	currentSubID := s.rc.removeSubscription(s)
	// The subscription is already removed locally, so the channel closes even if the server call fails
	defer s.closeNotifications()
	var resultBool bool
	if currentSubID != "" {
		// If currently active, we need to unsubscribe
		rpcErr := s.rc.CallRPC(ctx, &resultBool, s.UnsubscribeMethod, currentSubID)
		if rpcErr != nil {
			return rpcErr
		}
	}
	log.L(ctx).Infof("Unsubscribed %s (subid=%s,result=%t)", s.localID, currentSubID, resultBool)
	return nil
}

func (s *sub) Err() ErrorRPC {
	s.rc.mux.Lock()
	defer s.rc.mux.Unlock()
	return s.err
}

// The subscription context must already be cancelled, so that any delivery in progress gives up the lock
func (s *sub) closeNotifications() {
	s.deliverMux.Lock()
	defer s.deliverMux.Unlock()
	s.closeNotificationsLocked()
}

func (s *sub) closeNotificationsLocked() {
	if !s.closed {
		s.closed = true
		close(s.notifications)
	}
}

func (rc *wsRPCClient) UnsubscribeAll(ctx context.Context) (lastErr ErrorRPC) {
	for _, s := range rc.getAllSubs() {
		if lastErr = s.Unsubscribe(ctx); lastErr != nil {
			log.L(ctx).Errorf("Failed to unsubscribe %s: %s", s.localID, lastErr)
		}
	}
	return lastErr
}

func (rc *wsRPCClient) Subscriptions() []Subscription {
	subs := rc.getAllSubs()
	iSubs := make([]Subscription, len(subs))
	for i, s := range subs {
		iSubs[i] = s
	}
	return iSubs
}

func (rc *wsRPCClient) sendRPC(ctx context.Context, reqID string, rpcReq *RPCRequest) ErrorRPC {
	jsonInput, err := json.Marshal(rpcReq)
	if err == nil {
		log.L(ctx).Debugf("RPC[%s] --> %s", reqID, rpcReq.Method)
		if log.IsTraceEnabled() {
			log.L(ctx).Tracef("RPC[%s] INPUT: %s", reqID, jsonInput)
		}
		err = rc.client.Send(ctx, jsonInput)
	}
	if err != nil {
		rpcErr := NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientRequestFailed, err)
		log.L(ctx).Errorf("RPC[%s] <-- ERROR: %s", reqID, err)
		return rpcErr
	}
	return nil
}

func (rc *wsRPCClient) CallRPC(ctx context.Context, result interface{}, method string, params ...interface{}) ErrorRPC {
	rpcReq, rpcErr := buildRequest(ctx, method, params)
	if rpcErr != nil {
		return rpcErr
	}

	reqID, resChannel := rc.addInflightRequest(rpcReq)
	defer rc.removeInflightRequest(reqID)

	rpcStartTime := time.Now()
	if rpcErr = rc.sendRPC(ctx, reqID, rpcReq); rpcErr != nil {
		return rpcErr
	}
	return rc.waitResponse(ctx, result, reqID, rpcReq, rpcStartTime, resChannel)
}

func (rc *wsRPCClient) waitResponse(ctx context.Context, result interface{}, reqID string, rpcReq *RPCRequest, rpcStartTime time.Time, resChannel chan *RPCResponse) ErrorRPC {
	var rpcRes *RPCResponse
	reqCtx, cancel := context.WithTimeout(ctx, rc.requestTimeout)
	defer cancel()
	select {
	case rpcRes = <-resChannel:
		// got response
	case <-reqCtx.Done():
		if ctx.Err() != nil {
			rpcErr := NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgContextCanceled, reqID)
			log.L(ctx).Errorf("RPC[%s] <-- ERROR: %s", reqID, rpcErr)
			return rpcErr
		}
		rpcErr := NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientRequestTimeout, reqID)
		log.L(ctx).Errorf("RPC[%s] <-- ERROR: %s (no response after %v)", reqID, rpcErr, rc.requestTimeout)
		return rpcErr
	}
	if rpcRes.Error != nil && rpcRes.Error.RPCError().Code != 0 {
		log.L(ctx).Errorf("RPC[%s] <-- ERROR: %s", reqID, rpcRes.Message())
		return rpcRes.Error
	}
	log.L(ctx).Infof("RPC[%s] <-- %s OK (%.2fms)", reqID, rpcReq.Method, float64(time.Since(rpcStartTime))/float64(time.Millisecond))
	if result != nil {
		if err := json.Unmarshal(rpcRes.Result.Bytes(), &result); err != nil {
			err = i18n.NewError(ctx, pldmsgs.MsgRPCClientResultParseFailed, result, err)
			return &RPCError{Code: int64(RPCCodeParseError), Message: err.Error()}
		}
	}
	return nil
}

func (rc *wsRPCClient) handleSubscriptionNotification(ctx context.Context, rpcRes *RPCResponse) {
	type rpcSubscriptionParams struct {
		Subscription string           `json:"subscription"` // probably hex, but not protocol assured
		Result       pldtypes.RawJSON `json:"result,omitempty"`
	}
	var subParams rpcSubscriptionParams
	if rpcRes.Params != nil {
		_ = json.Unmarshal(rpcRes.Params.Bytes(), &subParams)
	}
	if len(subParams.Subscription) == 0 {
		log.L(ctx).Warnf("RPC[%s] <-- Unable to extract subscription id from notification: %s", rpcRes.ID.StringValue(), rpcRes.Params)
		return
	}

	s := rc.getActiveSub(subParams.Subscription)
	if s == nil {
		log.L(ctx).Warnf("RPC[%s] <-- Notification for unknown subscription '%s'", rpcRes.ID.StringValue(), subParams.Subscription)
		return
	}

	// This is a notification that should match an active subscription
	log.L(ctx).Debugf("RPC[%s] <-- Notification for subscription %s (serverId=%s)", rpcRes.ID.StringValue(), s.localID, subParams.Subscription)
	// Held across the send so the channel cannot be closed while a send to it is pending
	s.deliverMux.Lock()
	defer s.deliverMux.Unlock()
	if s.closed {
		log.L(ctx).Warnf("RPC[%s] <-- Received subscription event after unsubscribe/close %s (serverId=%s)", rpcRes.ID.StringValue(), s.localID, subParams.Subscription)
		return
	}
	var deliveryTimeout <-chan time.Time
	if rc.subscriptionDeliveryTimeout > 0 {
		timer := time.NewTimer(rc.subscriptionDeliveryTimeout)
		defer timer.Stop()
		deliveryTimeout = timer.C
	}
	select {
	case s.notifications <- &rpcSubscriptionNotification{
		sub:          s,
		wsc:          rc,
		CurrentSubID: subParams.Subscription,
		Result:       subParams.Result,
	}:
	case <-s.ctx.Done():
		// The subscription has been unsubscribed, or we're closing
		log.L(ctx).Warnf("RPC[%s] <-- Received subscription event after unsubscribe/close %s (serverId=%s)", rpcRes.ID.StringValue(), s.localID, subParams.Subscription)
	case <-deliveryTimeout:
		rc.failSubscriptionLocked(ctx, s, NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientSubscriptionDeliveryTimeout, s.localID, rc.subscriptionDeliveryTimeout))
	}
}

// Runs on the receive loop, so must not wait for any RPC response
func (rc *wsRPCClient) failSubscriptionLocked(ctx context.Context, s *sub, rpcErr ErrorRPC) {
	log.L(ctx).Errorf("%s", rpcErr)
	currentSubID := rc.removeSubscription(s)
	rc.mux.Lock()
	s.err = rpcErr
	rc.mux.Unlock()
	s.closeNotificationsLocked()
	if currentSubID != "" {
		go rc.unsubscribeFailedSub(ctx, s, currentSubID)
	}
}

func (rc *wsRPCClient) unsubscribeFailedSub(ctx context.Context, s *sub, currentSubID string) {
	var resultBool bool
	if rpcErr := rc.CallRPC(ctx, &resultBool, s.UnsubscribeMethod, currentSubID); rpcErr != nil {
		log.L(ctx).Warnf("Failed to unsubscribe failed subscription %s (subid=%s): %s", s.localID, currentSubID, rpcErr)
		return
	}
	log.L(ctx).Infof("Unsubscribed failed subscription %s (subid=%s,result=%t)", s.localID, currentSubID, resultBool)
}

func (rc *wsRPCClient) handleSubscriptionConfirm(ctx context.Context, inflightSub *sub, rpcRes *RPCResponse) {
	resChl := inflightSub.newSubResponse
	inflightSub.newSubResponse = nil // we only dispatch once (it's only new once, on reconnect it's old and there's nobody to tell if we fail)
	if rpcRes.Error != nil && rpcRes.Error.RPCError().Code != 0 {
		log.L(ctx).Warnf("RPC[%s] <-- Error creating subscription %s (%s %v): %s", rpcRes.ID.StringValue(), inflightSub.localID, inflightSub.SubscribeMethod, inflightSub.params, rpcRes.Error)
		if resChl != nil {
			resChl <- rpcRes.Error
		} else {
			rc.retryResubscribe(ctx, inflightSub)
		}
		return
	}
	var subscriptionID string // we know it's probably hex, but we cannot rely on that being guaranteed
	if rpcRes.Result != nil {
		_ = json.Unmarshal(rpcRes.Result.Bytes(), &subscriptionID)
	}
	if len(subscriptionID) == 0 {
		log.L(ctx).Warnf("RPC[%s] <-- Unable to extract subscription id from eth_subscribe response: %s", rpcRes.ID.StringValue(), rpcRes.Result)
		if resChl != nil {
			resChl <- NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientSubscribeResponseInvalid)
		} else {
			rc.retryResubscribe(ctx, inflightSub)
		}
		return
	}
	log.L(ctx).Infof("Subscribed %s with server subscription ID '%s'", inflightSub.localID, subscriptionID)
	rc.addActiveSub(inflightSub, subscriptionID)
	// all was good, if someone is waiting to be told, notify them
	if resChl != nil {
		resChl <- nil
	}
}

// A single retry loop owns a subscription's recovery, however many resubscribes are rejected while it runs
func (rc *wsRPCClient) retryResubscribe(ctx context.Context, s *sub) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	if !s.resubscribing {
		s.resubscribing = true
		go rc.resubscribeLoop(ctx, s)
	}
}

// Sends without waiting for the response: a rejection comes back through handleSubscriptionConfirm,
// and the loop tries again after the next delay unless the subscription has become active
func (rc *wsRPCClient) resubscribeLoop(ctx context.Context, s *sub) {
	loopCtx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()

	rpcReq, rpcErr := buildRequest(ctx, s.SubscribeMethod, s.params)
	if rpcErr != nil {
		log.L(ctx).Errorf("Unable to retry subscribe %s: %s", s.localID, rpcErr)
		return
	}
	r := retry.NewRetryIndefinite(&rc.wsConf.ConnectRetry)
	for failCount := 1; ; failCount++ {
		if r.WaitDelay(loopCtx, failCount) != nil {
			return
		}
		reqID, done := rc.nextResubscribeAttempt(s)
		if done {
			return
		}
		if reqID == "" {
			continue
		}
		log.L(ctx).Infof("Retrying subscribe %s (attempt=%d)", s.localID, failCount)
		rpcReq.ID = pldtypes.RawJSON(`"` + reqID + `"`)
		if rpcErr := rc.sendRPC(loopCtx, reqID, rpcReq); rpcErr != nil {
			rc.removeInflightSub(s, reqID)
		}
	}
}

// Returns no request ID while a subscribe is already in flight, whether from this loop or from a reconnect
func (rc *wsRPCClient) nextResubscribeAttempt(s *sub) (reqID string, done bool) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	if rc.configuredSubs[s.localID] == nil || s.currentSubID != "" {
		s.resubscribing = false
		return "", true
	}
	if s.pendingReqID != "" {
		return "", false
	}
	return rc.addInflightSubLocked(s), false
}

func (rc *wsRPCClient) removeInflightSub(s *sub, reqID string) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	delete(rc.pendingSubsByReqID, reqID)
	if s.pendingReqID == reqID {
		s.pendingReqID = ""
	}
}

func (rc *wsRPCClient) deliverCallResponse(inflightCall chan *RPCResponse, rpcRes *RPCResponse) {
	select {
	case inflightCall <- rpcRes:
	default:
		// only considered for the very edge case of reconnect - the inflight response should only be
		// in the map until it's sent a single response, and there's a slot to ensure it never blocks
	}
}

func (rc *wsRPCClient) isNotificationMethod(method string) bool {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	return rc.notificationMethods[method]
}

func (rc *wsRPCClient) receiveLoop(ctx context.Context) {
	// Cancelled when the loop exits, which stops any resubscribe retries it started
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		bytes, ok := <-rc.client.Receive()
		if !ok {
			log.L(ctx).Debugf("WebSocket closed")
			return
		}
		rpcRes := RPCResponse{}
		err := json.Unmarshal(bytes, &rpcRes)
		switch {
		case err != nil:
			log.L(ctx).Errorf("RPC <-- ERROR invalid data '%s': %s", bytes, err)
		case rc.isNotificationMethod(rpcRes.Method):
			rc.handleSubscriptionNotification(ctx, &rpcRes)
		default:
			// ID should match a request we sent
			inflightSub, inflightCall := rc.popInflight(rpcRes.ID.StringValue())
			switch {
			case inflightSub != nil:
				rc.handleSubscriptionConfirm(ctx, inflightSub, &rpcRes)
			case inflightCall != nil:
				rc.deliverCallResponse(inflightCall, &rpcRes)
			default:
				log.L(ctx).Warnf("RPC[%s] <-- Received unexpected RPC response: %+v", rpcRes.ID.StringValue(), rpcRes)
			}
		}
	}
}
