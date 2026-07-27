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
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.types.EthAddress;

class TransactionCallTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void serializesUnwrappedInputWithBlockAndDataFormat() throws Exception {
    final EthAddress to = EthAddress.fromString("0x05d936207F04D81a85881b72A0D17854Ee8BE45A");
    final TransactionInput input =
        TransactionInput.builder().type(TransactionType.PUBLIC).from("key1").to(to).build();
    final TransactionCall call =
        TransactionCall.builder(input).block("latest").dataFormat("mapping").build();

    final String json = MAPPER.writeValueAsString(call);
    assertTrue(json.contains("\"type\":\"public\""));
    assertTrue(json.contains("\"from\":\"key1\""));
    assertTrue(json.contains("\"block\":\"latest\""));
    assertTrue(json.contains("\"dataFormat\":\"mapping\""));
    assertEquals(input, call.input());
    assertEquals("latest", call.block());
    assertEquals("mapping", call.dataFormat());
    assertTrue(call.toString().contains("latest"));
  }

  @Test
  void omitsUnsetBlockAndDataFormat() throws Exception {
    final TransactionInput input = TransactionInput.builder().build();
    final TransactionCall call = TransactionCall.builder(input).build();

    final String json = MAPPER.writeValueAsString(call);
    assertEquals("{}", json);
    assertNull(call.block());
    assertNull(call.dataFormat());
  }

  @Test
  void rejectsNullInput() {
    assertThrows(NullPointerException.class, () -> TransactionCall.builder(null).build());
  }
}
