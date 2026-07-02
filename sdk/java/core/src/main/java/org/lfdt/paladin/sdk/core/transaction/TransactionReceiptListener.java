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
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * A named, filtered stream of transaction receipts, mirroring {@code
 * pldapi.TransactionReceiptListener}. Immutable.
 *
 * <p>Used both as the input to {@code ptx_createReceiptListener} and as the result of {@code
 * ptx_getReceiptListener} / {@code ptx_queryReceiptListeners}; build one with the {@linkplain
 * #builder() fluent builder} to create a listener ({@link #created()} is server-assigned).
 */
@JsonPropertyOrder({"name", "created", "started", "filters", "options"})
public final class TransactionReceiptListener {

  private final String name;
  private final Timestamp created;
  private final Boolean started;
  private final TransactionReceiptFilters filters;
  private final TransactionReceiptListenerOptions options;

  @JsonCreator
  TransactionReceiptListener(
      @JsonProperty("name") String name,
      @JsonProperty("created") Timestamp created,
      @JsonProperty("started") Boolean started,
      @JsonProperty("filters") TransactionReceiptFilters filters,
      @JsonProperty("options") TransactionReceiptListenerOptions options) {
    this.name = name;
    // A zero timestamp is "unset" (Go omitempty); normalize to null to keep round-trips clean.
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
   * The filters that select which receipts the listener streams.
   *
   * @return the filters, or {@code null} if unset
   */
  @JsonProperty("filters")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionReceiptFilters filters() {
    return filters;
  }

  /**
   * The delivery options for the listener.
   *
   * @return the options, or {@code null} if unset
   */
  @JsonProperty("options")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionReceiptListenerOptions options() {
    return options;
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
    return "TransactionReceiptListener{name=" + name + ", started=" + started + "}";
  }

  /** Fluent builder for {@link TransactionReceiptListener}. */
  public static final class Builder {
    private String name;
    private Boolean started;
    private TransactionReceiptFilters filters;
    private TransactionReceiptListenerOptions options;

    private Builder() {}

    /**
     * Sets the unique listener name.
     *
     * @param name the listener name
     * @return this builder
     */
    public Builder name(String name) {
      this.name = name;
      return this;
    }

    /**
     * Sets whether the listener starts in the started state.
     *
     * @param started the started flag
     * @return this builder
     */
    public Builder started(Boolean started) {
      this.started = started;
      return this;
    }

    /**
     * Sets the filters that select which receipts the listener streams.
     *
     * @param filters the filters
     * @return this builder
     */
    public Builder filters(TransactionReceiptFilters filters) {
      this.filters = filters;
      return this;
    }

    /**
     * Sets the delivery options for the listener.
     *
     * @param options the options
     * @return this builder
     */
    public Builder options(TransactionReceiptListenerOptions options) {
      this.options = options;
      return this;
    }

    /**
     * Builds the immutable {@link TransactionReceiptListener}.
     *
     * @return a new {@link TransactionReceiptListener} with the configured values
     */
    public TransactionReceiptListener build() {
      return new TransactionReceiptListener(name, null, started, filters, options);
    }
  }
}
