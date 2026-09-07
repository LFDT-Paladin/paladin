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
import org.lfdt.paladin.sdk.core.types.HexBytes;

/**
 * The filters that select which messages a listener streams. Immutable; build one with the
 * {@linkplain #builder() fluent builder}.
 *
 * <p>Every filter is optional and unset filters match everything. {@link #sequenceAbove()} starts
 * the stream after a known local sequence rather than from the beginning.
 */
@JsonPropertyOrder({"sequenceAbove", "domain", "group", "topic"})
public final class PrivacyGroupMessageListenerFilters {

  private final Long sequenceAbove;
  private final String domain;
  private final HexBytes group;
  private final String topic;

  @JsonCreator
  PrivacyGroupMessageListenerFilters(
      @JsonProperty("sequenceAbove") final Long sequenceAbove,
      @JsonProperty("domain") final String domain,
      @JsonProperty("group") final HexBytes group,
      @JsonProperty("topic") final String topic) {
    this.sequenceAbove = sequenceAbove;
    this.domain = domain;
    this.group = group;
    this.topic = topic;
  }

  /**
   * Streams only messages with a local sequence above this value.
   *
   * @return the sequence to start after, or {@code null} to stream from the beginning
   */
  @JsonProperty("sequenceAbove")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Long sequenceAbove() {
    return sequenceAbove;
  }

  /**
   * Streams only messages for groups in this domain.
   *
   * @return the domain name, or an empty string to match every domain
   */
  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  /**
   * Streams only messages for this group.
   *
   * @return the group id, or {@code null} to match every group
   */
  @JsonProperty("group")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes group() {
    return group;
  }

  /**
   * Streams only messages on this topic.
   *
   * @return the topic, or an empty string to match every topic
   */
  @JsonProperty("topic")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String topic() {
    return topic;
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
    return "PrivacyGroupMessageListenerFilters{domain="
        + domain
        + ", group="
        + group
        + ", topic="
        + topic
        + "}";
  }

  /** Fluent builder for {@link PrivacyGroupMessageListenerFilters}. */
  public static final class Builder {
    private Long sequenceAbove;
    private String domain;
    private HexBytes group;
    private String topic;

    private Builder() {}

    /**
     * Sets the local sequence to start the stream after.
     *
     * @param sequenceAbove the sequence to start after
     * @return this builder
     */
    public Builder sequenceAbove(final Long sequenceAbove) {
      this.sequenceAbove = sequenceAbove;
      return this;
    }

    /**
     * Restricts the stream to groups in one domain.
     *
     * @param domain the domain name
     * @return this builder
     */
    public Builder domain(final String domain) {
      this.domain = domain;
      return this;
    }

    /**
     * Restricts the stream to one group.
     *
     * @param group the group id
     * @return this builder
     */
    public Builder group(final HexBytes group) {
      this.group = group;
      return this;
    }

    /**
     * Restricts the stream to one topic.
     *
     * @param topic the topic
     * @return this builder
     */
    public Builder topic(final String topic) {
      this.topic = topic;
      return this;
    }

    /**
     * Builds the immutable {@link PrivacyGroupMessageListenerFilters}.
     *
     * @return a new {@link PrivacyGroupMessageListenerFilters} with the configured values
     */
    public PrivacyGroupMessageListenerFilters build() {
      return new PrivacyGroupMessageListenerFilters(sequenceAbove, domain, group, topic);
    }
  }
}
