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
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/** Traffic counters and timestamps for a peer connection. Immutable. */
@JsonPropertyOrder({
  "createdAt",
  "sentMsgs",
  "receivedMsgs",
  "sentBytes",
  "receivedBytes",
  "lastSend",
  "lastReceive",
  "reliableHighestSent",
  "reliableAckBase"
})
public final class PeerStats {

  private final Timestamp createdAt;
  private final long sentMsgs;
  private final long receivedMsgs;
  private final long sentBytes;
  private final long receivedBytes;
  private final Timestamp lastSend;
  private final Timestamp lastReceive;
  private final long reliableHighestSent;
  private final long reliableAckBase;

  @JsonCreator
  PeerStats(
      @JsonProperty("createdAt") final Timestamp createdAt,
      @JsonProperty("sentMsgs") final long sentMsgs,
      @JsonProperty("receivedMsgs") final long receivedMsgs,
      @JsonProperty("sentBytes") final long sentBytes,
      @JsonProperty("receivedBytes") final long receivedBytes,
      @JsonProperty("lastSend") final Timestamp lastSend,
      @JsonProperty("lastReceive") final Timestamp lastReceive,
      @JsonProperty("reliableHighestSent") final long reliableHighestSent,
      @JsonProperty("reliableAckBase") final long reliableAckBase) {
    this.createdAt = createdAt;
    this.sentMsgs = sentMsgs;
    this.receivedMsgs = receivedMsgs;
    this.sentBytes = sentBytes;
    this.receivedBytes = receivedBytes;
    this.lastSend = lastSend;
    this.lastReceive = lastReceive;
    this.reliableHighestSent = reliableHighestSent;
    this.reliableAckBase = reliableAckBase;
  }

  /**
   * When the peer connection was first established, or {@code null} if not recorded.
   *
   * @return the creation timestamp, or {@code null}
   */
  @JsonProperty("createdAt")
  public Timestamp createdAt() {
    return createdAt;
  }

  /**
   * The number of messages sent to the peer.
   *
   * @return the sent message count
   */
  @JsonProperty("sentMsgs")
  public long sentMsgs() {
    return sentMsgs;
  }

  /**
   * The number of messages received from the peer.
   *
   * @return the received message count
   */
  @JsonProperty("receivedMsgs")
  public long receivedMsgs() {
    return receivedMsgs;
  }

  /**
   * The number of bytes sent to the peer.
   *
   * @return the sent byte count
   */
  @JsonProperty("sentBytes")
  public long sentBytes() {
    return sentBytes;
  }

  /**
   * The number of bytes received from the peer.
   *
   * @return the received byte count
   */
  @JsonProperty("receivedBytes")
  public long receivedBytes() {
    return receivedBytes;
  }

  /**
   * When a message was last sent to the peer, or {@code null} if none has been sent.
   *
   * @return the last send timestamp, or {@code null}
   */
  @JsonProperty("lastSend")
  public Timestamp lastSend() {
    return lastSend;
  }

  /**
   * When a message was last received from the peer, or {@code null} if none has been received.
   *
   * @return the last receive timestamp, or {@code null}
   */
  @JsonProperty("lastReceive")
  public Timestamp lastReceive() {
    return lastReceive;
  }

  /**
   * The highest sequence number sent reliably to the peer.
   *
   * @return the highest reliably-sent sequence
   */
  @JsonProperty("reliableHighestSent")
  public long reliableHighestSent() {
    return reliableHighestSent;
  }

  /**
   * The base sequence number up to which reliable messages have been acknowledged.
   *
   * @return the reliable acknowledgement base
   */
  @JsonProperty("reliableAckBase")
  public long reliableAckBase() {
    return reliableAckBase;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof PeerStats other
        && sentMsgs == other.sentMsgs
        && receivedMsgs == other.receivedMsgs
        && sentBytes == other.sentBytes
        && receivedBytes == other.receivedBytes
        && reliableHighestSent == other.reliableHighestSent
        && reliableAckBase == other.reliableAckBase
        && Objects.equals(createdAt, other.createdAt)
        && Objects.equals(lastSend, other.lastSend)
        && Objects.equals(lastReceive, other.lastReceive);
  }

  @Override
  public int hashCode() {
    return Objects.hash(
        createdAt,
        sentMsgs,
        receivedMsgs,
        sentBytes,
        receivedBytes,
        lastSend,
        lastReceive,
        reliableHighestSent,
        reliableAckBase);
  }

  @Override
  public String toString() {
    return "PeerStats{sentMsgs="
        + sentMsgs
        + ", receivedMsgs="
        + receivedMsgs
        + ", sentBytes="
        + sentBytes
        + ", receivedBytes="
        + receivedBytes
        + "}";
  }
}
