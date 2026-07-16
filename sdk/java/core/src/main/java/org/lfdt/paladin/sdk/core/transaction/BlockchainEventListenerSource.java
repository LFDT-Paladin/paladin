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
package org.lfdt.paladin.sdk.core.transaction;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.types.EthAddress;

/**
 * An ABI, optionally scoped to a single contract address, that a blockchain-event listener matches
 * against. Immutable; build one with the {@linkplain #builder() fluent builder} to configure a
 * listener.
 */
@JsonPropertyOrder({"abi", "address"})
public final class BlockchainEventListenerSource {

  private final List<AbiEntry> abi;
  private final EthAddress address;

  @JsonCreator
  BlockchainEventListenerSource(
      @JsonProperty("abi") final List<AbiEntry> abi,
      @JsonProperty("address") final EthAddress address) {
    this.abi = abi == null ? Collections.emptyList() : List.copyOf(abi);
    this.address = address;
  }

  /**
   * The ABI entries whose events the listener matches.
   *
   * @return the ABI entries, never {@code null} (empty when unset)
   */
  @JsonProperty("abi")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<AbiEntry> abi() {
    return abi;
  }

  /**
   * The contract address the source is scoped to, or {@code null} to match any address.
   *
   * @return the contract address, or {@code null} if unscoped
   */
  @JsonProperty("address")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress address() {
    return address;
  }

  /**
   * Starts an empty builder.
   *
   * @return a new builder
   */
  public static Builder builder() {
    return new Builder();
  }

  @Override
  public String toString() {
    return "BlockchainEventListenerSource{entries=" + abi.size() + ", address=" + address + "}";
  }

  /** Fluent builder for {@link BlockchainEventListenerSource}. */
  public static final class Builder {
    private final List<AbiEntry> abi = new ArrayList<>();
    private EthAddress address;

    private Builder() {}

    /**
     * Adds an entry to the source ABI.
     *
     * @param entry the ABI entry to add
     * @return this builder
     */
    public Builder abiEntry(final AbiEntry entry) {
      this.abi.add(entry);
      return this;
    }

    /**
     * Adds entries to the source ABI.
     *
     * @param abi the ABI entries to add
     * @return this builder
     */
    public Builder abi(final List<AbiEntry> abi) {
      this.abi.addAll(abi);
      return this;
    }

    /**
     * Scopes the source to a single contract address.
     *
     * @param address the contract address
     * @return this builder
     */
    public Builder address(final EthAddress address) {
      this.address = address;
      return this;
    }

    /**
     * Builds the immutable {@link BlockchainEventListenerSource}.
     *
     * @return a new {@link BlockchainEventListenerSource} with the configured values
     */
    public BlockchainEventListenerSource build() {
      return new BlockchainEventListenerSource(abi, address);
    }
  }
}
