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
package org.lfdt.paladin.sdk.core.statestore;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * A state held in the state store, with its private data and any confirm, read, spend, lock, and
 * nullifier records.
 *
 * <p>The lifecycle records ({@link #confirmed()}, {@link #read()}, {@link #spent()}, {@link
 * #locks()}, {@link #nullifier()}) are present only when they apply; otherwise they are {@code
 * null}. Immutable.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
@JsonPropertyOrder({
  "id",
  "created",
  "domain",
  "schema",
  "contractAddress",
  "data",
  "confirmed",
  "read",
  "spent",
  "locks",
  "nullifier"
})
public final class State {

  private final HexBytes id;
  private final Timestamp created;
  private final String domain;
  private final Bytes32 schema;
  private final EthAddress contractAddress;
  private final JsonNode data;
  private final StateConfirmRecord confirmed;
  private final StateReadRecord read;
  private final StateSpendRecord spent;
  private final List<StateLock> locks;
  private final StateNullifier nullifier;

  @JsonCreator
  State(
      @JsonProperty("id") final HexBytes id,
      @JsonProperty("created") final Timestamp created,
      @JsonProperty("domain") final String domain,
      @JsonProperty("schema") final Bytes32 schema,
      @JsonProperty("contractAddress") final EthAddress contractAddress,
      @JsonProperty("data") final JsonNode data,
      @JsonProperty("confirmed") final StateConfirmRecord confirmed,
      @JsonProperty("read") final StateReadRecord read,
      @JsonProperty("spent") final StateSpendRecord spent,
      @JsonProperty("locks") final List<StateLock> locks,
      @JsonProperty("nullifier") final StateNullifier nullifier) {
    this.id = id;
    this.created = created;
    this.domain = domain;
    this.schema = schema;
    this.contractAddress = contractAddress;
    this.data = data;
    this.confirmed = confirmed;
    this.read = read;
    this.spent = spent;
    this.locks = locks;
    this.nullifier = nullifier;
  }

  /**
   * The state identifier.
   *
   * @return the state id
   */
  @JsonProperty("id")
  public HexBytes id() {
    return id;
  }

  /**
   * When the state was created.
   *
   * @return the creation timestamp
   */
  @JsonProperty("created")
  public Timestamp created() {
    return created;
  }

  /**
   * The name of the domain that owns the state.
   *
   * @return the domain name
   */
  @JsonProperty("domain")
  public String domain() {
    return domain;
  }

  /**
   * The identifier of the schema the state conforms to.
   *
   * @return the schema id
   */
  @JsonProperty("schema")
  public Bytes32 schema() {
    return schema;
  }

  /**
   * The contract the state belongs to, or {@code null} for states (such as a privacy group genesis)
   * that exist before their contract.
   *
   * @return the contract address, or {@code null}
   */
  @JsonProperty("contractAddress")
  public EthAddress contractAddress() {
    return contractAddress;
  }

  /**
   * The state's private data as raw JSON.
   *
   * @return the state data
   */
  @JsonProperty("data")
  public JsonNode data() {
    return data;
  }

  /**
   * The confirm record for the state, or {@code null} if it is not confirmed on chain.
   *
   * @return the confirm record, or {@code null}
   */
  @JsonProperty("confirmed")
  public StateConfirmRecord confirmed() {
    return confirmed;
  }

  /**
   * The read record for the state, or {@code null} if none applies.
   *
   * @return the read record, or {@code null}
   */
  @JsonProperty("read")
  public StateReadRecord read() {
    return read;
  }

  /**
   * The spend record for the state, or {@code null} if it has not been spent.
   *
   * @return the spend record, or {@code null}
   */
  @JsonProperty("spent")
  public StateSpendRecord spent() {
    return spent;
  }

  /**
   * The in-memory locks held against the state, or {@code null} if there are none.
   *
   * @return the locks, or {@code null}
   */
  @JsonProperty("locks")
  public List<StateLock> locks() {
    return locks;
  }

  /**
   * The nullifier associated with the state, or {@code null} if the domain does not use nullifiers.
   *
   * @return the nullifier, or {@code null}
   */
  @JsonProperty("nullifier")
  public StateNullifier nullifier() {
    return nullifier;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof State other
        && Objects.equals(id, other.id)
        && Objects.equals(created, other.created)
        && Objects.equals(domain, other.domain)
        && Objects.equals(schema, other.schema)
        && Objects.equals(contractAddress, other.contractAddress)
        && Objects.equals(data, other.data)
        && Objects.equals(confirmed, other.confirmed)
        && Objects.equals(read, other.read)
        && Objects.equals(spent, other.spent)
        && Objects.equals(locks, other.locks)
        && Objects.equals(nullifier, other.nullifier);
  }

  @Override
  public int hashCode() {
    return Objects.hash(
        id,
        created,
        domain,
        schema,
        contractAddress,
        data,
        confirmed,
        read,
        spent,
        locks,
        nullifier);
  }

  @Override
  public String toString() {
    return "State{id=" + id + ", domain=" + domain + ", schema=" + schema + "}";
  }
}
