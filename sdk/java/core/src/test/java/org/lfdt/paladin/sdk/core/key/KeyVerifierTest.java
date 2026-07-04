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
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

class KeyVerifierTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTrips() throws Exception {
    KeyVerifier verifier =
        new KeyVerifier("0x05d936207F04D81a85881b72A0D17854Ee8BE45A", "eth_address", "ecdsa:secp256k1");
    String json = MAPPER.writeValueAsString(verifier);

    KeyVerifier parsed = MAPPER.readValue(json, KeyVerifier.class);
    assertEquals(verifier, parsed);
    assertEquals("0x05d936207F04D81a85881b72A0D17854Ee8BE45A", parsed.verifier());
    assertEquals("eth_address", parsed.type());
    assertEquals("ecdsa:secp256k1", parsed.algorithm());
  }

  @Test
  void equalsAndHashCode() {
    KeyVerifier a = new KeyVerifier("v", "t", "alg");
    KeyVerifier b = new KeyVerifier("v", "t", "alg");
    KeyVerifier different = new KeyVerifier("other", "t", "alg");

    assertEquals(a, a);
    assertEquals(a, b);
    assertEquals(a.hashCode(), b.hashCode());
    assertNotEquals(a, different);
    assertNotEquals(a, "not a verifier");
    assertTrue(a.toString().contains("verifier=v"));
  }
}
