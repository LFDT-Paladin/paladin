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
package org.lfdt.paladin.sdk.core.statestore;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

class StateStoreTypesTest {

  private static final ObjectMapper MAPPER = PaladinObjectMapper.shared();

  private static final String SCHEMA_ID =
      "0x1111111111111111111111111111111111111111111111111111111111111111";
  private static final String CONTRACT = "0x2222222222222222222222222222222222222222";
  private static final String TX = "3b8c1a2e-0000-0000-0000-000000000001";

  @Test
  void schemaTypeRoundTripsAndRejectsUnknown() throws Exception {
    assertEquals(SchemaType.ABI, MAPPER.readValue("\"abi\"", SchemaType.class));
    assertEquals("abi", SchemaType.ABI.jsonValue());
    assertEquals(SchemaType.ABI, SchemaType.fromJson("ABI"));
    assertThrows(IllegalArgumentException.class, () -> SchemaType.fromJson("bogus"));
    assertThrows(IllegalArgumentException.class, () -> SchemaType.fromJson(null));
  }

  @Test
  void stateLockTypeRoundTripsAndRejectsUnknown() throws Exception {
    for (final StateLockType t : StateLockType.values()) {
      assertEquals(t, MAPPER.readValue(MAPPER.writeValueAsString(t), StateLockType.class));
    }
    assertEquals("spend", StateLockType.SPEND.jsonValue());
    assertEquals(StateLockType.CREATE, StateLockType.fromJson("CREATE"));
    assertThrows(IllegalArgumentException.class, () -> StateLockType.fromJson("bogus"));
    assertThrows(IllegalArgumentException.class, () -> StateLockType.fromJson(null));
  }

  @Test
  void stateStatusQualifierHandlesStandardValuesAndUuids() throws Exception {
    assertEquals(
        StateStatusQualifier.AVAILABLE,
        MAPPER.readValue("\"available\"", StateStatusQualifier.class));
    assertEquals(StateStatusQualifier.SPENT, StateStatusQualifier.fromString("SPENT"));
    assertEquals("all", StateStatusQualifier.ALL.value());
    assertEquals("\"confirmed\"", MAPPER.writeValueAsString(StateStatusQualifier.CONFIRMED));

    final UUID txId = UUID.fromString(TX);
    final StateStatusQualifier forTx = StateStatusQualifier.forTransaction(txId);
    assertEquals(TX, forTx.value());
    assertEquals(forTx, StateStatusQualifier.fromString(TX));
    assertEquals(forTx.hashCode(), StateStatusQualifier.fromString(TX).hashCode());
    assertEquals(forTx.toString(), TX);
    assertNotEquals(StateStatusQualifier.ALL, forTx);
    assertNotEquals(StateStatusQualifier.ALL, "all");

    assertThrows(
        IllegalArgumentException.class, () -> StateStatusQualifier.fromString("not-a-uuid"));
    assertThrows(IllegalArgumentException.class, () -> StateStatusQualifier.fromString(null));
    assertThrows(NullPointerException.class, () -> StateStatusQualifier.forTransaction(null));
  }

  @Test
  void schemaRoundTrips() throws Exception {
    final String json =
        "{\"id\":\""
            + SCHEMA_ID
            + "\",\"created\":\"2024-01-01T00:00:00Z\",\"domain\":\"noto\",\"type\":\"abi\","
            + "\"signature\":\"type=Coin\",\"definition\":{\"type\":\"tuple\"},"
            + "\"labels\":[\"owner\"]}";
    final Schema schema = MAPPER.readValue(json, Schema.class);
    assertEquals(Bytes32.fromString(SCHEMA_ID), schema.id());
    assertEquals("noto", schema.domain());
    assertEquals(SchemaType.ABI, schema.type());
    assertEquals("type=Coin", schema.signature());
    assertEquals("tuple", schema.definition().get("type").asText());
    assertEquals(java.util.List.of("owner"), schema.labels());
    assertTrue(schema.created().toString().startsWith("2024-01-01"));

    final Schema reparsed = MAPPER.readValue(MAPPER.writeValueAsString(schema), Schema.class);
    assertEquals(schema, reparsed);
    assertEquals(schema.hashCode(), reparsed.hashCode());
    assertEquals(schema, schema);
    assertNotEquals(schema, "not a schema");
    assertTrue(schema.toString().contains("domain=noto"));
  }

