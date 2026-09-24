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
package org.lfdt.paladin.sdk.core.transport;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.Objects;
import java.util.UUID;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * A standalone acknowledgement of a reliable message, as returned by {@code
 * transport_queryReliableMessageAcks}. Immutable.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
@JsonPropertyOrder({"messageId", "time", "error"})
public final class ReliableMessageAck {

  private final UUID messageId;
  private final Timestamp time;
  private final String error;

  @JsonCreator
  ReliableMessageAck(
      @JsonProperty("messageId") final UUID messageId,
      @JsonProperty("time") final Timestamp time,
      @JsonProperty("error") final String error) {
    this.messageId = messageId;
    this.time = time;
    this.error = error;
  }

  /**
   * The identifier of the message this acknowledgement refers to.
   *
   * @return the message identifier
   */
  @JsonProperty("messageId")
  public UUID messageId() {
    return messageId;
  }

  /**
   * When the acknowledgement was recorded, or {@code null} if not set.
   *
   * @return the acknowledgement timestamp, or {@code null}
   */
  @JsonProperty("time")
  public Timestamp time() {
    return time;
  }

  /**
   * The error recorded against the message, or {@code null} if it was acknowledged successfully.
   *
   * @return the error string, or {@code null}
   */
  @JsonProperty("error")
  public String error() {
    return error;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof ReliableMessageAck other
        && Objects.equals(messageId, other.messageId)
        && Objects.equals(time, other.time)
        && Objects.equals(error, other.error);
  }

  @Override
  public int hashCode() {
    return Objects.hash(messageId, time, error);
  }

  @Override
  public String toString() {
    return "ReliableMessageAck{messageId="
        + messageId
        + ", time="
        + time
        + ", error="
        + error
        + "}";
  }
}
