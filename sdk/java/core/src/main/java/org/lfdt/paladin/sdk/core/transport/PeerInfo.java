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
import java.util.Map;
import java.util.Objects;
import org.lfdt.paladin.sdk.core.types.Timestamp;

/**
 * Information about a peer node this node is connected to: its name, traffic statistics, and
 * current outbound transport details.
 *
 * <p>{@link #outboundTransport()}, {@link #outbound()}, and {@link #outboundError()} are present
 * only when there is an active outbound connection or a connection error; otherwise they are {@code
 * null}. Immutable.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
@JsonPropertyOrder({"name", "stats", "activated", "outboundTransport", "outbound", "outboundError"})
public final class PeerInfo {

  private final String name;
  private final PeerStats stats;
  private final Timestamp activated;
  private final String outboundTransport;
  private final Map<String, Object> outbound;
  private final JsonNode outboundError;

  @JsonCreator
  PeerInfo(
      @JsonProperty("name") final String name,
      @JsonProperty("stats") final PeerStats stats,
      @JsonProperty("activated") final Timestamp activated,
      @JsonProperty("outboundTransport") final String outboundTransport,
      @JsonProperty("outbound") final Map<String, Object> outbound,
      @JsonProperty("outboundError") final JsonNode outboundError) {
    this.name = name;
    this.stats = stats;
    this.activated = activated;
    this.outboundTransport = outboundTransport;
    this.outbound = outbound;
    this.outboundError = outboundError;
  }

  /**
   * The node name of the peer.
   *
   * @return the peer node name
   */
  @JsonProperty("name")
  public String name() {
    return name;
  }

  /**
   * Traffic counters and timestamps for the connection to this peer.
   *
   * @return the peer statistics
   */
  @JsonProperty("stats")
  public PeerStats stats() {
    return stats;
  }

  /**
   * When the peer connection was activated.
   *
   * @return the activation timestamp
   */
  @JsonProperty("activated")
  public Timestamp activated() {
    return activated;
  }

  /**
   * The name of the transport used for the outbound connection, or {@code null} if there is none.
   *
   * @return the outbound transport name, or {@code null}
   */
  @JsonProperty("outboundTransport")
  public String outboundTransport() {
    return outboundTransport;
  }

  /**
   * Transport-specific details of the outbound connection, or {@code null} if there is none.
   *
   * @return the outbound connection details, or {@code null}
   */
  @JsonProperty("outbound")
  public Map<String, Object> outbound() {
    return outbound;
  }

  /**
   * The most recent outbound connection error as raw JSON, or {@code null} if there is none.
   *
   * @return the outbound error, or {@code null}
   */
  @JsonProperty("outboundError")
  public JsonNode outboundError() {
    return outboundError;
  }

  @Override
  public boolean equals(final Object o) {
    if (this == o) {
      return true;
    }
    return o instanceof PeerInfo other
        && Objects.equals(name, other.name)
        && Objects.equals(stats, other.stats)
        && Objects.equals(activated, other.activated)
        && Objects.equals(outboundTransport, other.outboundTransport)
        && Objects.equals(outbound, other.outbound)
        && Objects.equals(outboundError, other.outboundError);
  }

  @Override
  public int hashCode() {
    return Objects.hash(name, stats, activated, outboundTransport, outbound, outboundError);
  }

  @Override
  public String toString() {
    return "PeerInfo{name=" + name + ", outboundTransport=" + outboundTransport + "}";
  }
}
