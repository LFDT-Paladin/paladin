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
package org.lfdt.paladin.sdk.client.statestore;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.statestore.Schema;
import org.lfdt.paladin.sdk.core.statestore.State;
import org.lfdt.paladin.sdk.core.statestore.StateStatusQualifier;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

/**
 * Client for the {@code pstate_*} RPC namespace (state store).
 *
 * <p>Each method maps one-to-one to a JSON-RPC call on the underlying {@link RpcClient} and returns
 * a {@link CompletableFuture}; failures complete it exceptionally with a {@code PaladinException}
 * subtype.
 */
public final class StateStoreClient {

  private final RpcClient rpc;

  /**
   * Creates a client over the given RPC transport.
   *
   * @param rpc the RPC client used to make calls; must not be {@code null}
   */
  public StateStoreClient(final RpcClient rpc) {
    this.rpc = Objects.requireNonNull(rpc, "rpc");
  }

  /**
   * Lists the schemas registered by a domain ({@code pstate_listSchemas}).
   *
   * @param domain the domain whose schemas to list
   * @return a future completing with the domain's schemas
   */
  public CompletableFuture<List<Schema>> listSchemas(final String domain) {
    return rpc.callRpc(new TypeReference<List<Schema>>() {}, "pstate_listSchemas", domain);
  }

  /**
   * Stores a state directly in the state store ({@code pstate_storeState}).
   *
   * @param domain the domain that owns the state
   * @param contractAddress the contract the state belongs to
   * @param schemaRef the schema the state conforms to
   * @param data the state's private data
   * @return a future completing with the stored state
   */
  public CompletableFuture<State> storeState(
      final String domain,
      final EthAddress contractAddress,
      final Bytes32 schemaRef,
      final JsonNode data) {
    return rpc.callRpc(State.class, "pstate_storeState", domain, contractAddress, schemaRef, data);
  }

  /**
   * Queries states of a schema across all contracts of a domain ({@code pstate_queryStates}).
   *
   * @param domain the domain to query
   * @param schemaRef the schema whose states to query
   * @param query the query to run
   * @param qualifier which states to include (a standard qualifier or a transaction id)
   * @return a future completing with the matching states
   */
  public CompletableFuture<List<State>> queryStates(
      final String domain,
      final Bytes32 schemaRef,
      final QueryJSON query,
      final StateStatusQualifier qualifier) {
    return rpc.callRpc(
        new TypeReference<List<State>>() {},
        "pstate_queryStates",
        domain,
        schemaRef,
        query,
        qualifier);
  }

  /**
   * Queries states of a schema within a single contract ({@code pstate_queryContractStates}).
   *
   * @param domain the domain to query
   * @param contractAddress the contract whose states to query
   * @param schemaRef the schema whose states to query
   * @param query the query to run
   * @param qualifier which states to include (a standard qualifier or a transaction id)
   * @return a future completing with the matching states
   */
  public CompletableFuture<List<State>> queryContractStates(
      final String domain,
      final EthAddress contractAddress,
      final Bytes32 schemaRef,
      final QueryJSON query,
      final StateStatusQualifier qualifier) {
    return rpc.callRpc(
        new TypeReference<List<State>>() {},
        "pstate_queryContractStates",
        domain,
        contractAddress,
        schemaRef,
        query,
        qualifier);
  }

  /**
   * Queries states by nullifier across all contracts of a domain ({@code pstate_queryNullifiers}).
   *
   * @param domain the domain to query
   * @param schemaRef the schema whose states to query
   * @param query the query to run
   * @param qualifier which states to include (a standard qualifier or a transaction id)
   * @return a future completing with the matching states
   */
  public CompletableFuture<List<State>> queryNullifiers(
      final String domain,
      final Bytes32 schemaRef,
      final QueryJSON query,
      final StateStatusQualifier qualifier) {
    return rpc.callRpc(
        new TypeReference<List<State>>() {},
        "pstate_queryNullifiers",
        domain,
        schemaRef,
        query,
        qualifier);
  }

  /**
   * Queries states by nullifier within a single contract ({@code pstate_queryContractNullifiers}).
   *
   * @param domain the domain to query
   * @param contractAddress the contract whose states to query
   * @param schemaRef the schema whose states to query
   * @param query the query to run
   * @param qualifier which states to include (a standard qualifier or a transaction id)
   * @return a future completing with the matching states
   */
  public CompletableFuture<List<State>> queryContractNullifiers(
      final String domain,
      final EthAddress contractAddress,
      final Bytes32 schemaRef,
      final QueryJSON query,
      final StateStatusQualifier qualifier) {
    return rpc.callRpc(
        new TypeReference<List<State>>() {},
        "pstate_queryContractNullifiers",
        domain,
        contractAddress,
        schemaRef,
        query,
        qualifier);
  }

  /**
   * Transfers a private state to another node for reliable delivery ({@code
   * pstate_transferPrivateState}).
   *
   * @param domain the domain that owns the state
   * @param stateId the id of the state to transfer
   * @param recipient the identity locator of the recipient
   * @return a future completing with the id of the reliable message created for the transfer
   */
  public CompletableFuture<UUID> transferPrivateState(
      final String domain, final HexBytes stateId, final String recipient) {
    return rpc.callRpc(UUID.class, "pstate_transferPrivateState", domain, stateId, recipient);
  }
}
