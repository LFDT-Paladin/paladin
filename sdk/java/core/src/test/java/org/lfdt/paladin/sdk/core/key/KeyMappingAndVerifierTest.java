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
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
import org.junit.jupiter.api.Test;

class KeyMappingAndVerifierTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsWithPathAndVerifier() throws Exception {
    final KeyVerifier verifier = new KeyVerifier("0xabc", "eth_address", "ecdsa:secp256k1");
    final KeyMappingAndVerifier mapping =
        new KeyMappingAndVerifier(
            "alice.key1",
            "wallet1",
            "handle-1",
            List.of(new KeyPathSegment("alice", 0), new KeyPathSegment("key1", 1)),
            verifier);

    final String json = MAPPER.writeValueAsString(mapping);
    final KeyMappingAndVerifier parsed = MAPPER.readValue(json, KeyMappingAndVerifier.class);

    assertEquals(mapping, parsed);
    assertEquals("alice.key1", parsed.identifier());
    assertEquals("wallet1", parsed.wallet());
    assertEquals("handle-1", parsed.keyHandle());
    assertEquals(2, parsed.path().size());
    assertEquals(verifier, parsed.verifier());
    assertTrue(mapping.toString().contains("alice.key1"));
  }

  @Test
  void defaultsPathToEmptyWhenNull() {
    final KeyMappingAndVerifier mapping = new KeyMappingAndVerifier(null, null, null, null, null);
    assertTrue(mapping.path().isEmpty());
    assertNull(mapping.identifier());
    assertNull(mapping.verifier());
  }

  @Test
  void equalsAndHashCode() {
    final KeyMappingAndVerifier a =
        new KeyMappingAndVerifier("id", "wallet", "handle", List.of(), null);
    final KeyMappingAndVerifier b =
        new KeyMappingAndVerifier("id", "wallet", "handle", List.of(), null);
    final KeyMappingAndVerifier different =
        new KeyMappingAndVerifier("other", "wallet", "handle", List.of(), null);

    assertEquals(a, a);
    assertEquals(a, b);
    assertEquals(a.hashCode(), b.hashCode());
    assertNotEquals(a, different);
    assertNotEquals(a, "not a mapping");
  }
}
