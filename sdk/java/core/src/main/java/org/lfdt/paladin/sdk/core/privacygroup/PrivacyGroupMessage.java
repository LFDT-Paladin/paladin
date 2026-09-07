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
package org.lfdt.paladin.sdk.core.privacygroup;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import com.fasterxml.jackson.databind.JsonNode;
import java.util.UUID;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * A privacy group message as recorded by the node. Immutable, server-assigned; send one with {@link
 * PrivacyGroupMessageInput}.
 *
 * <p>The message's own fields ({@link #correlationId()} through {@link #data()}) are flattened onto
 * the flat JSON wire form alongside the node-assigned delivery metadata. {@link #localSequence()}
 * orders messages on the receiving node and is what a listener checkpoints against.
 */
@JsonPropertyOrder({
  "id",
  "localSequence",
  "sent",
  "received",
  "node",
  "correlationId",
  "domain",
  "group",
  "topic",
  "data"
})
public final class PrivacyGroupMessage {

  private final UUID id;
  private final long localSequence;
  private final Timestamp sent;
  private final Timestamp received;
  private final String node;
  private final UUID correlationId;
  private final String domain;
  private final HexBytes group;
  private final String topic;
  private final JsonNode data;

  @JsonCreator
  PrivacyGroupMessage(
      @JsonProperty("id") final UUID id,
      @JsonProperty("localSequence") final long localSequence,
      @JsonProperty("sent") final Timestamp sent,
      @JsonProperty("received") final Timestamp received,
      @JsonProperty("node") final String node,
      @JsonProperty("correlationId") final UUID correlationId,
      @JsonProperty("domain") final String domain,
      @JsonProperty("group") final HexBytes group,
      @JsonProperty("topic") final String topic,
      @JsonProperty("data") final JsonNode data) {
    this.id = id;
    this.localSequence = localSequence;
    // The node sends a zero timestamp for "unset"; normalize to null to keep round-trips clean.
    this.sent = (sent == null || sent.isZero()) ? null : sent;
    this.received = (received == null || received.isZero()) ? null : received;
    this.node = node;
    this.correlationId = correlationId;
    this.domain = domain;
    this.group = group;
    this.topic = topic;
    this.data = data;
  }

  /**
   * The unique id of the message.
   *
   * @return the message id, or {@code null} if unset
   */
  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID id() {
    return id;
  }

  /**
   * The sequence of the message on the receiving node; a listener checkpoints against this.
   *
   * @return the local sequence
   */
  @JsonProperty("localSequence")
  public long localSequence() {
    return localSequence;
  }

  /**
   * The time the sending node sent the message.
   *
   * @return the sent timestamp, or {@code null} if unset
   */
  @JsonProperty("sent")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp sent() {
    return sent;
  }

  /**
   * The time this node received the message.
   *
   * @return the received timestamp, or {@code null} if unset
   */
  @JsonProperty("received")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public Timestamp received() {
    return received;
  }

  /**
   * The node that sent the message.
   *
   * @return the sending node name, or an empty string when unset
   */
  @JsonProperty("node")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String node() {
    return node;
  }

  /**
   * The id of an earlier message this one responds to.
   *
   * @return the correlation id, or {@code null} if unset
   */
  @JsonProperty("correlationId")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public UUID correlationId() {
    return correlationId;
  }

  /**
   * The domain the group belongs to.
   *
   * @return the domain name, or an empty string when unset
   */
  @JsonProperty("domain")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String domain() {
    return domain;
  }

  /**
   * The identifier of the group the message was sent to.
   *
   * @return the group id, or {@code null} if unset
   */
  @JsonProperty("group")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public HexBytes group() {
    return group;
  }

  /**
   * The free-form topic that message listeners filter on.
   *
   * @return the topic, or an empty string when unset
   */
  @JsonProperty("topic")
  @JsonInclude(JsonInclude.Include.NON_EMPTY)
  public String topic() {
    return topic;
  }

  /**
   * The message payload, as arbitrary JSON.
   *
   * @return the payload, or {@code null} if unset
   */
  @JsonProperty("data")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode data() {
    return data;
  }

  @Override
  public String toString() {
    return "PrivacyGroupMessage{id="
        + id
        + ", localSequence="
        + localSequence
        + ", group="
        + group
        + ", topic="
        + topic
        + "}";
  }
}
