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

// Immutable; mirrors pldapi.TransactionReceiptListenerOptions (delivery options for a receipt
// listener). incompleteStateReceiptBehavior mirrors the Go Enum[IncompleteStateReceiptBehavior] and
// is one of "block_contract" (default), "process" or "complete_only".
@JsonPropertyOrder({"domainReceipts", "incompleteStateReceiptBehavior"})
public final class TransactionReceiptListenerOptions {

  private final boolean domainReceipts;
  private final String incompleteStateReceiptBehavior;

  @JsonCreator
  TransactionReceiptListenerOptions(
      @JsonProperty("domainReceipts") boolean domainReceipts,
      @JsonProperty("incompleteStateReceiptBehavior") String incompleteStateReceiptBehavior) {
    this.domainReceipts = domainReceipts;
    this.incompleteStateReceiptBehavior = incompleteStateReceiptBehavior;
  }

  @JsonProperty("domainReceipts")
  public boolean domainReceipts() {
    return domainReceipts;
  }

  @JsonProperty("incompleteStateReceiptBehavior")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String incompleteStateReceiptBehavior() {
    return incompleteStateReceiptBehavior;
  }

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

    public Builder domainReceipts(boolean domainReceipts) {
      this.domainReceipts = domainReceipts;
      return this;
    }

    public Builder incompleteStateReceiptBehavior(String incompleteStateReceiptBehavior) {
      this.incompleteStateReceiptBehavior = incompleteStateReceiptBehavior;
      return this;
    }

    public TransactionReceiptListenerOptions build() {
      return new TransactionReceiptListenerOptions(domainReceipts, incompleteStateReceiptBehavior);
    }
  }
}
