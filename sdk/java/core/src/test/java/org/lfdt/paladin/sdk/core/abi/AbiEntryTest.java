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
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
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

  @Test
  void equalityCoversEveryField() {
    final AbiEntry entry =
        AbiEntry.function("transfer")
            .stateMutability(StateMutability.NONPAYABLE)
            .payable(true)
            .constant(true)
            .anonymous(true)
            .input(AbiParameter.of("amount", "uint256"))
            .output(AbiParameter.of("", "bool"))
            .build();

    assertEquals(entry, entry);
    assertEquals(entry, copyOf(entry).build());
    assertEquals(entry.hashCode(), copyOf(entry).build().hashCode());

    assertNotEquals(entry, copyOf(entry).payable(false).build());
    assertNotEquals(entry, copyOf(entry).constant(false).build());
    assertNotEquals(entry, copyOf(entry).anonymous(false).build());
    assertNotEquals(entry, copyOf(entry).stateMutability(StateMutability.VIEW).build());
    assertNotEquals(entry, AbiEntry.event("transfer").build());
    assertNotEquals(entry, AbiEntry.function("approve").build());
    assertNotEquals(entry, copyOf(entry).input(AbiParameter.of("extra", "bool")).build());
    assertNotEquals(entry, copyOf(entry).output(AbiParameter.of("extra", "bool")).build());
    assertNotEquals(entry, null);
    assertNotEquals(entry, "transfer");
  }

  /** A builder pre-populated to match {@code entry}, so a single field can be varied from it. */
  private static AbiEntry.Builder copyOf(final AbiEntry entry) {
    return AbiEntry.builder(entry.type())
        .name(entry.name())
        .stateMutability(entry.stateMutability())
        .payable(entry.payable())
        .constant(entry.constant())
        .anonymous(entry.anonymous())
        .inputs(entry.inputs())
        .outputs(entry.outputs());
  }

  @Test
  void builderAddsParameterListsInBulk() {
    final AbiEntry entry =
        AbiEntry.function("mint")
            .inputs(List.of(AbiParameter.of("to", "address"), AbiParameter.of("amount", "uint256")))
            .outputs(List.of(AbiParameter.of("", "bool")))
            .build();
    assertEquals(2, entry.inputs().size());
    assertEquals("amount", entry.inputs().get(1).name());
    assertEquals(1, entry.outputs().size());
  }

  @Test
  void serializesLegacyPayableAndConstantFlagsWhenSet() throws Exception {
    final AbiEntry entry = AbiEntry.function("legacy").payable(true).constant(true).build();
    final String json = MAPPER.writeValueAsString(entry);
    assertTrue(json.contains("\"payable\":true"));
    assertTrue(json.contains("\"constant\":true"));
    assertEquals(entry, MAPPER.readValue(json, AbiEntry.class));
  }

  @Test
  void toStringSummarizesTypeNameAndArity() {
    final AbiEntry entry =
        AbiEntry.function("transfer").input(AbiParameter.of("amount", "uint256")).build();
    assertEquals("AbiEntry{type=FUNCTION, name=transfer, inputs=1, outputs=0}", entry.toString());
  }

  @Test
  void constructorEntryHasNoName() throws Exception {
    final AbiEntry entry =
        AbiEntry.constructor().input(AbiParameter.of("owner", "address")).build();
    assertEquals(EntryType.CONSTRUCTOR, entry.type());
    assertEquals("", entry.name());
    assertEquals(entry, MAPPER.readValue(MAPPER.writeValueAsString(entry), AbiEntry.class));
  }
}
