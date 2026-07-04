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

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.types.Timestamp;

class BlockchainEventListenerTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void builderCreatesInputWithoutCreatedTimestamp() throws Exception {
    BlockchainEventListenerSource source =
        BlockchainEventListenerSource.builder().abiEntry(AbiEntry.event("Transfer").build()).build();
    BlockchainEventListener listener =
        BlockchainEventListener.builder()
            .name("my-listener")
            .started(true)
            .source(source)
            .options(BlockchainEventListenerOptions.builder().batchSize(10).build())
            .build();

    assertNull(listener.created());
    String json = MAPPER.writeValueAsString(listener);
    assertFalse(json.contains("created"));

    BlockchainEventListener parsed = MAPPER.readValue(json, BlockchainEventListener.class);
    assertEquals("my-listener", parsed.name());
    assertTrue(parsed.started());
    assertEquals(1, parsed.sources().size());
    assertEquals(10, parsed.options().batchSize());
  }

  @Test
  void builderSourcesListAppends() {
    BlockchainEventListenerSource s1 = BlockchainEventListenerSource.builder().build();
    BlockchainEventListenerSource s2 = BlockchainEventListenerSource.builder().build();
    BlockchainEventListener listener =
        BlockchainEventListener.builder().name("l").sources(java.util.List.of(s1, s2)).build();
    assertEquals(2, listener.sources().size());
  }

  @Test
  void parsesServerResponseWithCreatedTimestamp() throws Exception {
    String json =
        "{\"name\":\"l1\",\"created\":\"2024-06-18T12:00:00Z\",\"started\":false,\"sources\":[]}";
    BlockchainEventListener listener = MAPPER.readValue(json, BlockchainEventListener.class);

    assertEquals("l1", listener.name());
    assertEquals(Timestamp.fromString("2024-06-18T12:00:00Z"), listener.created());
    assertFalse(listener.started());
    assertTrue(listener.sources().isEmpty());
    assertTrue(listener.toString().contains("l1"));
  }

  @Test
  void zeroTimestampIsNormalizedToNull() {
    BlockchainEventListener listener =
        new BlockchainEventListener("l", Timestamp.ZERO, null, null, null);
    assertNull(listener.created());
  }
}
