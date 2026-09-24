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
package org.lfdt.paladin.sdk.core.blockindex;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/** A block as recorded by the node's block indexer. Immutable. */
@JsonPropertyOrder({"number", "hash", "timestamp"})
public final class IndexedBlock {

  private final long number;
  private final Bytes32 hash;
  private final Timestamp timestamp;

  @JsonCreator
  IndexedBlock(
      @JsonProperty("number") final long number,
      @JsonProperty("hash") final Bytes32 hash,
      @JsonProperty("timestamp") final Timestamp timestamp) {
    this.number = number;
    this.hash = hash;
    this.timestamp = timestamp;
  }

  /**
   * The block number.
   *
   * @return the block number
   */
  @JsonProperty("number")
  public long number() {
    return number;
  }

  /**
   * The block hash.
   *
   * @return the block hash
   */
  @JsonProperty("hash")
  public Bytes32 hash() {
    return hash;
  }

  /**
   * The block timestamp.
   *
   * @return the block timestamp
   */
  @JsonProperty("timestamp")
  public Timestamp timestamp() {
    return timestamp;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof IndexedBlock other
        && number == other.number
        && Objects.equals(hash, other.hash)
        && Objects.equals(timestamp, other.timestamp);
  }

  @Override
  public int hashCode() {
    return Objects.hash(number, hash, timestamp);
  }

  @Override
  public String toString() {
    return "IndexedBlock{number=" + number + ", hash=" + hash + ", timestamp=" + timestamp + "}";
  }
}
