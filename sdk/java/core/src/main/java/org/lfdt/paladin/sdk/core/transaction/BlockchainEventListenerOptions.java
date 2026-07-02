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
import com.fasterxml.jackson.databind.JsonNode;

// Immutable; mirrors pldapi.BlockchainEventListenerOptions (batching and start-block options for a
// blockchain-event listener). fromBlock mirrors Go json.RawMessage (a block number or a special
// string such as "latest") and is surfaced as raw JSON. All fields follow the Go omitempty
// convention.
@JsonPropertyOrder({"batchSize", "batchTimeout", "fromBlock"})
public final class BlockchainEventListenerOptions {

  private final Integer batchSize;
  private final String batchTimeout;
  private final JsonNode fromBlock;

  @JsonCreator
  BlockchainEventListenerOptions(
      @JsonProperty("batchSize") Integer batchSize,
      @JsonProperty("batchTimeout") String batchTimeout,
      @JsonProperty("fromBlock") JsonNode fromBlock) {
    this.batchSize = batchSize;
    this.batchTimeout = batchTimeout;
    this.fromBlock = fromBlock;
  }

  @JsonProperty("batchSize")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Integer batchSize() {
    return batchSize;
  }

  @JsonProperty("batchTimeout")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String batchTimeout() {
    return batchTimeout;
  }

  @JsonProperty("fromBlock")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode fromBlock() {
    return fromBlock;
  }

  public static Builder builder() {
    return new Builder();
  }

  @Override
  public String toString() {
    return "BlockchainEventListenerOptions{batchSize="
        + batchSize
        + ", batchTimeout="
        + batchTimeout
        + "}";
  }

  /** Fluent builder for {@link BlockchainEventListenerOptions}. */
  public static final class Builder {
    private Integer batchSize;
    private String batchTimeout;
    private JsonNode fromBlock;

    private Builder() {}

    public Builder batchSize(Integer batchSize) {
      this.batchSize = batchSize;
      return this;
    }

    public Builder batchTimeout(String batchTimeout) {
      this.batchTimeout = batchTimeout;
      return this;
    }

    public Builder fromBlock(JsonNode fromBlock) {
      this.fromBlock = fromBlock;
      return this;
    }

    public BlockchainEventListenerOptions build() {
      return new BlockchainEventListenerOptions(batchSize, batchTimeout, fromBlock);
    }
  }
}
