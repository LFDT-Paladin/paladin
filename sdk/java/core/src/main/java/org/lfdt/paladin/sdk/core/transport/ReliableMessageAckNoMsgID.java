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
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * The acknowledgement embedded in a {@link ReliableMessage}, without the message identifier (which
 * is implied by the enclosing message). Immutable.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
@JsonPropertyOrder({"time", "error"})
public final class ReliableMessageAckNoMsgID {

  private final Timestamp time;
  private final String error;

  @JsonCreator
  ReliableMessageAckNoMsgID(
      @JsonProperty("time") final Timestamp time, @JsonProperty("error") final String error) {
    this.time = time;
    this.error = error;
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
    return o instanceof ReliableMessageAckNoMsgID other
        && Objects.equals(time, other.time)
        && Objects.equals(error, other.error);
  }

  @Override
  public int hashCode() {
    return Objects.hash(time, error);
  }

  @Override
  public String toString() {
    return "ReliableMessageAckNoMsgID{time=" + time + ", error=" + error + "}";
  }
}
