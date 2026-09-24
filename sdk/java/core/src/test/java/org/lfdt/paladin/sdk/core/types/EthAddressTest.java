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
package org.lfdt.paladin.sdk.core.types;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

class EthAddressTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();
  private static final String ADDR = "0x5e7e6322f3f6cc8fc94f5f6f3f6cc8fc94f5f6f3";

  @Test
  void serializesAsLowerCase0xHex() throws Exception {
    final EthAddress addr = EthAddress.fromString("0x5E7E6322F3F6CC8FC94F5F6F3F6CC8FC94F5F6F3");
    assertEquals("\"" + ADDR + "\"", MAPPER.writeValueAsString(addr));
  }

  @Test
  void roundTripsThroughJson() throws Exception {
    final EthAddress original = EthAddress.fromString(ADDR);
    final String json = MAPPER.writeValueAsString(original);
    assertEquals(original, MAPPER.readValue(json, EthAddress.class));
  }

  @Test
  void parsesWithoutPrefix() {
    assertEquals(EthAddress.fromString(ADDR), EthAddress.fromString(ADDR.substring(2)));
  }

  @Test
  void rejectsWrongLength() {
    assertThrows(IllegalArgumentException.class, () -> EthAddress.fromString("0xdeadbeef"));
    assertThrows(IllegalArgumentException.class, () -> EthAddress.wrap(new byte[19]));
  }

  @Test
  void zeroDetection() {
    assertTrue(EthAddress.wrap(new byte[20]).isZero());
    assertFalse(EthAddress.fromString(ADDR).isZero());
  }
}
