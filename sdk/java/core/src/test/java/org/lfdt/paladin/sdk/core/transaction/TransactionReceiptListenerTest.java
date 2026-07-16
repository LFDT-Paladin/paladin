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
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.types.Timestamp;

class TransactionReceiptListenerTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void builderCreatesInputWithoutCreatedTimestamp() throws Exception {
    final TransactionReceiptFilters filters =
        TransactionReceiptFilters.builder().domain("noto").build();
    final TransactionReceiptListenerOptions options =
        TransactionReceiptListenerOptions.builder().domainReceipts(true).build();

    final TransactionReceiptListener listener =
        TransactionReceiptListener.builder()
            .name("listener-1")
            .started(true)
            .filters(filters)
            .options(options)
            .build();

    assertNull(listener.created());
    final String json = MAPPER.writeValueAsString(listener);
    assertFalse(json.contains("created"));

    final TransactionReceiptListener parsed =
        MAPPER.readValue(json, TransactionReceiptListener.class);
    assertEquals("listener-1", parsed.name());
    assertTrue(parsed.started());
    assertEquals("noto", parsed.filters().domain());
    assertTrue(parsed.options().domainReceipts());
  }

  @Test
  void parsesServerResponseWithCreatedTimestamp() throws Exception {
    final String json = "{\"name\":\"l1\",\"created\":\"2024-06-18T12:00:00Z\",\"started\":false}";
    final TransactionReceiptListener listener =
        MAPPER.readValue(json, TransactionReceiptListener.class);

    assertEquals("l1", listener.name());
    assertEquals(Timestamp.fromString("2024-06-18T12:00:00Z"), listener.created());
    assertFalse(listener.started());
    assertTrue(listener.toString().contains("l1"));
  }

  @Test
  void zeroTimestampIsNormalizedToNull() {
    final TransactionReceiptListener listener =
        new TransactionReceiptListener("l", Timestamp.ZERO, null, null, null);
    assertNull(listener.created());
  }
}
