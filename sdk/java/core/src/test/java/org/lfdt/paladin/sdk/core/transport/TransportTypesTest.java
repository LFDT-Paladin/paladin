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

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;

class TransportTypesTest {

  private static final ObjectMapper MAPPER = PaladinObjectMapper.shared();

  private static final String PEER_JSON =
      "{\"name\":\"node2\",\"activated\":\"2024-01-01T00:00:00Z\","
          + "\"outboundTransport\":\"grpc\",\"outbound\":{\"endpoint\":\"dns:node2:9000\"},"
          + "\"outboundError\":{\"message\":\"boom\"},"
          + "\"stats\":{\"createdAt\":\"2024-01-01T00:00:00Z\",\"sentMsgs\":5,\"receivedMsgs\":3,"
          + "\"sentBytes\":100,\"receivedBytes\":80,\"lastSend\":\"2024-01-02T00:00:00Z\","
          + "\"lastReceive\":\"2024-01-03T00:00:00Z\",\"reliableHighestSent\":5,"
          + "\"reliableAckBase\":2}}";

  private static final String RELIABLE_MSG_JSON =
      "{\"sequence\":7,\"id\":\"3b8c1a2e-0000-0000-0000-000000000001\","
          + "\"created\":\"2024-01-01T00:00:00Z\",\"node\":\"node2\",\"messageType\":\"state\","
          + "\"metadata\":{\"foo\":\"bar\"},"
          + "\"ack\":{\"time\":\"2024-01-01T00:00:01Z\",\"error\":\"oops\"}}";

  @Test
  void reliableMessageTypeRoundTripsAndRejectsUnknown() throws Exception {
    for (final ReliableMessageType t : ReliableMessageType.values()) {
      assertEquals(t, MAPPER.readValue(MAPPER.writeValueAsString(t), ReliableMessageType.class));
    }
    assertEquals("prepared_txn", ReliableMessageType.PREPARED_TRANSACTION.jsonValue());
    assertEquals(ReliableMessageType.STATE, ReliableMessageType.fromJson("STATE"));
    assertThrows(IllegalArgumentException.class, () -> ReliableMessageType.fromJson("bogus"));
    assertThrows(IllegalArgumentException.class, () -> ReliableMessageType.fromJson(null));
  }

  @Test
  void peerInfoRoundTrips() throws Exception {
    final PeerInfo peer = MAPPER.readValue(PEER_JSON, PeerInfo.class);
    assertEquals("node2", peer.name());
    assertEquals("grpc", peer.outboundTransport());
    assertEquals("dns:node2:9000", peer.outbound().get("endpoint"));
    assertEquals("boom", peer.outboundError().get("message").asText());
    assertTrue(peer.activated().toString().startsWith("2024-01-01"));

    final PeerStats stats = peer.stats();
    assertEquals(5L, stats.sentMsgs());
    assertEquals(3L, stats.receivedMsgs());
    assertEquals(100L, stats.sentBytes());
    assertEquals(80L, stats.receivedBytes());
    assertEquals(5L, stats.reliableHighestSent());
    assertEquals(2L, stats.reliableAckBase());
    assertTrue(stats.createdAt().toString().startsWith("2024-01-01"));
    assertTrue(stats.lastSend().toString().startsWith("2024-01-02"));
    assertTrue(stats.lastReceive().toString().startsWith("2024-01-03"));

    final PeerInfo reparsed = MAPPER.readValue(MAPPER.writeValueAsString(peer), PeerInfo.class);
    assertEquals(peer, reparsed);
    assertEquals(peer.hashCode(), reparsed.hashCode());
    assertEquals(peer, peer);
    assertNotEquals(peer, "not a peer");
    assertEquals(stats, reparsed.stats());
    assertEquals(stats.hashCode(), reparsed.stats().hashCode());
    assertNotEquals(stats, "not stats");
    assertTrue(peer.toString().contains("name=node2"));
    assertTrue(stats.toString().contains("sentMsgs=5"));
  }

  @Test
  void reliableMessageRoundTrips() throws Exception {
    final ReliableMessage msg = MAPPER.readValue(RELIABLE_MSG_JSON, ReliableMessage.class);
    assertEquals(7L, msg.sequence());
    assertEquals("node2", msg.node());
    assertEquals(ReliableMessageType.STATE, msg.messageType());
    assertEquals("bar", msg.metadata().get("foo").asText());
    assertEquals("oops", msg.ack().error());
    assertTrue(msg.ack().time().toString().startsWith("2024-01-01"));

    final ReliableMessage reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(msg), ReliableMessage.class);
    assertEquals(msg, reparsed);
    assertEquals(msg.hashCode(), reparsed.hashCode());
    assertEquals(msg, msg);
    assertNotEquals(msg, "not a message");
    assertEquals(msg.ack(), reparsed.ack());
    assertEquals(msg.ack().hashCode(), reparsed.ack().hashCode());
    assertNotEquals(msg.ack(), "not an ack");
    assertTrue(msg.toString().contains("node=node2"));
    assertTrue(msg.ack().toString().contains("error=oops"));
  }

  @Test
  void reliableMessageAckRoundTrips() throws Exception {
    final String json =
        "{\"messageId\":\"3b8c1a2e-0000-0000-0000-000000000001\","
            + "\"time\":\"2024-01-01T00:00:01Z\",\"error\":\"boom\"}";
    final ReliableMessageAck ack = MAPPER.readValue(json, ReliableMessageAck.class);
    assertEquals("boom", ack.error());
    assertTrue(ack.time().toString().startsWith("2024-01-01"));
    assertEquals(
        java.util.UUID.fromString("3b8c1a2e-0000-0000-0000-000000000001"), ack.messageId());

    final ReliableMessageAck reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(ack), ReliableMessageAck.class);
    assertEquals(ack, reparsed);
    assertEquals(ack.hashCode(), reparsed.hashCode());
    assertEquals(ack, ack);
    assertNotEquals(ack, "not an ack");
    assertTrue(ack.toString().contains("error=boom"));
  }
}
