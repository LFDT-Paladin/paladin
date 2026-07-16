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

class TransactionReceiptListenerOptionsTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void builderRoundTripsAllFields() throws Exception {
    final TransactionReceiptListenerOptions options =
        TransactionReceiptListenerOptions.builder()
            .domainReceipts(true)
            .incompleteStateReceiptBehavior("process")
            .build();

    final String json = MAPPER.writeValueAsString(options);
    final TransactionReceiptListenerOptions parsed =
        MAPPER.readValue(json, TransactionReceiptListenerOptions.class);

    assertTrue(parsed.domainReceipts());
    assertEquals("process", parsed.incompleteStateReceiptBehavior());
    assertTrue(options.toString().contains("process"));
  }

  @Test
  void defaultsOmitFalseAndEmptyBehavior() throws Exception {
    final TransactionReceiptListenerOptions options =
        TransactionReceiptListenerOptions.builder().build();
    final String json = MAPPER.writeValueAsString(options);
    assertFalse(json.contains("incompleteStateReceiptBehavior"));

    final TransactionReceiptListenerOptions parsed =
        MAPPER.readValue(json, TransactionReceiptListenerOptions.class);
    assertFalse(parsed.domainReceipts());
  }
}
