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
package org.lfdt.paladin.sdk.client.registry;

import com.fasterxml.jackson.core.type.TypeReference;
import java.util.List;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.registry.ActiveFilter;
import org.lfdt.paladin.sdk.core.registry.RegistryEntry;
import org.lfdt.paladin.sdk.core.registry.RegistryEntryWithProperties;
import org.lfdt.paladin.sdk.core.registry.RegistryProperty;
import org.lfdt.paladin.sdk.core.types.HexBytes;

/**
 * Client for the {@code reg_*} RPC namespace (registry).
 *
 * <p>Each method maps one-to-one to a JSON-RPC call on the underlying {@link RpcClient} and returns
 * a {@link CompletableFuture}; failures complete it exceptionally with a {@code PaladinException}
 * subtype.
 */
public final class RegistryClient {

  private final RpcClient rpc;

  /**
   * Creates a client over the given RPC transport.
   *
   * @param rpc the RPC client used to make calls; must not be {@code null}
   */
  public RegistryClient(final RpcClient rpc) {
    this.rpc = Objects.requireNonNull(rpc, "rpc");
  }

  /**
   * Lists the names of the registries configured on the node ({@code reg_registries}).
   *
   * @return a future completing with the registry names
   */
  public CompletableFuture<List<String>> registries() {
    return rpc.callRpc(new TypeReference<List<String>>() {}, "reg_registries");
  }

  /**
   * Queries the entries of a registry ({@code reg_queryEntries}).
   *
   * @param registryName the registry to query
   * @param query the query to run
   * @param activeFilter whether to return active, inactive, or all entries
   * @return a future completing with the matching entries
   */
  public CompletableFuture<List<RegistryEntry>> queryEntries(
      final String registryName, final QueryJSON query, final ActiveFilter activeFilter) {
    return rpc.callRpc(
        new TypeReference<List<RegistryEntry>>() {},
        "reg_queryEntries",
        registryName,
        query,
        activeFilter);
  }

  /**
   * Queries the entries of a registry, each with its properties flattened in ({@code
   * reg_queryEntriesWithProps}).
   *
   * @param registryName the registry to query
   * @param query the query to run
   * @param activeFilter whether to return active, inactive, or all entries
   * @return a future completing with the matching entries and their properties
   */
  public CompletableFuture<List<RegistryEntryWithProperties>> queryEntriesWithProps(
      final String registryName, final QueryJSON query, final ActiveFilter activeFilter) {
    return rpc.callRpc(
        new TypeReference<List<RegistryEntryWithProperties>>() {},
        "reg_queryEntriesWithProps",
        registryName,
        query,
        activeFilter);
  }

  /**
   * Lists the properties of a single registry entry, with provenance ({@code
   * reg_getEntryProperties}).
   *
   * @param registryName the registry that holds the entry
   * @param entryId the identifier of the entry whose properties to list
   * @param activeFilter whether to return active, inactive, or all properties
   * @return a future completing with the entry's properties
   */
  public CompletableFuture<List<RegistryProperty>> getEntryProperties(
      final String registryName, final HexBytes entryId, final ActiveFilter activeFilter) {
    return rpc.callRpc(
        new TypeReference<List<RegistryProperty>>() {},
        "reg_getEntryProperties",
        registryName,
        entryId,
        activeFilter);
  }
}
