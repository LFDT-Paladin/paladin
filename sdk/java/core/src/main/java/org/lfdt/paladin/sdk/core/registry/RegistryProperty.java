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
 * A single name/value property of a registry entry, together with its on-chain provenance and
 * active state.
 *
 * <p>As with {@link RegistryEntry}, the on-chain location fields are present only for registries
 * that use blockchain indexing and {@link #active()} only for queries that look for inactive
 * entries; otherwise they are {@code null}. Immutable.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
@JsonPropertyOrder({
  "registry",
  "entryId",
  "name",
  "value",
  "blockNumber",
  "transactionIndex",
  "logIndex",
  "active"
})
public final class RegistryProperty {

  private final String registry;
  private final HexBytes entryId;
  private final String name;
  private final String value;
  private final Long blockNumber;
  private final Long transactionIndex;
  private final Long logIndex;
  private final Boolean active;

  @JsonCreator
  RegistryProperty(
      @JsonProperty("registry") final String registry,
      @JsonProperty("entryId") final HexBytes entryId,
      @JsonProperty("name") final String name,
      @JsonProperty("value") final String value,
      @JsonProperty("blockNumber") final Long blockNumber,
      @JsonProperty("transactionIndex") final Long transactionIndex,
      @JsonProperty("logIndex") final Long logIndex,
      @JsonProperty("active") final Boolean active) {
    this.registry = registry;
    this.entryId = entryId;
    this.name = name;
    this.value = value;
    this.blockNumber = blockNumber;
    this.transactionIndex = transactionIndex;
    this.logIndex = logIndex;
    this.active = active;
  }

  /**
   * The name of the registry that maintains this property.
   *
   * @return the registry name
   */
  @JsonProperty("registry")
  public String registry() {
    return registry;
  }

  /**
   * The identifier of the entry that owns this property.
   *
   * @return the owning entry identifier
   */
  @JsonProperty("entryId")
  public HexBytes entryId() {
    return entryId;
  }

  /**
   * The name of this property.
   *
   * @return the property name
   */
  @JsonProperty("name")
  public String name() {
    return name;
  }

  /**
   * The value of this property.
   *
   * @return the property value
   */
  @JsonProperty("value")
  public String value() {
    return value;
  }

  /**
   * The block number at which this property was recorded on chain, or {@code null} if the registry
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
   * Whether the owning entry is currently active, populated only by queries that explicitly look
   * for inactive entries; otherwise {@code null}.
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
    return o instanceof RegistryProperty other
        && Objects.equals(registry, other.registry)
        && Objects.equals(entryId, other.entryId)
        && Objects.equals(name, other.name)
        && Objects.equals(value, other.value)
        && Objects.equals(blockNumber, other.blockNumber)
        && Objects.equals(transactionIndex, other.transactionIndex)
        && Objects.equals(logIndex, other.logIndex)
        && Objects.equals(active, other.active);
  }

  @Override
  public int hashCode() {
    return Objects.hash(
        registry, entryId, name, value, blockNumber, transactionIndex, logIndex, active);
  }

  @Override
  public String toString() {
    return "RegistryProperty{registry="
        + registry
        + ", entryId="
        + entryId
        + ", name="
        + name
        + ", value="
        + value
        + "}";
  }
}
