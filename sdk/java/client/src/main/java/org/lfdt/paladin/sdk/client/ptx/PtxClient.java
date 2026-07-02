/*
 * Copyright contributors to Paladin, an LFDT project
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

package org.lfdt.paladin.sdk.client.ptx;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.core.abi.ABIDecodedData;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.StoredABI;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.transaction.BlockchainEventListener;
import org.lfdt.paladin.sdk.core.transaction.BlockchainEventListenerStatus;
import org.lfdt.paladin.sdk.core.transaction.ChainedDispatch;
import org.lfdt.paladin.sdk.core.transaction.Dispatch;
import org.lfdt.paladin.sdk.core.transaction.PreparedTransaction;
import org.lfdt.paladin.sdk.core.transaction.PublicTxWithBinding;
import org.lfdt.paladin.sdk.core.transaction.Transaction;
import org.lfdt.paladin.sdk.core.transaction.TransactionCall;
import org.lfdt.paladin.sdk.core.transaction.TransactionFull;
import org.lfdt.paladin.sdk.core.transaction.TransactionInput;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceiptFull;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceiptListener;
import org.lfdt.paladin.sdk.core.transaction.TransactionStates;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.HexBytes;

// Client for the ptx_* RPC namespace (private transaction manager), mirroring Go's pldclient.PTX
// (sdk/go/pkg/pldclient/ptx.go). Each method maps one-to-one to a JSON-RPC call on the underlying
// RpcClient and returns a CompletableFuture; failures complete it exceptionally with a
// PaladinException subtype. This covers the full HTTP method surface: the transaction lifecycle
// (send/prepare/update/call, get and query), receipts, state, prepared and public transactions,
// stored ABIs, decode, verifier resolution, receipt and blockchain-event listeners, and dispatch
// queries. The WebSocket-only subscribe/unsubscribe methods are added with the WS transport.
public final class PtxClient {

  private final RpcClient rpc;

  public PtxClient(RpcClient rpc) {
    this.rpc = Objects.requireNonNull(rpc, "rpc");
  }

  public CompletableFuture<UUID> sendTransaction(TransactionInput transaction) {
    return rpc.callRpc(UUID.class, "ptx_sendTransaction", transaction);
  }

  public CompletableFuture<List<UUID>> sendTransactions(List<TransactionInput> transactions) {
    return rpc.callRpc(new TypeReference<List<UUID>>() {}, "ptx_sendTransactions", transactions);
  }

  public CompletableFuture<UUID> prepareTransaction(TransactionInput transaction) {
    return rpc.callRpc(UUID.class, "ptx_prepareTransaction", transaction);
  }

  public CompletableFuture<List<UUID>> prepareTransactions(List<TransactionInput> transactions) {
    return rpc.callRpc(
        new TypeReference<List<UUID>>() {}, "ptx_prepareTransactions", transactions);
  }

  public CompletableFuture<UUID> updateTransaction(UUID id, TransactionInput transaction) {
    return rpc.callRpc(UUID.class, "ptx_updateTransaction", id, transaction);
  }

  public CompletableFuture<JsonNode> call(TransactionCall transaction) {
    return rpc.callRpc(JsonNode.class, "ptx_call", transaction);
  }

  public CompletableFuture<Transaction> getTransaction(UUID id) {
    return rpc.callRpc(Transaction.class, "ptx_getTransaction", id);
  }

  public CompletableFuture<TransactionFull> getTransactionFull(UUID id) {
    return rpc.callRpc(TransactionFull.class, "ptx_getTransactionFull", id);
  }

  public CompletableFuture<Transaction> getTransactionByIdempotencyKey(String idempotencyKey) {
    return rpc.callRpc(
        Transaction.class, "ptx_getTransactionByIdempotencyKey", idempotencyKey);
  }

  public CompletableFuture<List<Transaction>> queryTransactions(QueryJSON query) {
    return rpc.callRpc(new TypeReference<List<Transaction>>() {}, "ptx_queryTransactions", query);
  }

  public CompletableFuture<List<TransactionFull>> queryTransactionsFull(QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<TransactionFull>>() {}, "ptx_queryTransactionsFull", query);
  }

  public CompletableFuture<TransactionReceipt> getTransactionReceipt(UUID id) {
    return rpc.callRpc(TransactionReceipt.class, "ptx_getTransactionReceipt", id);
  }

  public CompletableFuture<TransactionReceiptFull> getTransactionReceiptFull(UUID id) {
    return rpc.callRpc(TransactionReceiptFull.class, "ptx_getTransactionReceiptFull", id);
  }

  public CompletableFuture<JsonNode> getDomainReceipt(String domain, UUID id) {
    return rpc.callRpc(JsonNode.class, "ptx_getDomainReceipt", domain, id);
  }

  public CompletableFuture<TransactionStates> getStateReceipt(UUID id) {
    return rpc.callRpc(TransactionStates.class, "ptx_getStateReceipt", id);
  }

  public CompletableFuture<List<TransactionReceipt>> queryTransactionReceipts(QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<TransactionReceipt>>() {}, "ptx_queryTransactionReceipts", query);
  }

  public CompletableFuture<PreparedTransaction> getPreparedTransaction(UUID id) {
    return rpc.callRpc(PreparedTransaction.class, "ptx_getPreparedTransaction", id);
  }

  public CompletableFuture<List<PreparedTransaction>> queryPreparedTransactions(QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<PreparedTransaction>>() {}, "ptx_queryPreparedTransactions", query);
  }

  public CompletableFuture<PublicTxWithBinding> getPublicTransaction(long id) {
    return rpc.callRpc(PublicTxWithBinding.class, "ptx_getPublicTransaction", id);
  }

  public CompletableFuture<Bytes32> storeABI(List<AbiEntry> abi) {
    return rpc.callRpc(Bytes32.class, "ptx_storeABI", abi);
  }

  public CompletableFuture<StoredABI> getStoredABI(Bytes32 hashRef) {
    return rpc.callRpc(StoredABI.class, "ptx_getStoredABI", hashRef);
  }

  public CompletableFuture<List<StoredABI>> queryStoredABIs(QueryJSON query) {
    return rpc.callRpc(new TypeReference<List<StoredABI>>() {}, "ptx_queryStoredABIs", query);
  }

  public CompletableFuture<ABIDecodedData> decodeError(HexBytes revertData, String dataFormat) {
    return rpc.callRpc(ABIDecodedData.class, "ptx_decodeError", revertData, dataFormat);
  }

  public CompletableFuture<ABIDecodedData> decodeCall(HexBytes callData, String dataFormat) {
    return rpc.callRpc(ABIDecodedData.class, "ptx_decodeCall", callData, dataFormat);
  }

  public CompletableFuture<ABIDecodedData> decodeEvent(
      List<Bytes32> topics, HexBytes data, String dataFormat) {
    return rpc.callRpc(ABIDecodedData.class, "ptx_decodeEvent", topics, data, dataFormat);
  }

  public CompletableFuture<String> resolveVerifier(
      String keyIdentifier, String algorithm, String verifierType) {
    return rpc.callRpc(
        String.class, "ptx_resolveVerifier", keyIdentifier, algorithm, verifierType);
  }

  public CompletableFuture<Boolean> createReceiptListener(TransactionReceiptListener listener) {
    return rpc.callRpc(Boolean.class, "ptx_createReceiptListener", listener);
  }

  public CompletableFuture<List<TransactionReceiptListener>> queryReceiptListeners(QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<TransactionReceiptListener>>() {}, "ptx_queryReceiptListeners", query);
  }

  public CompletableFuture<TransactionReceiptListener> getReceiptListener(String listenerName) {
    return rpc.callRpc(TransactionReceiptListener.class, "ptx_getReceiptListener", listenerName);
  }

  public CompletableFuture<Boolean> startReceiptListener(String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_startReceiptListener", listenerName);
  }

  public CompletableFuture<Boolean> stopReceiptListener(String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_stopReceiptListener", listenerName);
  }

  public CompletableFuture<Boolean> deleteReceiptListener(String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_deleteReceiptListener", listenerName);
  }

  public CompletableFuture<Boolean> createBlockchainEventListener(BlockchainEventListener listener) {
    return rpc.callRpc(Boolean.class, "ptx_createBlockchainEventListener", listener);
  }

  public CompletableFuture<List<BlockchainEventListener>> queryBlockchainEventListeners(
      QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<BlockchainEventListener>>() {},
        "ptx_queryBlockchainEventListeners",
        query);
  }

  public CompletableFuture<BlockchainEventListener> getBlockchainEventListener(String listenerName) {
    return rpc.callRpc(
        BlockchainEventListener.class, "ptx_getBlockchainEventListener", listenerName);
  }

  public CompletableFuture<Boolean> startBlockchainEventListener(String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_startBlockchainEventListener", listenerName);
  }

  public CompletableFuture<Boolean> stopBlockchainEventListener(String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_stopBlockchainEventListener", listenerName);
  }

  public CompletableFuture<Boolean> deleteBlockchainEventListener(String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_deleteBlockchainEventListener", listenerName);
  }

  public CompletableFuture<BlockchainEventListenerStatus> getBlockchainEventListenerStatus(
      String listenerName) {
    return rpc.callRpc(
        BlockchainEventListenerStatus.class, "ptx_getBlockchainEventListenerStatus", listenerName);
  }

  public CompletableFuture<List<Dispatch>> queryDispatches(QueryJSON query) {
    return rpc.callRpc(new TypeReference<List<Dispatch>>() {}, "ptx_queryDispatches", query);
  }

  public CompletableFuture<Dispatch> getDispatch(String id) {
    return rpc.callRpc(Dispatch.class, "ptx_getDispatch", id);
  }

  public CompletableFuture<List<ChainedDispatch>> queryChainedDispatches(QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<ChainedDispatch>>() {}, "ptx_queryChainedDispatches", query);
  }

  public CompletableFuture<ChainedDispatch> getChainedDispatch(String id) {
    return rpc.callRpc(ChainedDispatch.class, "ptx_getChainedDispatch", id);
  }
}
