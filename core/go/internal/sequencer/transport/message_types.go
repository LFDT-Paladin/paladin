/*
 * Copyright © 2026 Kaleido, Inc.
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

package transport

const (
	MessageType_AssembleRequest                  = "AssembleRequest"
	MessageType_AssembleResponse                 = "AssembleResponse"
	MessageType_AssembleError                    = "AssembleError"
	MessageType_AssembleRejection                = "AssembleRejection"
	MessageType_CoordinatorHeartbeatNotification = "CoordinatorHeartbeatNotification"
	MessageType_DelegationRequest                = "DelegationRequest"
	MessageType_DelegationResponse               = "DelegationResponse"
	MessageType_DelegationRejection              = "DelegationRejection"
	MessageType_Dispatched                       = "Dispatched"
	MessageType_EndorsementRequest               = "EndorsementRequest"
	MessageType_EndorsementResponse              = "EndorsementResponse"
	MessageType_EndorsementError                 = "EndorsementError"
	MessageType_EndorsementRejection             = "EndorsementRejection"
	MessageType_HandoverRequest                  = "HandoverRequest"
	MessageType_NonceAssigned                    = "NonceAssigned"
	MessageType_PreDispatchRequest               = "PreDispatchRequest"
	MessageType_PreDispatchResponse              = "PreDispatchResponse"
	MessageType_PreDispatchRejection             = "PreDispatchRejection"
	MessageType_TransactionRequest               = "TransactionRequest"
	MessageType_TransactionSubmitted             = "TransactionSubmitted"
	MessageType_TransactionConfirmed             = "TransactionConfirmed"
)
