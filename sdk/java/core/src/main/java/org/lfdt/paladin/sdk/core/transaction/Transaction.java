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
import java.util.UUID;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * A transaction as recorded by the node, mirroring {@code pldapi.Transaction} — the
 * server-generated {@code id}/{@code created}/{@code submitMode} plus the embedded {@code
 * TransactionBase} fields. Immutable.
 *
 * <p>The base fields are flattened here to match the flat JSON wire form, as in {@link
 * TransactionInput}. Subclassed by {@link TransactionFull}, so this type is not {@code final}.
 */
@JsonPropertyOrder({
  "id",
  "created",
  "submitMode",
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
  "maxFeePerGas"
})
public class Transaction {

  private final UUID id;
  private final Timestamp created;
  private final SubmitMode submitMode;
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

  @JsonCreator
  Transaction(
      @JsonProperty("id") final UUID id,
      @JsonProperty("created") final Timestamp created,
      @JsonProperty("submitMode") final SubmitMode submitMode,
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
      @JsonProperty("maxFeePerGas") final HexUint256 maxFeePerGas) {
    this.id = id;
    // A zero timestamp is "unset" (Go omitempty); normalize to null to keep round-trips clean.
    this.created = (created == null || created.isZero()) ? null : created;
    this.submitMode = submitMode;
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
  }

  /**
   * The server-assigned transaction id.
   *
   * @return the transaction id, or {@code null} if unset
   */
  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID id() {
    return id;
  }

  /**
   * The time the transaction was created.
   *
   * @return the created timestamp, or {@code null} if unset
   */
  @JsonProperty("created")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp created() {
    return created;
  }

  /**
   * How the transaction was submitted (for example, auto, external, or prepared).
   *
   * @return the submit mode, or {@code null} if unset
   */
  @JsonProperty("submitMode")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public SubmitMode submitMode() {
    return submitMode;
  }

  /**
   * Caller-supplied key that makes submission idempotent.
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

  @Override
  public String toString() {
    return "Transaction{id=" + id + ", type=" + type + ", domain=" + domain + "}";
  }
}
