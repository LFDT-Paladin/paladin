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
import com.fasterxml.jackson.databind.JsonNode;
import java.util.ArrayList;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;

/**
 * The body submitted when sending a transaction to a Paladin node, mirroring {@code
 * pldapi.TransactionInput} (which embeds {@code TransactionBase} and {@code PublicTxOptions}).
 *
 * <p>Immutable and self-serializing: every field follows the Go {@code omitempty} convention and is
 * omitted when null/empty, so a minimal submission emits a minimal JSON body. The {@code dependsOn}
 * and {@code abi} lists are never null (empty when unset). Round-trips through any {@code
 * ObjectMapper}.
 *
 * <p>The public-transaction options ({@link #gas()}, {@link #value()}, {@link
 * #maxPriorityFeePerGas()}, {@link #maxFeePerGas()}) mirror Go's embedded {@code
 * PublicTxOptions}/{@code PublicTxGasPricing} and are flattened here to match the flat JSON wire
 * form. Supplying any gas-pricing field fixes pricing for the transaction, disabling the node's
 * gas-pricing engine.
 */
@JsonPropertyOrder({
  "idempotencyKey",
  "type",
  "domain",
  "function",
  "abiReference",
  "from",
  "to",
  "data",
  "gas",
  "value",
  "maxPriorityFeePerGas",
  "maxFeePerGas",
  "dependsOn",
  "abi",
  "bytecode"
})
public final class TransactionInput {

  private final String idempotencyKey;
  private final TransactionType type;
  private final String domain;
  private final String function;
  private final Bytes32 abiReference;
  private final String from;
  private final EthAddress to;
  private final JsonNode data;
  private final HexUint64 gas;
  private final HexUint256 value;
  private final HexUint256 maxPriorityFeePerGas;
  private final HexUint256 maxFeePerGas;
  private final List<UUID> dependsOn;
  private final List<AbiEntry> abi;
  private final HexBytes bytecode;

  @JsonCreator
  TransactionInput(
      @JsonProperty("idempotencyKey") final String idempotencyKey,
      @JsonProperty("type") final TransactionType type,
      @JsonProperty("domain") final String domain,
      @JsonProperty("function") final String function,
      @JsonProperty("abiReference") final Bytes32 abiReference,
      @JsonProperty("from") final String from,
      @JsonProperty("to") final EthAddress to,
      @JsonProperty("data") final JsonNode data,
      @JsonProperty("gas") final HexUint64 gas,
      @JsonProperty("value") final HexUint256 value,
      @JsonProperty("maxPriorityFeePerGas") final HexUint256 maxPriorityFeePerGas,
      @JsonProperty("maxFeePerGas") final HexUint256 maxFeePerGas,
      @JsonProperty("dependsOn") final List<UUID> dependsOn,
      @JsonProperty("abi") final List<AbiEntry> abi,
      @JsonProperty("bytecode") final HexBytes bytecode) {
    this.idempotencyKey = idempotencyKey;
    this.type = type;
    this.domain = domain;
    this.function = function;
    this.abiReference = abiReference;
    this.from = from;
    this.to = to;
    this.data = data;
    this.gas = gas;
    this.value = value;
    this.maxPriorityFeePerGas = maxPriorityFeePerGas;
    this.maxFeePerGas = maxFeePerGas;
    this.dependsOn = dependsOn == null ? List.of() : List.copyOf(dependsOn);
    this.abi = abi == null ? List.of() : List.copyOf(abi);
    this.bytecode = bytecode;
  }

  /**
   * Externally supplied unique identifier; a re-submit with the same key yields 409 Conflict.
   *
   * @return the idempotency key, or an empty string when unset
   */
  @JsonProperty("idempotencyKey")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String idempotencyKey() {
    return idempotencyKey;
  }

  /**
   * Public (straight to the base ledger) or private (masked through a domain), or {@code null} if
   * unset.
   *
   * @return the transaction type, or {@code null} if unset
   */
  @JsonProperty("type")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionType type() {
    return type;
  }

  /**
   * Domain name; required only for private deploy transactions (inferred from {@code to} for
   * invoke).
   *
   * @return the domain name, or an empty string when unset
   */
  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  /**
   * Function name; inferred from the ABI if not supplied, then resolved to a full signature and
   * stored.
   *
   * @return the function name, or an empty string when unset
   */
  @JsonProperty("function")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String function() {
    return function;
  }

  /**
   * Reference to a stored ABI; calculated and stored for you if not supplied.
   *
   * @return the ABI reference, or {@code null} when unset
   */
  @JsonProperty("abiReference")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Bytes32 abiReference() {
    return abiReference;
  }

