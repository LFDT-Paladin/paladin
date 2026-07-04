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
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.types.EthAddress;

class PreparedTransactionTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsFullPreparedTransaction() throws Exception {
    UUID id = UUID.randomUUID();
    EthAddress to = EthAddress.fromString("0x05d936207F04D81a85881b72A0D17854Ee8BE45A");
    TransactionInput transaction =
        TransactionInput.builder().type(TransactionType.PRIVATE).domain("noto").to(to).build();
    TransactionStates states = new TransactionStates(true, null, null, null, null, null);

    PreparedTransaction prepared =
        new PreparedTransaction(
            id, "noto", to, transaction, MAPPER.readTree("{\"note\":\"x\"}"), states);

    String json = MAPPER.writeValueAsString(prepared);
    PreparedTransaction parsed = MAPPER.readValue(json, PreparedTransaction.class);

    assertEquals(id, parsed.id());
    assertEquals("noto", parsed.domain());
    assertEquals(to, parsed.to());
    assertEquals(TransactionType.PRIVATE, parsed.transaction().type());
    assertEquals("x", parsed.metadata().get("note").asText());
    assertTrue(parsed.states().none());
    assertTrue(prepared.toString().contains("noto"));
  }

  @Test
  void omitsUnsetFields() throws Exception {
    PreparedTransaction prepared = new PreparedTransaction(null, null, null, null, null, null);
    assertEquals("{}", MAPPER.writeValueAsString(prepared));
    assertNull(prepared.id());
    assertNull(prepared.transaction());
  }
}
