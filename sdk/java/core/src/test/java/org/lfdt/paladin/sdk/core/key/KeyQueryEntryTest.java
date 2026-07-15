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
package org.lfdt.paladin.sdk.core.key;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
import org.junit.jupiter.api.Test;

class KeyQueryEntryTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsKeyNode() throws Exception {
    final KeyVerifier verifier = new KeyVerifier("0xabc", "eth_address", "ecdsa:secp256k1");
    final KeyQueryEntry entry =
        new KeyQueryEntry(
            true,
            false,
            "alice",
            "alice.key1",
            "key1",
            1,
            "wallet1",
            "handle-1",
            List.of(verifier));

    final String json = MAPPER.writeValueAsString(entry);
    final KeyQueryEntry parsed = MAPPER.readValue(json, KeyQueryEntry.class);

    assertEquals(entry, parsed);
    assertTrue(parsed.isKey());
    assertFalse(parsed.hasChildren());
    assertEquals("alice", parsed.parent());
    assertEquals("alice.key1", parsed.path());
    assertEquals("key1", parsed.name());
    assertEquals(1, parsed.index());
    assertEquals("wallet1", parsed.wallet());
    assertEquals("handle-1", parsed.keyHandle());
    assertEquals(1, parsed.verifiers().size());
    assertTrue(entry.toString().contains("alice.key1"));
  }

  @Test
  void roundTripsIntermediatePathNode() throws Exception {
    final KeyQueryEntry entry =
        new KeyQueryEntry(false, true, "", "alice", "alice", 0, null, null, null);

    final String json = MAPPER.writeValueAsString(entry);
    final KeyQueryEntry parsed = MAPPER.readValue(json, KeyQueryEntry.class);

    assertFalse(parsed.isKey());
    assertTrue(parsed.hasChildren());
    assertTrue(parsed.verifiers().isEmpty());
  }

  @Test
  void equalsAndHashCode() {
    final KeyQueryEntry a = new KeyQueryEntry(true, false, "p", "p.k", "k", 0, "w", "h", List.of());
    final KeyQueryEntry b = new KeyQueryEntry(true, false, "p", "p.k", "k", 0, "w", "h", List.of());
    final KeyQueryEntry different =
        new KeyQueryEntry(false, false, "p", "p.k", "k", 0, "w", "h", List.of());

    assertEquals(a, a);
    assertEquals(a, b);
    assertEquals(a.hashCode(), b.hashCode());
    assertNotEquals(a, different);
    assertNotEquals(a, "not an entry");
  }
}
