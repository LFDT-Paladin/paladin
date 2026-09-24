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
package org.lfdt.paladin.sdk.core.registry;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.types.HexBytes;

class RegistryTypesTest {

  private static final ObjectMapper MAPPER = PaladinObjectMapper.shared();

  private static final String ENTRY_JSON =
      "{\"registry\":\"reg1\",\"id\":\"0x01\",\"name\":\"acme\",\"parentId\":\"0x00\","
          + "\"blockNumber\":10,\"transactionIndex\":2,\"logIndex\":1,\"active\":true}";

  @Test
  void activeFilterRoundTripsAndRejectsUnknown() throws Exception {
    for (final ActiveFilter f : ActiveFilter.values()) {
      final String json = MAPPER.writeValueAsString(f);
      assertEquals(f, MAPPER.readValue(json, ActiveFilter.class));
    }
    assertEquals("active", ActiveFilter.ACTIVE.jsonValue());
    assertEquals(ActiveFilter.ANY, ActiveFilter.fromJson("ANY"));
    assertThrows(IllegalArgumentException.class, () -> ActiveFilter.fromJson("bogus"));
    assertThrows(IllegalArgumentException.class, () -> ActiveFilter.fromJson(null));
  }

  @Test
  void registryEntryRoundTrips() throws Exception {
    final RegistryEntry entry = MAPPER.readValue(ENTRY_JSON, RegistryEntry.class);
    assertEquals("reg1", entry.registry());
    assertEquals(HexBytes.fromString("0x01"), entry.id());
    assertEquals("acme", entry.name());
    assertEquals(HexBytes.fromString("0x00"), entry.parentId());
    assertEquals(10L, entry.blockNumber());
    assertEquals(2L, entry.transactionIndex());
    assertEquals(1L, entry.logIndex());
    assertTrue(entry.active());

    final RegistryEntry reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(entry), RegistryEntry.class);
    assertEquals(entry, reparsed);
    assertEquals(entry.hashCode(), reparsed.hashCode());
    assertEquals(entry, entry);
    assertNotEquals(entry, "not an entry");
    assertTrue(entry.toString().contains("name=acme"));
  }

  @Test
  void registryEntryOmitsAbsentOnChainFields() throws Exception {
    final RegistryEntry entry =
        MAPPER.readValue(
            "{\"registry\":\"reg1\",\"id\":\"0x01\",\"name\":\"acme\"}", RegistryEntry.class);
    assertNull(entry.parentId());
    assertNull(entry.blockNumber());
    assertNull(entry.active());
    assertTrue(MAPPER.writeValueAsString(entry).indexOf("blockNumber") < 0);
  }

  @Test
  void registryPropertyRoundTrips() throws Exception {
    final String json =
        "{\"registry\":\"reg1\",\"entryId\":\"0x01\",\"name\":\"url\","
            + "\"value\":\"https://acme.example\",\"blockNumber\":10,"
            + "\"transactionIndex\":2,\"logIndex\":1,\"active\":true}";
    final RegistryProperty prop = MAPPER.readValue(json, RegistryProperty.class);
    assertEquals("reg1", prop.registry());
    assertEquals(HexBytes.fromString("0x01"), prop.entryId());
    assertEquals("url", prop.name());
    assertEquals("https://acme.example", prop.value());
    assertEquals(10L, prop.blockNumber());
    assertEquals(2L, prop.transactionIndex());
    assertEquals(1L, prop.logIndex());
    assertTrue(prop.active());

    final RegistryProperty reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(prop), RegistryProperty.class);
    assertEquals(prop, reparsed);
    assertEquals(prop.hashCode(), reparsed.hashCode());
    assertEquals(prop, prop);
    assertNotEquals(prop, "not a property");
    assertTrue(prop.toString().contains("name=url"));
  }

  @Test
  void registryEntryWithPropertiesRoundTrips() throws Exception {
    final String json =
        "{\"registry\":\"reg1\",\"id\":\"0x01\",\"name\":\"acme\",\"blockNumber\":10,"
            + "\"transactionIndex\":2,\"logIndex\":1,\"active\":true,"
            + "\"properties\":{\"url\":\"https://acme.example\",\"tier\":\"gold\"}}";
    final RegistryEntryWithProperties entry =
        MAPPER.readValue(json, RegistryEntryWithProperties.class);
    assertEquals("reg1", entry.registry());
    assertEquals(HexBytes.fromString("0x01"), entry.id());
    assertEquals("acme", entry.name());
    assertNull(entry.parentId());
    assertEquals(10L, entry.blockNumber());
    assertEquals(2L, entry.transactionIndex());
    assertEquals(1L, entry.logIndex());
    assertTrue(entry.active());
    assertEquals("gold", entry.properties().get("tier"));

    final RegistryEntryWithProperties reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(entry), RegistryEntryWithProperties.class);
    assertEquals(entry, reparsed);
    assertEquals(entry.hashCode(), reparsed.hashCode());
    assertEquals(entry, entry);
    assertNotEquals(entry, "not an entry");
    assertTrue(entry.toString().contains("name=acme"));
  }
}
