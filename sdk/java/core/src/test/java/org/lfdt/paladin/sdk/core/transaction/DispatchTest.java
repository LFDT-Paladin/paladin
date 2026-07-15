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
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

class DispatchTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTrips() throws Exception {
    final Dispatch dispatch = new Dispatch("dispatch-1", "txn-1", 42L);
    final String json = MAPPER.writeValueAsString(dispatch);

    final Dispatch parsed = MAPPER.readValue(json, Dispatch.class);
    assertEquals("dispatch-1", parsed.id());
    assertEquals("txn-1", parsed.transactionID());
    assertEquals(42L, parsed.publicTransactionID());
    assertTrue(dispatch.toString().contains("dispatch-1"));
  }

  @Test
  void omitsEmptyStringsButKeepsZeroId() throws Exception {
    final Dispatch dispatch = new Dispatch("", "", 0L);
    final String json = MAPPER.writeValueAsString(dispatch);
    assertFalse(json.contains("\"id\""));
    assertFalse(json.contains("\"transactionID\""));
    assertTrue(json.contains("\"publicTransactionID\":0"));
  }
}
