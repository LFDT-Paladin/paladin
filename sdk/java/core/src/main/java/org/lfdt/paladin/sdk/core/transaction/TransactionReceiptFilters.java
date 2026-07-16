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

/**
 * The filters that select which receipts a listener delivers. Immutable; build one with the
 * {@linkplain #builder() fluent builder} to configure a receipt listener. All fields are optional
 * and omitted from the JSON form when unset.
 */
@JsonPropertyOrder({"sequenceAbove", "type", "domain"})
public final class TransactionReceiptFilters {

  private final Long sequenceAbove;
  private final TransactionType type;
  private final String domain;

  @JsonCreator
  TransactionReceiptFilters(
      @JsonProperty("sequenceAbove") final Long sequenceAbove,
      @JsonProperty("type") final TransactionType type,
      @JsonProperty("domain") final String domain) {
    this.sequenceAbove = sequenceAbove;
    this.type = type;
    this.domain = domain;
  }

  /**
   * Delivers only receipts with a sequence above this value.
   *
   * @return the sequence lower bound, or {@code null} if unset
   */
  @JsonProperty("sequenceAbove")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Long sequenceAbove() {
    return sequenceAbove;
  }

  /**
   * Delivers only receipts for transactions of this type.
   *
   * @return the transaction type, or {@code null} if unset
   */
  @JsonProperty("type")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public TransactionType type() {
    return type;
  }

  /**
   * Delivers only receipts for transactions in this domain.
   *
   * @return the domain name, or an empty string when unset
   */
  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
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
    return "TransactionReceiptFilters{sequenceAbove="
        + sequenceAbove
        + ", type="
        + type
        + ", domain="
        + domain
        + "}";
  }

  /** Fluent builder for {@link TransactionReceiptFilters}. */
  public static final class Builder {
    private Long sequenceAbove;
    private TransactionType type;
    private String domain;

    private Builder() {}

    /**
     * Delivers only receipts with a sequence above this value.
     *
     * @param sequenceAbove the sequence lower bound
     * @return this builder
     */
    public Builder sequenceAbove(final Long sequenceAbove) {
      this.sequenceAbove = sequenceAbove;
      return this;
    }

    /**
     * Delivers only receipts for transactions of this type.
     *
     * @param type the transaction type
     * @return this builder
     */
    public Builder type(final TransactionType type) {
      this.type = type;
      return this;
    }

    /**
     * Delivers only receipts for transactions in this domain.
     *
     * @param domain the domain name
     * @return this builder
     */
    public Builder domain(final String domain) {
      this.domain = domain;
      return this;
    }

    /**
     * Builds the immutable {@link TransactionReceiptFilters}.
     *
     * @return a new {@link TransactionReceiptFilters} with the configured values
     */
    public TransactionReceiptFilters build() {
      return new TransactionReceiptFilters(sequenceAbove, type, domain);
    }
  }
}
