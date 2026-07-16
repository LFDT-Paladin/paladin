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
import java.util.List;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.types.Bytes32;

class StoredABITest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsWithEntries() throws Exception {
    final Bytes32 hash =
        Bytes32.fromString("0x1111111111111111111111111111111111111111111111111111111111111111");
    final AbiEntry entry =
        AbiEntry.function("transfer").input(AbiParameter.of("to", "string")).build();
    final StoredABI stored = new StoredABI(hash, List.of(entry));

    final String json = MAPPER.writeValueAsString(stored);
    final StoredABI parsed = MAPPER.readValue(json, StoredABI.class);

    assertEquals(hash, parsed.hash());
    assertEquals(1, parsed.abi().size());
    assertEquals("transfer", parsed.abi().get(0).name());
    assertTrue(stored.toString().contains("hash="));
  }

  @Test
  void defaultsToEmptyAbiWhenNull() throws Exception {
    final StoredABI stored = new StoredABI(null, null);
    assertTrue(stored.abi().isEmpty());
    assertNull(stored.hash());
    assertEquals("{}", MAPPER.writeValueAsString(stored));
  }
}
