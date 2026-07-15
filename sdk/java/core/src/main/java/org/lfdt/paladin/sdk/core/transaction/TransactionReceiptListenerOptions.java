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
 * The delivery options for a receipt listener, mirroring {@code
 * pldapi.TransactionReceiptListenerOptions}. Immutable; build one with the {@linkplain #builder()
 * fluent builder}.
 *
 * <p>{@link #incompleteStateReceiptBehavior()} mirrors the Go {@code
 * Enum[IncompleteStateReceiptBehavior]} and is one of {@code "block_contract"} (default), {@code
 * "process"}, or {@code "complete_only"}.
 */
@JsonPropertyOrder({"domainReceipts", "incompleteStateReceiptBehavior"})
public final class TransactionReceiptListenerOptions {

  private final boolean domainReceipts;
  private final String incompleteStateReceiptBehavior;

  @JsonCreator
  TransactionReceiptListenerOptions(
      @JsonProperty("domainReceipts") final boolean domainReceipts,
      @JsonProperty("incompleteStateReceiptBehavior") final String incompleteStateReceiptBehavior) {
    this.domainReceipts = domainReceipts;
    this.incompleteStateReceiptBehavior = incompleteStateReceiptBehavior;
  }

  /**
   * Whether domain receipts are included in the delivered receipts.
   *
   * @return {@code true} if domain receipts are included
   */
  @JsonProperty("domainReceipts")
  public boolean domainReceipts() {
    return domainReceipts;
  }

  /**
   * How the listener behaves when a state receipt is incomplete.
   *
   * @return the incomplete-state-receipt behavior, or an empty string when unset
   */
  @JsonProperty("incompleteStateReceiptBehavior")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String incompleteStateReceiptBehavior() {
    return incompleteStateReceiptBehavior;
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
    return "TransactionReceiptListenerOptions{domainReceipts="
        + domainReceipts
        + ", incompleteStateReceiptBehavior="
        + incompleteStateReceiptBehavior
        + "}";
  }

  /** Fluent builder for {@link TransactionReceiptListenerOptions}. */
  public static final class Builder {
    private boolean domainReceipts;
    private String incompleteStateReceiptBehavior;

    private Builder() {}

    /**
     * Sets whether domain receipts are included in the delivered receipts.
     *
     * @param domainReceipts whether to include domain receipts
     * @return this builder
     */
    public Builder domainReceipts(final boolean domainReceipts) {
      this.domainReceipts = domainReceipts;
      return this;
    }

    /**
     * Sets how the listener behaves when a state receipt is incomplete.
     *
     * @param incompleteStateReceiptBehavior one of {@code "block_contract"}, {@code "process"}, or
     *     {@code "complete_only"}
     * @return this builder
     */
    public Builder incompleteStateReceiptBehavior(final String incompleteStateReceiptBehavior) {
      this.incompleteStateReceiptBehavior = incompleteStateReceiptBehavior;
      return this;
    }

    /**
     * Builds the immutable {@link TransactionReceiptListenerOptions}.
     *
     * @return a new {@link TransactionReceiptListenerOptions} with the configured values
     */
    public TransactionReceiptListenerOptions build() {
      return new TransactionReceiptListenerOptions(domainReceipts, incompleteStateReceiptBehavior);
    }
  }
}
