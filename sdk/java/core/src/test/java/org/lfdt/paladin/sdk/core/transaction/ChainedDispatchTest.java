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
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

class ChainedDispatchTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTrips() throws Exception {
    ChainedDispatch dispatch = new ChainedDispatch("cd-1", "txn-1", "txn-2");
    String json = MAPPER.writeValueAsString(dispatch);

    ChainedDispatch parsed = MAPPER.readValue(json, ChainedDispatch.class);
    assertEquals("cd-1", parsed.id());
    assertEquals("txn-1", parsed.transactionID());
    assertEquals("txn-2", parsed.chainedTransactionID());
    assertTrue(dispatch.toString().contains("cd-1"));
  }

  @Test
  void omitsEmptyStrings() throws Exception {
    ChainedDispatch dispatch = new ChainedDispatch(null, null, null);
    assertEquals("{}", MAPPER.writeValueAsString(dispatch));
  }
}
