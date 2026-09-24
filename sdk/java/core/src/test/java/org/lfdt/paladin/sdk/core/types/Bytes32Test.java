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
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

class Bytes32Test {

  private static final ObjectMapper MAPPER = new ObjectMapper();
  private static final String HASH =
      "0x1122334455667788990011223344556677889900112233445566778899001122";

  @Test
  void serializesAsLowerCase0xHash() throws Exception {
    assertEquals("\"" + HASH + "\"", MAPPER.writeValueAsString(Bytes32.fromString(HASH)));
  }

  @Test
  void roundTripsThroughJson() throws Exception {
    final Bytes32 original = Bytes32.fromString(HASH);
    final String json = MAPPER.writeValueAsString(original);
    assertEquals(original, MAPPER.readValue(json, Bytes32.class));
  }

  @Test
  void rejectsWrongLength() {
    assertThrows(IllegalArgumentException.class, () -> Bytes32.fromString(HASH + "00"));
    assertThrows(IllegalArgumentException.class, () -> Bytes32.wrap(new byte[31]));
  }

  @Test
  void zeroDetection() {
    assertTrue(Bytes32.wrap(new byte[32]).isZero());
  }
}
