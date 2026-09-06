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
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * A named, filtered stream of privacy group messages. Immutable.
 *
 * <p>Used both as the input to {@code pgroup_createMessageListener} and as the result of {@code
 * pgroup_getMessageListener} / {@code pgroup_queryMessageListeners}; build one with the {@linkplain
 * #builder(String) fluent builder} to create a listener ({@link #created()} is server-assigned).
 */
@JsonPropertyOrder({"name", "created", "started", "filters", "options"})
public final class PrivacyGroupMessageListener {

  private final String name;
  private final Timestamp created;
  private final Boolean started;
  private final PrivacyGroupMessageListenerFilters filters;
  private final PrivacyGroupMessageListenerOptions options;

  @JsonCreator
  PrivacyGroupMessageListener(
      @JsonProperty("name") final String name,
      @JsonProperty("created") final Timestamp created,
      @JsonProperty("started") final Boolean started,
      @JsonProperty("filters") final PrivacyGroupMessageListenerFilters filters,
      @JsonProperty("options") final PrivacyGroupMessageListenerOptions options) {
    this.name = name;
    // The node sends a zero timestamp for "unset"; normalize to null to keep round-trips clean.
    this.created = (created == null || created.isZero()) ? null : created;
    this.started = started;
    this.filters = filters;
    this.options = options;
  }

  /**
   * The unique name of the listener.
   *
   * @return the listener name, or an empty string when unset
   */
  @JsonProperty("name")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String name() {
    return name;
  }

  /**
   * The time the listener was created (server-assigned).
   *
   * @return the created timestamp, or {@code null} if unset
   */
  @JsonProperty("created")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp created() {
    return created;
  }

  /**
   * Whether the listener is currently started.
   *
   * @return the started flag, or {@code null} if unset
   */
  @JsonProperty("started")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Boolean started() {
    return started;
  }

  /**
   * The filters that select which messages the listener streams.
   *
   * @return the filters, or {@code null} if unset
   */
  @JsonProperty("filters")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public PrivacyGroupMessageListenerFilters filters() {
    return filters;
  }

  /**
   * The delivery options for the listener.
   *
   * @return the options, or {@code null} if unset
   */
  @JsonProperty("options")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public PrivacyGroupMessageListenerOptions options() {
    return options;
  }

  /**
   * Starts a builder for a listener with the given name.
   *
   * @param name the unique listener name
   * @return a new builder
   */
  public static Builder builder(final String name) {
    return new Builder(name);
  }

  @Override
  public String toString() {
    return "PrivacyGroupMessageListener{name=" + name + ", started=" + started + "}";
  }

  /** Fluent builder for {@link PrivacyGroupMessageListener}. */
  public static final class Builder {
    private final String name;
    private Boolean started;
    private PrivacyGroupMessageListenerFilters filters;
    private PrivacyGroupMessageListenerOptions options;

    private Builder(final String name) {
      this.name = name;
    }

    /**
     * Sets whether the listener starts in the started state.
     *
     * @param started the started flag
     * @return this builder
     */
    public Builder started(final Boolean started) {
      this.started = started;
      return this;
    }

    /**
     * Sets the filters that select which messages the listener streams.
     *
     * @param filters the filters
     * @return this builder
     */
    public Builder filters(final PrivacyGroupMessageListenerFilters filters) {
      this.filters = filters;
      return this;
    }

    /**
     * Sets the delivery options for the listener.
     *
     * @param options the options
     * @return this builder
     */
    public Builder options(final PrivacyGroupMessageListenerOptions options) {
      this.options = options;
      return this;
    }

    /**
     * Builds the immutable {@link PrivacyGroupMessageListener}.
     *
     * @return a new {@link PrivacyGroupMessageListener} with the configured values
     */
    public PrivacyGroupMessageListener build() {
      return new PrivacyGroupMessageListener(name, null, started, filters, options);
    }
  }
}
