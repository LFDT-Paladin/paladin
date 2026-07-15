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
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.types.EthAddress;

class BlockchainEventListenerSourceTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void builderRoundTripsAbiAndAddress() throws Exception {
    final EthAddress address = EthAddress.fromString("0x05d936207F04D81a85881b72A0D17854Ee8BE45A");
    final AbiEntry event = AbiEntry.event("Transfer").build();
    final BlockchainEventListenerSource source =
        BlockchainEventListenerSource.builder().abiEntry(event).address(address).build();

    final String json = MAPPER.writeValueAsString(source);
    final BlockchainEventListenerSource parsed =
        MAPPER.readValue(json, BlockchainEventListenerSource.class);

    assertEquals(1, parsed.abi().size());
    assertEquals("Transfer", parsed.abi().get(0).name());
    assertEquals(address, parsed.address());
    assertTrue(source.toString().contains("entries=1"));
  }

  @Test
  void builderAbiListAppends() {
    final AbiEntry e1 = AbiEntry.event("A").build();
    final AbiEntry e2 = AbiEntry.event("B").build();
    final BlockchainEventListenerSource source =
        BlockchainEventListenerSource.builder().abi(java.util.List.of(e1, e2)).build();
    assertEquals(2, source.abi().size());
    assertNull(source.address());
  }

  @Test
  void defaultsToEmptyAbiWhenNull() {
    final BlockchainEventListenerSource source = new BlockchainEventListenerSource(null, null);
    assertTrue(source.abi().isEmpty());
  }
}