  /**
   * Locator for a local signing identity used to submit this transaction.
   *
   * @return the signing identity locator, or an empty string when unset
   */
  @JsonProperty("from")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String from() {
    return from;
  }

  /**
   * Target contract address, or {@code null} for a deploy.
   *
   * @return the target contract address, or {@code null} for a deploy
   */
  @JsonProperty("to")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress to() {
    return to;
  }

  /**
   * Pre-encoded call inputs — an array (with or without the function selector) or an object.
   *
   * @return the call inputs, or {@code null} when unset
   */
  @JsonProperty("data")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode data() {
    return data;
  }

  /**
   * Gas limit, or {@code null} to let the node estimate.
   *
   * @return the gas limit, or {@code null} to let the node estimate
   */
  @JsonProperty("gas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint64 gas() {
    return gas;
  }

  /**
   * Native value to transfer with the transaction, or {@code null} for none.
   *
   * @return the native value to transfer, or {@code null} for none
   */
  @JsonProperty("value")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 value() {
    return value;
  }

  /**
   * EIP-1559 max priority fee per gas; supplying it fixes gas pricing for this transaction.
   *
   * @return the max priority fee per gas, or {@code null} when unset
   */
  @JsonProperty("maxPriorityFeePerGas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 maxPriorityFeePerGas() {
    return maxPriorityFeePerGas;
  }

  /**
   * EIP-1559 max fee per gas; supplying it fixes gas pricing for this transaction.
   *
   * @return the max fee per gas, or {@code null} when unset
   */
  @JsonProperty("maxFeePerGas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 maxFeePerGas() {
    return maxFeePerGas;
  }

  /**
   * Transactions that must be mined (or deleted) before this one submits. Never null.
   *
   * @return the dependency transaction ids (never null, empty when there are none)
   */
  @JsonProperty("dependsOn")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<UUID> dependsOn() {
    return dependsOn;
  }

  /**
   * Inline ABI; required if {@link #abiReference()} is not supplied. Never null.
   *
   * @return the inline ABI entries (never null, empty when there are none)
   */
  @JsonProperty("abi")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public List<AbiEntry> abi() {
    return abi;
  }

  /**
   * Deploy bytecode, prepended to the encoded data inputs; {@code null} for an invoke.
   *
   * @return the deploy bytecode, or {@code null} for an invoke
   */
  @JsonProperty("bytecode")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes bytecode() {
    return bytecode;
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
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof TransactionInput other
        && Objects.equals(idempotencyKey, other.idempotencyKey)
        && type == other.type
        && Objects.equals(domain, other.domain)
        && Objects.equals(function, other.function)
        && Objects.equals(abiReference, other.abiReference)
        && Objects.equals(from, other.from)
        && Objects.equals(to, other.to)
        && Objects.equals(data, other.data)
        && Objects.equals(gas, other.gas)
        && Objects.equals(value, other.value)
        && Objects.equals(maxPriorityFeePerGas, other.maxPriorityFeePerGas)
        && Objects.equals(maxFeePerGas, other.maxFeePerGas)
        && dependsOn.equals(other.dependsOn)
        && abi.equals(other.abi)
        && Objects.equals(bytecode, other.bytecode);
  }

  @Override
  public int hashCode() {
    return Objects.hash(
        idempotencyKey,
        type,
        domain,
        function,
        abiReference,
        from,
        to,
        data,
        gas,
        value,
        maxPriorityFeePerGas,
        maxFeePerGas,
        dependsOn,
        abi,
        bytecode);
  }

  @Override
  public String toString() {
    return "TransactionInput{type="
        + type
        + ", from="
        + from
        + ", to="
        + to
        + ", function="
        + function
        + "}";
  }

  /** Fluent builder for {@link TransactionInput}. */
  public static final class Builder {
    private String idempotencyKey;
    private TransactionType type;
    private String domain;
    private String function;
    private Bytes32 abiReference;
    private String from;
    private EthAddress to;
    private JsonNode data;
    private HexUint64 gas;
    private HexUint256 value;
    private HexUint256 maxPriorityFeePerGas;
    private HexUint256 maxFeePerGas;
    private final List<UUID> dependsOn = new ArrayList<>();
    private final List<AbiEntry> abi = new ArrayList<>();
    private HexBytes bytecode;

    private Builder() {}

