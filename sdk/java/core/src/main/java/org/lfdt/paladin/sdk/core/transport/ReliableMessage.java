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
import com.fasterxml.jackson.databind.JsonNode;
import java.util.Objects;
import java.util.UUID;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * A message queued for reliable, acknowledged delivery to another node, as returned by {@code
 * transport_queryReliableMessages}. Immutable.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
@JsonPropertyOrder({"sequence", "id", "created", "node", "messageType", "metadata", "ack"})
public final class ReliableMessage {

  private final long sequence;
  private final UUID id;
  private final Timestamp created;
  private final String node;
  private final ReliableMessageType messageType;
  private final JsonNode metadata;
  private final ReliableMessageAckNoMsgID ack;

  @JsonCreator
  ReliableMessage(
      @JsonProperty("sequence") final long sequence,
      @JsonProperty("id") final UUID id,
      @JsonProperty("created") final Timestamp created,
      @JsonProperty("node") final String node,
      @JsonProperty("messageType") final ReliableMessageType messageType,
      @JsonProperty("metadata") final JsonNode metadata,
      @JsonProperty("ack") final ReliableMessageAckNoMsgID ack) {
    this.sequence = sequence;
    this.id = id;
    this.created = created;
    this.node = node;
    this.messageType = messageType;
    this.metadata = metadata;
    this.ack = ack;
  }

  /**
   * The local sequence number of this message.
   *
   * @return the sequence number
   */
  @JsonProperty("sequence")
  public long sequence() {
    return sequence;
  }

  /**
   * The unique identifier of this message.
   *
   * @return the message identifier
   */
  @JsonProperty("id")
  public UUID id() {
    return id;
  }

  /**
   * When the message was created.
   *
   * @return the creation timestamp
   */
  @JsonProperty("created")
  public Timestamp created() {
    return created;
  }

  /**
   * The node the message is destined for.
   *
   * @return the destination node id
   */
  @JsonProperty("node")
  public String node() {
    return node;
  }

  /**
   * The kind of payload this message carries.
   *
   * @return the message type
   */
  @JsonProperty("messageType")
  public ReliableMessageType messageType() {
    return messageType;
  }

  /**
   * Type-specific metadata for the message as raw JSON.
   *
   * @return the message metadata
   */
  @JsonProperty("metadata")
  public JsonNode metadata() {
    return metadata;
  }

  /**
   * The acknowledgement for this message, or {@code null} if it has not yet been acknowledged.
   *
   * @return the acknowledgement, or {@code null}
   */
  @JsonProperty("ack")
  public ReliableMessageAckNoMsgID ack() {
    return ack;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof ReliableMessage other
        && sequence == other.sequence
        && Objects.equals(id, other.id)
        && Objects.equals(created, other.created)
        && Objects.equals(node, other.node)
        && messageType == other.messageType
        && Objects.equals(metadata, other.metadata)
        && Objects.equals(ack, other.ack);
  }

  @Override
  public int hashCode() {
    return Objects.hash(sequence, id, created, node, messageType, metadata, ack);
  }

  @Override
  public String toString() {
    return "ReliableMessage{sequence="
        + sequence
        + ", id="
        + id
        + ", node="
        + node
        + ", messageType="
        + messageType
        + "}";
  }
}
