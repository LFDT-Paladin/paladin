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

// Immutable; mirrors pldapi.Transaction (server-generated id/created/submitMode plus the embedded
// TransactionBase fields). The base fields are flattened here to match the flat JSON wire form, as
// in TransactionInput. Subclassed by TransactionFull, so this type is not final.
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
      @JsonProperty("id") UUID id,
      @JsonProperty("created") Timestamp created,
      @JsonProperty("submitMode") SubmitMode submitMode,
      @JsonProperty("idempotencyKey") String idempotencyKey,
      @JsonProperty("type") TransactionType type,
      @JsonProperty("domain") String domain,
      @JsonProperty("function") String function,
      @JsonProperty("abiReference") Bytes32 abiReference,
      @JsonProperty("from") String from,
      @JsonProperty("to") EthAddress to,
      @JsonProperty("data") JsonNode data,
      @JsonProperty("gas") HexUint64 gas,
      @JsonProperty("value") HexUint256 value,
      @JsonProperty("maxPriorityFeePerGas") HexUint256 maxPriorityFeePerGas,
      @JsonProperty("maxFeePerGas") HexUint256 maxFeePerGas) {
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

  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID id() {
    return id;
  }

  @JsonProperty("created")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp created() {
    return created;
  }

  @JsonProperty("submitMode")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public SubmitMode submitMode() {
    return submitMode;
  }

  @JsonProperty("idempotencyKey")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String idempotencyKey() {
    return idempotencyKey;
  }

  @JsonProperty("type")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionType type() {
    return type;
  }

  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  @JsonProperty("function")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String function() {
    return function;
  }

  @JsonProperty("abiReference")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Bytes32 abiReference() {
    return abiReference;
  }

  @JsonProperty("from")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String from() {
    return from;
  }

  @JsonProperty("to")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress to() {
    return to;
  }

  @JsonProperty("data")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode data() {
    return data;
  }

  @JsonProperty("gas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint64 gas() {
    return gas;
  }

  @JsonProperty("value")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 value() {
    return value;
  }

  @JsonProperty("maxPriorityFeePerGas")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexUint256 maxPriorityFeePerGas() {
    return maxPriorityFeePerGas;
  }

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
