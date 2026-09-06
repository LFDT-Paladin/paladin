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
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/** A state schema registered by a domain, defining the shape of its states. Immutable. */
@JsonPropertyOrder({"id", "created", "domain", "type", "signature", "definition", "labels"})
public final class Schema {

  private final Bytes32 id;
  private final Timestamp created;
  private final String domain;
  private final SchemaType type;
  private final String signature;
  private final JsonNode definition;
  private final List<String> labels;

  @JsonCreator
  Schema(
      @JsonProperty("id") final Bytes32 id,
      @JsonProperty("created") final Timestamp created,
      @JsonProperty("domain") final String domain,
      @JsonProperty("type") final SchemaType type,
      @JsonProperty("signature") final String signature,
      @JsonProperty("definition") final JsonNode definition,
      @JsonProperty("labels") final List<String> labels) {
    this.id = id;
    this.created = created;
    this.domain = domain;
    this.type = type;
    this.signature = signature;
    this.definition = definition;
    this.labels = labels;
  }

  /**
   * The schema identifier.
   *
   * @return the schema id
   */
  @JsonProperty("id")
  public Bytes32 id() {
    return id;
  }

  /**
   * When the schema was created.
   *
   * @return the creation timestamp
   */
  @JsonProperty("created")
  public Timestamp created() {
    return created;
  }

  /**
   * The name of the domain that owns the schema.
   *
   * @return the domain name
   */
  @JsonProperty("domain")
  public String domain() {
    return domain;
  }

  /**
   * The kind of schema.
   *
   * @return the schema type
   */
  @JsonProperty("type")
  public SchemaType type() {
    return type;
  }

  /**
   * The signature that uniquely identifies the schema definition.
   *
   * @return the schema signature
   */
  @JsonProperty("signature")
  public String signature() {
    return signature;
  }

  /**
   * The schema definition as raw JSON.
   *
   * @return the schema definition
   */
  @JsonProperty("definition")
  public JsonNode definition() {
    return definition;
  }

  /**
   * The names of the indexed labels defined by the schema.
   *
   * @return the label names
   */
  @JsonProperty("labels")
  public List<String> labels() {
    return labels;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof Schema other
        && Objects.equals(id, other.id)
        && Objects.equals(created, other.created)
        && Objects.equals(domain, other.domain)
        && type == other.type
        && Objects.equals(signature, other.signature)
        && Objects.equals(definition, other.definition)
        && Objects.equals(labels, other.labels);
  }

  @Override
  public int hashCode() {
    return Objects.hash(id, created, domain, type, signature, definition, labels);
  }

  @Override
  public String toString() {
    return "Schema{id=" + id + ", domain=" + domain + ", signature=" + signature + "}";
  }
}
