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

// Immutable; mirrors pldapi.TransactionReceiptFilters (the receipts a listener delivers). All fields
// follow the Go omitempty convention. Build with the fluent builder to configure a receipt listener.
@JsonPropertyOrder({"sequenceAbove", "type", "domain"})
public final class TransactionReceiptFilters {

  private final Long sequenceAbove;
  private final TransactionType type;
  private final String domain;

  @JsonCreator
  TransactionReceiptFilters(
      @JsonProperty("sequenceAbove") Long sequenceAbove,
      @JsonProperty("type") TransactionType type,
      @JsonProperty("domain") String domain) {
    this.sequenceAbove = sequenceAbove;
    this.type = type;
    this.domain = domain;
  }

  @JsonProperty("sequenceAbove")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Long sequenceAbove() {
    return sequenceAbove;
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

    public Builder sequenceAbove(Long sequenceAbove) {
      this.sequenceAbove = sequenceAbove;
      return this;
    }

    public Builder type(TransactionType type) {
      this.type = type;
      return this;
    }

    public Builder domain(String domain) {
      this.domain = domain;
      return this;
    }

    public TransactionReceiptFilters build() {
      return new TransactionReceiptFilters(sequenceAbove, type, domain);
    }
  }
}
