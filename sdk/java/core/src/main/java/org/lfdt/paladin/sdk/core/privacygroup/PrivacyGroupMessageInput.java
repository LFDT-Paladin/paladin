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
import com.fasterxml.jackson.databind.JsonNode;
import java.util.UUID;
import org.lfdt.paladin.sdk.core.types.HexBytes;

/**
 * A message to send to the members of a privacy group. Immutable; build one with the {@linkplain
 * #builder(String, HexBytes) fluent builder}.
 *
 * <p>Messages are delivered off-chain to the group's members and are independent of the group's
 * transactions. {@link #topic()} is free-form and is what message listeners filter on.
 */
@JsonPropertyOrder({"correlationId", "domain", "group", "topic", "data"})
public final class PrivacyGroupMessageInput {

  private final UUID correlationId;
  private final String domain;
  private final HexBytes group;
  private final String topic;
  private final JsonNode data;

  @JsonCreator
  PrivacyGroupMessageInput(
      @JsonProperty("correlationId") final UUID correlationId,
      @JsonProperty("domain") final String domain,
      @JsonProperty("group") final HexBytes group,
      @JsonProperty("topic") final String topic,
      @JsonProperty("data") final JsonNode data) {
    this.correlationId = correlationId;
    this.domain = domain;
    this.group = group;
    this.topic = topic;
    this.data = data;
  }

  /**
   * The id of an earlier message this one responds to.
   *
   * @return the correlation id, or {@code null} if unset
   */
  @JsonProperty("correlationId")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID correlationId() {
    return correlationId;
  }

  /**
   * The domain the target group belongs to.
   *
   * @return the domain name, or an empty string when unset
   */
  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  /**
   * The identifier of the group to send the message to.
   *
   * @return the group id, or {@code null} if unset
   */
  @JsonProperty("group")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes group() {
    return group;
  }

  /**
   * The free-form topic that message listeners filter on.
   *
   * @return the topic, or an empty string when unset
   */
  @JsonProperty("topic")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String topic() {
    return topic;
  }

  /**
   * The message payload, as arbitrary JSON.
   *
   * @return the payload, or {@code null} if unset
   */
  @JsonProperty("data")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode data() {
    return data;
  }

  /**
   * Starts a builder for a message to the given group.
   *
   * @param domain the domain the group belongs to
   * @param group the identifier of the group to send to
   * @return a new builder
   */
  public static Builder builder(final String domain, final HexBytes group) {
    return new Builder(domain, group);
  }

  @Override
  public String toString() {
    return "PrivacyGroupMessageInput{domain="
        + domain
        + ", group="
        + group
        + ", topic="
        + topic
        + "}";
  }

  /** Fluent builder for {@link PrivacyGroupMessageInput}. */
  public static final class Builder {
    private final String domain;
    private final HexBytes group;
    private UUID correlationId;
    private String topic;
    private JsonNode data;

    private Builder(final String domain, final HexBytes group) {
      this.domain = domain;
      this.group = group;
    }

    /**
     * Sets the id of an earlier message this one responds to.
     *
     * @param correlationId the correlation id
     * @return this builder
     */
    public Builder correlationId(final UUID correlationId) {
      this.correlationId = correlationId;
      return this;
    }

    /**
     * Sets the free-form topic that message listeners filter on.
     *
     * @param topic the topic
     * @return this builder
     */
    public Builder topic(final String topic) {
      this.topic = topic;
      return this;
    }

    /**
     * Sets the message payload.
     *
     * @param data the payload, as arbitrary JSON
     * @return this builder
     */
    public Builder data(final JsonNode data) {
      this.data = data;
      return this;
    }

    /**
     * Builds the immutable {@link PrivacyGroupMessageInput}.
     *
     * @return a new {@link PrivacyGroupMessageInput} with the configured values
     */
    public PrivacyGroupMessageInput build() {
      return new PrivacyGroupMessageInput(correlationId, domain, group, topic, data);
    }
  }
}
