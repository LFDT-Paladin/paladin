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
package org.lfdt.paladin.sdk.core.transaction;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
import org.junit.jupiter.api.Test;

class TransactionStatesTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsPopulatedStates() throws Exception {
    TransactionStates states =
        new TransactionStates(
            false,
            List.of(MAPPER.readTree("{\"id\":\"s1\"}")),
            List.of(MAPPER.readTree("{\"id\":\"r1\"}")),
            List.of(MAPPER.readTree("{\"id\":\"c1\"}")),
            List.of(MAPPER.readTree("{\"id\":\"i1\"}")),
            MAPPER.readTree("{\"count\":1}"));

    String json = MAPPER.writeValueAsString(states);
    TransactionStates parsed = MAPPER.readValue(json, TransactionStates.class);

    assertFalse(parsed.none());
    assertEquals(1, parsed.spent().size());
    assertEquals(1, parsed.read().size());
    assertEquals(1, parsed.confirmed().size());
    assertEquals(1, parsed.info().size());
    assertEquals(1, parsed.unavailable().get("count").asInt());
    assertTrue(states.toString().contains("none=false"));
  }

  @Test
  void noneStateOmitsEmptyLists() throws Exception {
    TransactionStates states = new TransactionStates(true, null, null, null, null, null);
    String json = MAPPER.writeValueAsString(states);
    assertEquals("{\"none\":true}", json);

    TransactionStates parsed = MAPPER.readValue(json, TransactionStates.class);
    assertTrue(parsed.none());
    assertTrue(parsed.spent().isEmpty());
    assertTrue(parsed.read().isEmpty());
    assertTrue(parsed.confirmed().isEmpty());
    assertTrue(parsed.info().isEmpty());
    assertNull(parsed.unavailable());
  }

  @Test
  void falseNoneIsOmittedByDefault() throws Exception {
    TransactionStates states = new TransactionStates(false, null, null, null, null, null);
    assertEquals("{}", MAPPER.writeValueAsString(states));
  }
}
