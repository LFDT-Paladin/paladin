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

class BlockchainEventListenerStatusTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsWithCheckpoint() throws Exception {
    final BlockchainEventListenerStatus status =
        new BlockchainEventListenerStatus(true, new BlockchainEventListenerCheckpoint(99L));

    final String json = MAPPER.writeValueAsString(status);
    final BlockchainEventListenerStatus parsed =
        MAPPER.readValue(json, BlockchainEventListenerStatus.class);

    assertTrue(parsed.catchup());
    assertEquals(99L, parsed.checkpoint().blockNumber());
    assertTrue(status.toString().contains("catchup=true"));
  }

  @Test
  void omitsNullCheckpoint() throws Exception {
    final BlockchainEventListenerStatus status = new BlockchainEventListenerStatus(false, null);
    final String json = MAPPER.writeValueAsString(status);
    assertFalse(json.contains("checkpoint"));

    final BlockchainEventListenerStatus parsed =
        MAPPER.readValue(json, BlockchainEventListenerStatus.class);
    assertFalse(parsed.catchup());
    assertNull(parsed.checkpoint());
  }
}
