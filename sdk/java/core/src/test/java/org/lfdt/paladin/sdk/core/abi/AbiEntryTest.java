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
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

class AbiEntryTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void serializesInputsAndOutputsAlwaysAndOmitsFalseFlags() throws Exception {
    // Legacy/false flags (payable, constant, anonymous) and a null type/name are dropped, but the
    // inputs and outputs arrays are always present — even when empty — mirroring the Go struct.
    final AbiEntry empty = AbiEntry.builder(EntryType.RECEIVE).build();
    assertEquals(
        "{\"type\":\"receive\",\"inputs\":[],\"outputs\":[]}", MAPPER.writeValueAsString(empty));
  }

  @Test
  void roundTripsTransferFunction() throws Exception {
    final AbiEntry transfer =
        AbiEntry.function("transfer")
            .stateMutability(StateMutability.NONPAYABLE)
            .input(AbiParameter.builder("recipient", "address").internalType("address").build())
            .input(AbiParameter.builder("amount", "uint256").internalType("uint256").build())
            .output(AbiParameter.builder("", "bool").internalType("bool").build())
            .build();

    final AbiEntry parsed = MAPPER.readValue(MAPPER.writeValueAsString(transfer), AbiEntry.class);
    assertEquals(transfer, parsed);
    assertEquals(EntryType.FUNCTION, parsed.type());
    assertEquals("transfer", parsed.name());
    assertEquals(StateMutability.NONPAYABLE, parsed.stateMutability());
    assertEquals(2, parsed.inputs().size());
    assertEquals(1, parsed.outputs().size());
    assertEquals("recipient", parsed.inputs().get(0).name());
  }

  @Test
  void parsesSolidityCompilerOutput() throws Exception {
    final String json =
        "{"
            + "\"inputs\":[{\"internalType\":\"address\",\"name\":\"recipient\",\"type\":\"address\"},"
            + "{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],"
            + "\"name\":\"transfer\","
            + "\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],"
            + "\"stateMutability\":\"nonpayable\","
            + "\"type\":\"function\"}";
    final AbiEntry e = MAPPER.readValue(json, AbiEntry.class);
    assertEquals(EntryType.FUNCTION, e.type());
    assertEquals("transfer", e.name());
    assertEquals(StateMutability.NONPAYABLE, e.stateMutability());
    assertEquals("amount", e.inputs().get(1).name());
  }

  @Test
  void eventCarriesIndexedAndAnonymousFlags() throws Exception {
    final AbiEntry event =
        AbiEntry.event("Transfer")
            .anonymous(true)
            .input(AbiParameter.builder("from", "address").indexed(true).build())
            .input(AbiParameter.of("value", "uint256"))
            .build();
    final String json = MAPPER.writeValueAsString(event);
    assertTrue(json.contains("\"type\":\"event\""));
    assertTrue(json.contains("\"anonymous\":true"));
    assertTrue(json.contains("\"indexed\":true"));
    assertEquals(event, MAPPER.readValue(json, AbiEntry.class));
  }

  @Test
  void parsesEntryTypeAndMutabilityCaseInsensitively() throws Exception {
    final AbiEntry e =
        MAPPER.readValue("{\"type\":\"FUNCTION\",\"stateMutability\":\"View\"}", AbiEntry.class);
    assertEquals(EntryType.FUNCTION, e.type());
    assertEquals(StateMutability.VIEW, e.stateMutability());
  }

  @Test
  void rejectsUnknownEntryType() {
    assertThrows(
        Exception.class, () -> MAPPER.readValue("{\"type\":\"modifier\"}", AbiEntry.class));
  }

  @Test
  void emptyEntryHasNonNullInputAndOutputLists() throws Exception {
    final AbiEntry e = MAPPER.readValue("{\"type\":\"constructor\"}", AbiEntry.class);
    assertEquals(EntryType.CONSTRUCTOR, e.type());
    assertTrue(e.inputs().isEmpty());
    assertTrue(e.outputs().isEmpty());
    assertFalse(e.payable());
  }
}
