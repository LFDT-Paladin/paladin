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
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;
import org.lfdt.paladin.sdk.core.types.Timestamp;

class TransactionTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsFullTransaction() throws Exception {
    UUID id = UUID.randomUUID();
    EthAddress to = EthAddress.fromString("0x05d936207F04D81a85881b72A0D17854Ee8BE45A");
    Bytes32 abiRef =
        Bytes32.fromString("0x1111111111111111111111111111111111111111111111111111111111111111");

    Transaction transaction =
        new Transaction(
            id,
            Timestamp.fromString("2024-06-18T12:00:00Z"),
            SubmitMode.AUTO,
            "idem-1",
            TransactionType.PRIVATE,
            "noto",
            "transfer",
            abiRef,
            "alice@node1",
            to,
            MAPPER.readTree("{\"amount\":\"100\"}"),
            HexUint64.of(21000),
            HexUint256.of(0),
            HexUint256.of(1),
            HexUint256.of(2));

    String json = MAPPER.writeValueAsString(transaction);
    Transaction parsed = MAPPER.readValue(json, Transaction.class);

    assertEquals(id, parsed.id());
    assertEquals(Timestamp.fromString("2024-06-18T12:00:00Z"), parsed.created());
    assertEquals(SubmitMode.AUTO, parsed.submitMode());
    assertEquals("idem-1", parsed.idempotencyKey());
    assertEquals(TransactionType.PRIVATE, parsed.type());
    assertEquals("noto", parsed.domain());
    assertEquals("transfer", parsed.function());
    assertEquals(abiRef, parsed.abiReference());
    assertEquals("alice@node1", parsed.from());
    assertEquals(to, parsed.to());
    assertEquals("100", parsed.data().get("amount").asText());
    assertEquals(HexUint64.of(21000), parsed.gas());
    assertEquals(HexUint256.of(0), parsed.value());
    assertEquals(HexUint256.of(1), parsed.maxPriorityFeePerGas());
    assertEquals(HexUint256.of(2), parsed.maxFeePerGas());
    assertTrue(transaction.toString().contains("noto"));
  }

  @Test
  void zeroTimestampAndEmptyFieldsAreNormalized() throws Exception {
    Transaction transaction =
        new Transaction(
            null, Timestamp.ZERO, null, null, null, null, null, null, null, null, null, null, null,
            null, null);
    assertNull(transaction.created());
    assertEquals("{}", MAPPER.writeValueAsString(transaction));
  }
}
