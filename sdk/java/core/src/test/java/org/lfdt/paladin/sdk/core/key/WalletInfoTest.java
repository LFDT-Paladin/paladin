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
import org.junit.jupiter.api.Test;

class WalletInfoTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTrips() throws Exception {
    final WalletInfo wallet = new WalletInfo("signer-1", ".*", false);
    final String json = MAPPER.writeValueAsString(wallet);

    final WalletInfo parsed = MAPPER.readValue(json, WalletInfo.class);
    assertEquals(wallet, parsed);
    assertEquals("signer-1", parsed.name());
    assertEquals(".*", parsed.keySelector());
    assertFalse(parsed.keySelectorMustNotMatch());
  }

  /** The exact shape a node returns from {@code keymgr_wallets} for a default devnet wallet. */
  @Test
  void parsesNodeResponseShape() throws Exception {
    final String json =
        "{\"name\":\"signer-1\",\"keySelector\":\".*\",\"keySelectorMustNotMatch\":false}";

    final WalletInfo parsed = MAPPER.readValue(json, WalletInfo.class);
    assertEquals("signer-1", parsed.name());
    assertEquals(".*", parsed.keySelector());
    assertFalse(parsed.keySelectorMustNotMatch());
  }

  @Test
  void parsesInvertedSelector() throws Exception {
    final String json =
        "{\"name\":\"fallback\",\"keySelector\":\"^hsm\\\\.\",\"keySelectorMustNotMatch\":true}";

    final WalletInfo parsed = MAPPER.readValue(json, WalletInfo.class);
    assertEquals("fallback", parsed.name());
    assertTrue(parsed.keySelectorMustNotMatch());
  }

  @Test
  void equalsAndHashCode() {
    final WalletInfo a = new WalletInfo("w", ".*", false);
    final WalletInfo b = new WalletInfo("w", ".*", false);
    final WalletInfo differentName = new WalletInfo("other", ".*", false);
    final WalletInfo differentSense = new WalletInfo("w", ".*", true);

    assertEquals(a, a);
    assertEquals(a, b);
    assertEquals(a.hashCode(), b.hashCode());
    assertNotEquals(a, differentName);
    assertNotEquals(a, differentSense);
    assertNotEquals(a, "not a wallet");
    assertTrue(a.toString().contains("name=w"));
  }
}
