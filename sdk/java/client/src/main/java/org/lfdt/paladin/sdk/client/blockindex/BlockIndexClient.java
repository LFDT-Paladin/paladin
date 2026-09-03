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
package org.lfdt.paladin.sdk.client.blockindex;

import com.fasterxml.jackson.core.type.TypeReference;
import java.util.List;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.blockindex.EventWithData;
import org.lfdt.paladin.sdk.core.blockindex.IndexedBlock;
import org.lfdt.paladin.sdk.core.blockindex.IndexedEvent;
import org.lfdt.paladin.sdk.core.blockindex.IndexedTransaction;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexUint64;

/**
 * Client for the {@code bidx_*} RPC namespace (block index).
 *
 * <p>Each method maps one-to-one to a JSON-RPC call on the underlying {@link RpcClient} and returns
 * a {@link CompletableFuture}; failures complete it exceptionally with a {@code PaladinException}
 * subtype.
 */
public final class BlockIndexClient {

  private final RpcClient rpc;

  /**
   * Creates a client over the given RPC transport.
   *
   * @param rpc the RPC client used to make calls; must not be {@code null}
   */
  public BlockIndexClient(final RpcClient rpc) {
    this.rpc = Objects.requireNonNull(rpc, "rpc");
  }

  /**
   * Returns an indexed block by its number ({@code bidx_getBlockByNumber}).
   *
   * @param blockNumber the block number to look up
   * @return a future completing with the block, or {@code null} if it is not indexed
   */
  public CompletableFuture<IndexedBlock> getBlockByNumber(final HexUint64 blockNumber) {
    return rpc.callRpc(IndexedBlock.class, "bidx_getBlockByNumber", blockNumber);
  }

  /**
   * Returns an indexed transaction by its hash ({@code bidx_getTransactionByHash}).
   *
   * @param transactionHash the transaction hash to look up
   * @return a future completing with the transaction, or {@code null} if it is not indexed
   */
  public CompletableFuture<IndexedTransaction> getTransactionByHash(final Bytes32 transactionHash) {
    return rpc.callRpc(IndexedTransaction.class, "bidx_getTransactionByHash", transactionHash);
  }

  /**
   * Returns an indexed transaction by its sender and nonce ({@code bidx_getTransactionByNonce}).
   *
   * @param from the sender address
   * @param nonce the sender's transaction nonce
   * @return a future completing with the transaction, or {@code null} if it is not indexed
   */
  public CompletableFuture<IndexedTransaction> getTransactionByNonce(
      final EthAddress from, final HexUint64 nonce) {
    return rpc.callRpc(IndexedTransaction.class, "bidx_getTransactionByNonce", from, nonce);
  }

  /**
   * Lists the transactions in a block by block number ({@code bidx_getBlockTransactionsByNumber}).
   *
   * @param blockNumber the block number whose transactions to list
   * @return a future completing with the block's transactions
   */
  public CompletableFuture<List<IndexedTransaction>> getBlockTransactionsByNumber(
      final HexUint64 blockNumber) {
    return rpc.callRpc(
        new TypeReference<List<IndexedTransaction>>() {},
        "bidx_getBlockTransactionsByNumber",
        blockNumber);
  }

  /**
   * Lists the events emitted by a transaction by its hash ({@code
   * bidx_getTransactionEventsByHash}).
   *
   * @param transactionHash the transaction hash whose events to list
   * @return a future completing with the transaction's events
   */
  public CompletableFuture<List<IndexedEvent>> getTransactionEventsByHash(
      final Bytes32 transactionHash) {
    return rpc.callRpc(
        new TypeReference<List<IndexedEvent>>() {},
        "bidx_getTransactionEventsByHash",
        transactionHash);
  }

  /**
   * Queries indexed blocks ({@code bidx_queryIndexedBlocks}).
   *
   * @param query the query to run
   * @return a future completing with the matching blocks
   */
  public CompletableFuture<List<IndexedBlock>> queryIndexedBlocks(final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<IndexedBlock>>() {}, "bidx_queryIndexedBlocks", query);
  }

  /**
   * Queries indexed transactions ({@code bidx_queryIndexedTransactions}).
   *
   * @param query the query to run
   * @return a future completing with the matching transactions
   */
  public CompletableFuture<List<IndexedTransaction>> queryIndexedTransactions(
      final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<IndexedTransaction>>() {}, "bidx_queryIndexedTransactions", query);
  }

  /**
   * Queries indexed events ({@code bidx_queryIndexedEvents}).
   *
   * @param query the query to run
   * @return a future completing with the matching events
   */
  public CompletableFuture<List<IndexedEvent>> queryIndexedEvents(final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<IndexedEvent>>() {}, "bidx_queryIndexedEvents", query);
  }

  /**
   * Returns the height of the highest confirmed block ({@code bidx_getConfirmedBlockHeight}).
   *
   * @return a future completing with the confirmed block height
   */
  public CompletableFuture<HexUint64> getConfirmedBlockHeight() {
    return rpc.callRpc(HexUint64.class, "bidx_getConfirmedBlockHeight");
  }

  /**
   * Decodes the events emitted by a transaction against a supplied ABI ({@code
   * bidx_decodeTransactionEvents}).
   *
   * @param transactionHash the transaction whose events to decode
   * @param abi the ABI to decode the events against
   * @param resultFormat the requested output data format
   * @return a future completing with the decoded events
   */
  public CompletableFuture<List<EventWithData>> decodeTransactionEvents(
      final Bytes32 transactionHash, final List<AbiEntry> abi, final String resultFormat) {
    return rpc.callRpc(
        new TypeReference<List<EventWithData>>() {},
        "bidx_decodeTransactionEvents",
        transactionHash,
        abi,
        resultFormat);
  }
}
