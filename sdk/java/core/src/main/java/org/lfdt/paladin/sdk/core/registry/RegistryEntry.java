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
package org.lfdt.paladin.sdk.core.registry;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.types.HexBytes;

/**
 * An entry (entity) within a registry, together with its current on-chain provenance and active
 * state.
 *
 * <p>The on-chain location fields ({@link #blockNumber()}, {@link #transactionIndex()}, {@link
 * #logIndex()}) are present only for registries that use blockchain indexing, and {@link #active()}
 * is populated only by queries that explicitly look for inactive entries; otherwise they are {@code
 * null}. Immutable.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
@JsonPropertyOrder({
  "registry",
  "id",
  "name",
  "parentId",
  "blockNumber",
  "transactionIndex",
  "logIndex",
  "active"
})
public final class RegistryEntry {

  private final String registry;
  private final HexBytes id;
  private final String name;
  private final HexBytes parentId;
  private final Long blockNumber;
  private final Long transactionIndex;
  private final Long logIndex;
  private final Boolean active;

  @JsonCreator
  RegistryEntry(
      @JsonProperty("registry") final String registry,
      @JsonProperty("id") final HexBytes id,
      @JsonProperty("name") final String name,
      @JsonProperty("parentId") final HexBytes parentId,
      @JsonProperty("blockNumber") final Long blockNumber,
      @JsonProperty("transactionIndex") final Long transactionIndex,
      @JsonProperty("logIndex") final Long logIndex,
      @JsonProperty("active") final Boolean active) {
    this.registry = registry;
    this.id = id;
    this.name = name;
    this.parentId = parentId;
    this.blockNumber = blockNumber;
    this.transactionIndex = transactionIndex;
    this.logIndex = logIndex;
    this.active = active;
  }

  /**
   * The name of the registry that maintains this entry.
   *
   * @return the registry name
   */
  @JsonProperty("registry")
  public String registry() {
    return registry;
  }

  /**
   * The identifier of this entry, unique within the registry across the whole entry hierarchy.
   *
   * @return the entry identifier
   */
  @JsonProperty("id")
  public HexBytes id() {
    return id;
  }

  /**
   * The name of this entry, unique among entries that share the same parent within the registry.
   *
   * @return the entry name
   */
  @JsonProperty("name")
  public String name() {
    return name;
  }

  /**
   * The identifier of this entry's parent, or {@code null} for a root entry.
   *
   * @return the parent entry identifier, or {@code null}
   */
  @JsonProperty("parentId")
  public HexBytes parentId() {
    return parentId;
  }

  /**
   * The block number at which this entry was recorded on chain, or {@code null} if the registry
   * does not use blockchain indexing.
   *
   * @return the block number, or {@code null}
   */
  @JsonProperty("blockNumber")
  public Long blockNumber() {
    return blockNumber;
  }

  /**
   * The index of the transaction within its block, or {@code null} if the registry does not use
   * blockchain indexing.
   *
   * @return the transaction index, or {@code null}
   */
  @JsonProperty("transactionIndex")
  public Long transactionIndex() {
    return transactionIndex;
  }

  /**
   * The index of the log within its transaction, or {@code null} if the registry does not use
   * blockchain indexing.
   *
   * @return the log index, or {@code null}
   */
  @JsonProperty("logIndex")
  public Long logIndex() {
    return logIndex;
  }

  /**
   * Whether this entry is currently active, populated only by queries that explicitly look for
   * inactive entries; otherwise {@code null}.
   *
   * @return the active flag, or {@code null}
   */
  @JsonProperty("active")
  public Boolean active() {
    return active;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof RegistryEntry other
        && Objects.equals(registry, other.registry)
        && Objects.equals(id, other.id)
        && Objects.equals(name, other.name)
        && Objects.equals(parentId, other.parentId)
        && Objects.equals(blockNumber, other.blockNumber)
        && Objects.equals(transactionIndex, other.transactionIndex)
        && Objects.equals(logIndex, other.logIndex)
        && Objects.equals(active, other.active);
  }

  @Override
  public int hashCode() {
    return Objects.hash(
        registry, id, name, parentId, blockNumber, transactionIndex, logIndex, active);
  }

  @Override
  public String toString() {
    return "RegistryEntry{registry=" + registry + ", id=" + id + ", name=" + name + "}";
  }
}
