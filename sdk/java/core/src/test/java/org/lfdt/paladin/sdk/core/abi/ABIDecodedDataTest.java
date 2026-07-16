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
package org.lfdt.paladin.sdk.core.abi;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

class ABIDecodedDataTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsFullDecodedError() throws Exception {
    final AbiEntry definition =
        AbiEntry.error("InsufficientBalance")
            .input(AbiParameter.of("available", "uint256"))
            .build();
    final ABIDecodedData decoded =
        new ABIDecodedData(
            MAPPER.readTree("{\"available\":\"100\"}"),
            "InsufficientBalance(100)",
            definition,
            "InsufficientBalance(uint256)");

    final String json = MAPPER.writeValueAsString(decoded);
    final ABIDecodedData parsed = MAPPER.readValue(json, ABIDecodedData.class);

    assertEquals("100", parsed.data().get("available").asText());
    assertEquals("InsufficientBalance(100)", parsed.summary());
    assertEquals("InsufficientBalance(uint256)", parsed.signature());
    assertEquals(EntryType.ERROR, parsed.definition().type());
    assertTrue(decoded.toString().contains("InsufficientBalance(uint256)"));
  }

  @Test
  void omitsUnsetFields() throws Exception {
    final ABIDecodedData decoded = new ABIDecodedData(null, null, null, null);
    final String json = MAPPER.writeValueAsString(decoded);
    assertEquals("{}", json);
    assertNull(decoded.data());
    assertNull(decoded.summary());
    assertNull(decoded.definition());
    assertNull(decoded.signature());
  }

  @Test
  void omitsEmptySummary() throws Exception {
    final ABIDecodedData decoded = new ABIDecodedData(null, "", null, null);
    assertEquals("{}", MAPPER.writeValueAsString(decoded));
  }
}
