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
import java.util.List;
import java.util.UUID;
import org.junit.jupiter.api.Test;

class TransactionReceiptFullTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsWithStatesAndDomainReceipt() throws Exception {
    final UUID id = UUID.randomUUID();
    final TransactionStates states = new TransactionStates(true, null, null, null, null, null);

    final TransactionReceiptFull full =
        new TransactionReceiptFull(
            id,
            null,
            5,
            "noto",
            true,
            null,
            null,
            null,
            null,
            null,
            null,
            null,
            null,
            states,
            MAPPER.readTree("{\"n\":1}"),
            null,
            List.of(MAPPER.readTree("{\"p\":1}")));

    final String json = MAPPER.writeValueAsString(full);
    final TransactionReceiptFull parsed = MAPPER.readValue(json, TransactionReceiptFull.class);

    assertEquals(id, parsed.id());
    assertEquals(5, parsed.sequence());
    assertEquals("noto", parsed.domain());
    assertTrue(parsed.success());
    assertTrue(parsed.states().none());
    assertEquals(1, parsed.domainReceipt().get("n").asInt());
    assertEquals(1, parsed.publicTransactions().size());
    assertTrue(full.toString().contains("success=true"));
  }

  @Test
  void carriesDomainReceiptError() throws Exception {
    final TransactionReceiptFull full =
        new TransactionReceiptFull(
            UUID.randomUUID(),
            null,
            1,
            null,
            false,
            null,
            null,
            null,
            null,
            null,
            null,
            null,
            null,
            null,
            null,
            "decode failed",
            null);

    final String json = MAPPER.writeValueAsString(full);
    final TransactionReceiptFull parsed = MAPPER.readValue(json, TransactionReceiptFull.class);

    assertEquals("decode failed", parsed.domainReceiptError());
    assertTrue(parsed.publicTransactions().isEmpty());
  }
}