  @Test
  void stateRoundTripsWithAllRecords() throws Exception {
    final String json =
        "{\"id\":\"0xaa\",\"created\":\"2024-01-01T00:00:00Z\",\"domain\":\"noto\",\"schema\":\""
            + SCHEMA_ID
            + "\",\"contractAddress\":\""
            + CONTRACT
            + "\",\"data\":{\"amount\":\"100\"},"
            + "\"confirmed\":{\"transaction\":\""
            + TX
            + "\"},\"read\":{\"transaction\":\""
            + TX
            + "\"},\"spent\":{\"transaction\":\""
            + TX
            + "\"},\"locks\":[{\"transaction\":\""
            + TX
            + "\",\"type\":\"spend\"}],"
            + "\"nullifier\":{\"id\":\"0xbb\",\"spent\":{\"transaction\":\""
            + TX
            + "\"}}}";
    final State state = MAPPER.readValue(json, State.class);
    assertEquals(HexBytes.fromString("0xaa"), state.id());
    assertEquals("noto", state.domain());
    assertEquals(Bytes32.fromString(SCHEMA_ID), state.schema());
    assertEquals(EthAddress.fromString(CONTRACT), state.contractAddress());
    assertEquals("100", state.data().get("amount").asText());

    final UUID txId = UUID.fromString(TX);
    assertEquals(txId, state.confirmed().transaction());
    assertEquals(txId, state.read().transaction());
    assertEquals(txId, state.spent().transaction());
    assertEquals(1, state.locks().size());
    assertEquals(StateLockType.SPEND, state.locks().get(0).type());
    assertEquals(txId, state.locks().get(0).transaction());
    assertEquals(HexBytes.fromString("0xbb"), state.nullifier().id());
    assertEquals(txId, state.nullifier().spent().transaction());

    final State reparsed = MAPPER.readValue(MAPPER.writeValueAsString(state), State.class);
    assertEquals(state, reparsed);
    assertEquals(state.hashCode(), reparsed.hashCode());
    assertEquals(state, state);
    assertNotEquals(state, "not a state");
    assertTrue(state.toString().contains("domain=noto"));

    // Cover record and lock equality/toString directly.
    assertEquals(state.confirmed(), reparsed.confirmed());
    assertEquals(state.confirmed().hashCode(), reparsed.confirmed().hashCode());
    assertNotEquals(state.confirmed(), "x");
    assertTrue(state.confirmed().toString().contains("transaction="));
    assertEquals(state.read(), reparsed.read());
    assertNotEquals(state.read(), "x");
    assertTrue(state.read().toString().contains("transaction="));
    assertEquals(state.spent(), reparsed.spent());
    assertNotEquals(state.spent(), "x");
    assertTrue(state.spent().toString().contains("transaction="));
    assertEquals(state.locks().get(0), reparsed.locks().get(0));
    assertEquals(state.locks().get(0).hashCode(), reparsed.locks().get(0).hashCode());
    assertNotEquals(state.locks().get(0), "x");
    assertTrue(state.locks().get(0).toString().contains("type=SPEND"));
    assertEquals(state.nullifier(), reparsed.nullifier());
    assertEquals(state.nullifier().hashCode(), reparsed.nullifier().hashCode());
    assertNotEquals(state.nullifier(), "x");
    assertTrue(state.nullifier().toString().contains("id="));
  }

  @Test
  void stateOmitsAbsentOptionalRecords() throws Exception {
    final State state =
        MAPPER.readValue(
            "{\"id\":\"0xaa\",\"created\":\"2024-01-01T00:00:00Z\",\"domain\":\"noto\","
                + "\"schema\":\""
                + SCHEMA_ID
                + "\",\"data\":{}}",
            State.class);
    assertNull(state.contractAddress());
    assertNull(state.confirmed());
    assertNull(state.read());
    assertNull(state.spent());
    assertNull(state.locks());
    assertNull(state.nullifier());
    final String serialized = MAPPER.writeValueAsString(state);
    assertTrue(serialized.indexOf("confirmed") < 0);
    assertTrue(serialized.indexOf("contractAddress") < 0);
  }
}
