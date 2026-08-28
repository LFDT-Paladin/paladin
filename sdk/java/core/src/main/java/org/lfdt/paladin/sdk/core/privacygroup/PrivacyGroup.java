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
package org.lfdt.paladin.sdk.core.privacygroup;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * A privacy group as returned by the node — a named set of members within a domain, with the
 * genesis data that pins its identity on the base ledger. Immutable, server-assigned; create one
 * with {@link PrivacyGroupInput}.
 *
 * <p>{@link #contractAddress()} is {@code null} until the genesis transaction has been mined. The
 * {@link #members()} list and the {@link #properties()} / {@link #configuration()} maps are never
 * null (empty when unset).
 */
@JsonPropertyOrder({
  "id",
  "domain",
  "created",
  "name",
  "members",
  "properties",
  "configuration",
  "genesisSalt",
  "genesisSchema",
  "genesisTransaction",
  "contractAddress"
})
public final class PrivacyGroup {

  private final HexBytes id;
  private final String domain;
  private final Timestamp created;
  private final String name;
  private final List<String> members;
  private final Map<String, String> properties;
  private final Map<String, String> configuration;
  private final Bytes32 genesisSalt;
  private final Bytes32 genesisSchema;
  private final UUID genesisTransaction;
  private final EthAddress contractAddress;

  @JsonCreator
  PrivacyGroup(
      @JsonProperty("id") final HexBytes id,
      @JsonProperty("domain") final String domain,
      @JsonProperty("created") final Timestamp created,
      @JsonProperty("name") final String name,
      @JsonProperty("members") final List<String> members,
      @JsonProperty("properties") final Map<String, String> properties,
      @JsonProperty("configuration") final Map<String, String> configuration,
      @JsonProperty("genesisSalt") final Bytes32 genesisSalt,
      @JsonProperty("genesisSchema") final Bytes32 genesisSchema,
      @JsonProperty("genesisTransaction") final UUID genesisTransaction,
      @JsonProperty("contractAddress") final EthAddress contractAddress) {
    this.id = id;
    this.domain = domain;
    // The node sends a zero timestamp for "unset"; normalize to null to keep round-trips clean.
    this.created = (created == null || created.isZero()) ? null : created;
    this.name = name;
    this.members = members == null ? List.of() : List.copyOf(members);
    this.properties = properties == null ? Map.of() : Map.copyOf(properties);
    this.configuration = configuration == null ? Map.of() : Map.copyOf(configuration);
    this.genesisSalt = genesisSalt;
    this.genesisSchema = genesisSchema;
    this.genesisTransaction = genesisTransaction;
    this.contractAddress = contractAddress;
  }

  /**
   * The group identifier, unique within its domain.
   *
   * @return the group id, or {@code null} if unset
   */
  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes id() {
    return id;
  }

  /**
   * The domain the group belongs to.
   *
   * @return the domain name, or an empty string when unset
   */
  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  /**
   * The time the group was created (server-assigned).
   *
   * @return the created timestamp, or {@code null} if unset
   */
  @JsonProperty("created")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp created() {
    return created;
  }

  /**
   * The name of the group.
   *
   * @return the group name, or an empty string when unset
   */
  @JsonProperty("name")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String name() {
    return name;
  }

  /**
   * The identity locators of the group's members.
   *
   * @return the members, never {@code null} (empty when unset)
   */
  @JsonProperty("members")
  public List<String> members() {
    return members;
  }

  /**
   * The free-form properties recorded in the group's genesis state.
   *
   * @return the properties, never {@code null} (empty when unset)
   */
  @JsonProperty("properties")
  public Map<String, String> properties() {
    return properties;
  }

  /**
   * The domain-interpreted configuration recorded in the group's genesis state.
   *
   * @return the configuration, never {@code null} (empty when unset)
   */
  @JsonProperty("configuration")
  public Map<String, String> configuration() {
    return configuration;
  }

  /**
   * The random salt that makes the genesis state unique.
   *
   * @return the genesis salt, or {@code null} if unset
   */
  @JsonProperty("genesisSalt")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Bytes32 genesisSalt() {
    return genesisSalt;
  }

  /**
   * The schema id of the group's genesis state.
   *
   * @return the genesis schema id, or {@code null} if unset
   */
  @JsonProperty("genesisSchema")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Bytes32 genesisSchema() {
    return genesisSchema;
  }

  /**
   * The id of the transaction that created the group.
   *
   * @return the genesis transaction id, or {@code null} if unset
   */
  @JsonProperty("genesisTransaction")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID genesisTransaction() {
    return genesisTransaction;
  }

  /**
   * The on-chain address of the group's contract.
   *
   * @return the contract address, or {@code null} until the genesis transaction is mined
   */
  @JsonProperty("contractAddress")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public EthAddress contractAddress() {
    return contractAddress;
  }

  @Override
  public String toString() {
    return "PrivacyGroup{id=" + id + ", domain=" + domain + ", name=" + name + "}";
  }
}
