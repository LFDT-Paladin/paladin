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

class TransactionFullTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsWithDependenciesAndReceipt() throws Exception {
    UUID id = UUID.randomUUID();
    UUID dependency = UUID.randomUUID();
    TransactionReceipt receipt = TransactionReceipt.builder().id(id).sequence(1).success(true).build();

    TransactionFull full =
        new TransactionFull(
            id,
            null,
            SubmitMode.AUTO,
            null,
            TransactionType.PUBLIC,
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
            List.of(dependency),
            receipt,
            List.of(MAPPER.readTree("{\"p\":1}")),
            List.of(MAPPER.readTree("{\"h\":1}")),
            List.of(MAPPER.readTree("{\"s\":1}")));

    String json = MAPPER.writeValueAsString(full);
    TransactionFull parsed = MAPPER.readValue(json, TransactionFull.class);

    assertEquals(id, parsed.id());
    assertEquals(1, parsed.dependsOn().size());
    assertEquals(dependency, parsed.dependsOn().get(0));
    assertEquals(receipt, parsed.receipt());
    assertEquals(1, parsed.publicTransactions().size());
    assertEquals(1, parsed.history().size());
    assertEquals(1, parsed.sequencerActivity().size());
    assertTrue(full.toString().contains("dependsOn=1"));
  }

  @Test
  void defaultsToEmptyListsWhenNull() {
    TransactionFull full =
        new TransactionFull(
            null, null, null, null, null, null, null, null, null, null, null, null, null, null,
            null, null, null, null, null, null);
    assertTrue(full.dependsOn().isEmpty());
    assertTrue(full.publicTransactions().isEmpty());
    assertTrue(full.history().isEmpty());
    assertTrue(full.sequencerActivity().isEmpty());
  }
}