    /**
     * Sets the idempotency key.
     *
     * @param idempotencyKey the externally supplied unique identifier
     * @return this builder
     */
    public Builder idempotencyKey(final String idempotencyKey) {
      this.idempotencyKey = idempotencyKey;
      return this;
    }

    /**
     * Sets the transaction type.
     *
     * @param type public or private
     * @return this builder
     */
    public Builder type(final TransactionType type) {
      this.type = type;
      return this;
    }

    /**
     * Sets the domain name.
     *
     * @param domain the domain name (private deploys only)
     * @return this builder
     */
    public Builder domain(final String domain) {
      this.domain = domain;
      return this;
    }

    /**
     * Sets the function name.
     *
     * @param function the function name or signature
     * @return this builder
     */
    public Builder function(final String function) {
      this.function = function;
      return this;
    }

    /**
     * Sets the stored ABI reference.
     *
     * @param abiReference reference to a stored ABI
     * @return this builder
     */
    public Builder abiReference(final Bytes32 abiReference) {
      this.abiReference = abiReference;
      return this;
    }

    /**
     * Sets the signing identity locator.
     *
     * @param from the local signing identity used to submit the transaction
     * @return this builder
     */
    public Builder from(final String from) {
      this.from = from;
      return this;
    }

    /**
     * Sets the target contract address.
     *
     * @param to the target contract address, or {@code null} for a deploy
     * @return this builder
     */
    public Builder to(final EthAddress to) {
      this.to = to;
      return this;
    }

    /**
     * Sets the pre-encoded call inputs.
     *
     * @param data the call inputs as an array or object
     * @return this builder
     */
    public Builder data(final JsonNode data) {
      this.data = data;
      return this;
    }

    /**
     * Sets the gas limit.
     *
     * @param gas the gas limit, or {@code null} to let the node estimate
     * @return this builder
     */
    public Builder gas(final HexUint64 gas) {
      this.gas = gas;
      return this;
    }

    /**
     * Sets the native value to transfer.
     *
     * @param value the native value to transfer
     * @return this builder
     */
    public Builder value(final HexUint256 value) {
      this.value = value;
      return this;
    }

    /**
     * Sets the EIP-1559 max priority fee per gas.
     *
     * @param maxPriorityFeePerGas the max priority fee per gas; supplying it fixes gas pricing
     * @return this builder
     */
    public Builder maxPriorityFeePerGas(final HexUint256 maxPriorityFeePerGas) {
      this.maxPriorityFeePerGas = maxPriorityFeePerGas;
      return this;
    }

    /**
     * Sets the EIP-1559 max fee per gas.
     *
     * @param maxFeePerGas the max fee per gas; supplying it fixes gas pricing
     * @return this builder
     */
    public Builder maxFeePerGas(final HexUint256 maxFeePerGas) {
      this.maxFeePerGas = maxFeePerGas;
      return this;
    }

    /**
     * Adds a transaction this one depends on.
     *
     * @param dependency the id of a transaction that must be mined (or deleted) first
     * @return this builder
     */
    public Builder dependsOn(final UUID dependency) {
      this.dependsOn.add(dependency);
      return this;
    }

    /**
     * Adds transactions this one depends on.
     *
     * @param dependencies the ids of transactions that must be mined (or deleted) first
     * @return this builder
     */
    public Builder dependsOn(final List<UUID> dependencies) {
      this.dependsOn.addAll(dependencies);
      return this;
    }

    /**
     * Adds an entry to the inline ABI.
     *
     * @param entry the ABI entry to add
     * @return this builder
     */
    public Builder abiEntry(final AbiEntry entry) {
      this.abi.add(entry);
      return this;
    }

    /**
     * Adds entries to the inline ABI.
     *
     * @param abi the ABI entries to add
     * @return this builder
     */
    public Builder abi(final List<AbiEntry> abi) {
      this.abi.addAll(abi);
      return this;
    }

    /**
     * Sets the deploy bytecode.
     *
     * @param bytecode the deploy bytecode, or {@code null} for an invoke
     * @return this builder
     */
    public Builder bytecode(final HexBytes bytecode) {
      this.bytecode = bytecode;
      return this;
    }

    /**
     * Builds the immutable {@link TransactionInput}.
     *
     * @return a new {@link TransactionInput} with the configured values
     */
    public TransactionInput build() {
      return new TransactionInput(
          idempotencyKey,
          type,
          domain,
          function,
          abiReference,
          from,
          to,
          data,
          gas,
          value,
          maxPriorityFeePerGas,
          maxFeePerGas,
          dependsOn,
          abi,
          bytecode);
    }
  }
}
