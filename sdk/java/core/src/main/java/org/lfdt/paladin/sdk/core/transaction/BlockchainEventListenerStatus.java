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
 * The runtime status of a blockchain-event listener — whether it is catching up and its current
 * checkpoint, mirroring {@code pldapi.BlockchainEventListenerStatus}. Immutable. Returned by {@code
 * ptx_getBlockchainEventListenerStatus}.
 */
@JsonPropertyOrder({"catchup", "checkpoint"})
public final class BlockchainEventListenerStatus {

  private final boolean catchup;
  private final BlockchainEventListenerCheckpoint checkpoint;

  @JsonCreator
  BlockchainEventListenerStatus(
      @JsonProperty("catchup") final boolean catchup,
      @JsonProperty("checkpoint") final BlockchainEventListenerCheckpoint checkpoint) {
    this.catchup = catchup;
    this.checkpoint = checkpoint;
  }

  /**
   * Whether the listener is still catching up to the head of the chain.
   *
   * @return {@code true} if the listener is catching up
   */
  @JsonProperty("catchup")
  public boolean catchup() {
    return catchup;
  }

  /**
   * The listener's current checkpoint.
   *
   * @return the checkpoint, or {@code null} if unset
   */
  @JsonProperty("checkpoint")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public BlockchainEventListenerCheckpoint checkpoint() {
    return checkpoint;
  }

  @Override
  public String toString() {
    return "BlockchainEventListenerStatus{catchup=" + catchup + ", checkpoint=" + checkpoint + "}";
  }
}
