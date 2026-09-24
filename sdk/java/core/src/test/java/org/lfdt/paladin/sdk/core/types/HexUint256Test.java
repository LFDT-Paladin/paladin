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

import com.fasterxml.jackson.databind.ObjectMapper;
import java.math.BigInteger;
import org.junit.jupiter.api.Test;

class HexUint256Test {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void formatsAsEvenLength0xHex() {
    assertEquals("0xff", HexUint256.of(255).to0xHex());
    assertEquals("0x0100", HexUint256.of(256).to0xHex()); // odd nibble count gets zero-padded
    assertEquals("0x00", HexUint256.of(0).to0xHex());
  }

  @Test
  void serializesAsString() throws Exception {
    assertEquals("\"0xff\"", MAPPER.writeValueAsString(HexUint256.of(255)));
  }

  @Test
  void deserializesFromHexString() throws Exception {
    assertEquals(HexUint256.of(255), MAPPER.readValue("\"0xff\"", HexUint256.class));
  }

  @Test
  void deserializesFromDecimalString() throws Exception {
    assertEquals(HexUint256.of(255), MAPPER.readValue("\"255\"", HexUint256.class));
  }

  @Test
  void deserializesFromJsonNumberWithoutPrecisionLoss() throws Exception {
    final BigInteger big = BigInteger.TWO.pow(200).add(BigInteger.ONE);
    assertEquals(HexUint256.of(big), MAPPER.readValue(big.toString(), HexUint256.class));
  }

  @Test
  void roundTripsThroughJson() throws Exception {
    final HexUint256 original = HexUint256.of(BigInteger.TWO.pow(255));
    assertEquals(original, MAPPER.readValue(MAPPER.writeValueAsString(original), HexUint256.class));
  }
}
