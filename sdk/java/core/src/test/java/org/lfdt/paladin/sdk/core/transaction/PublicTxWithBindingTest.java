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
import java.util.List;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;
import org.lfdt.paladin.sdk.core.types.Timestamp;

class PublicTxWithBindingTest {

  private static final ObjectMapper MAPPER = new ObjectMapper();

  @Test
  void roundTripsCompletedTransaction() throws Exception {
    final EthAddress to = EthAddress.fromString("0x05d936207F04D81a85881b72A0D17854Ee8BE45A");
    final EthAddress from = EthAddress.fromString("0x1111111111111111111111111111111111111111");
    final UUID transactionId = UUID.randomUUID();
    final Bytes32 hash =
        Bytes32.fromString("0x1111111111111111111111111111111111111111111111111111111111111111");

    final PublicTxWithBinding publicTx =
        new PublicTxWithBinding(
            7L,
            to,
            HexBytes.fromString("0x1234"),
            from,
            HexUint64.of(3),
            Timestamp.fromString("2024-06-18T12:00:00Z"),
            "dispatcher1",
            Timestamp.fromString("2024-06-18T12:01:00Z"),
            hash,
            true,
            null,
            List.of(MAPPER.readTree("{\"n\":1}")),
            List.of(MAPPER.readTree("{\"a\":1}")),
            HexUint64.of(21000),
            HexUint256.of(0),
            HexUint256.of(1),
            HexUint256.of(2),
            transactionId,
            TransactionType.PUBLIC,
            "alice",
            "0x2222222222222222222222222222222222222222");

    final String json = MAPPER.writeValueAsString(publicTx);
    final PublicTxWithBinding parsed = MAPPER.readValue(json, PublicTxWithBinding.class);

    assertEquals(7L, parsed.localId());
    assertEquals(to, parsed.to());
    assertEquals(HexBytes.fromString("0x1234"), parsed.data());
    assertEquals(from, parsed.from());
    assertEquals(HexUint64.of(3), parsed.nonce());
    assertEquals(Timestamp.fromString("2024-06-18T12:00:00Z"), parsed.created());
    assertEquals("dispatcher1", parsed.dispatcher());
    assertEquals(Timestamp.fromString("2024-06-18T12:01:00Z"), parsed.completedAt());
    assertEquals(hash, parsed.transactionHash());
    assertTrue(parsed.success());
    assertNull(parsed.revertData());
    assertEquals(1, parsed.submissions().size());
    assertEquals(1, parsed.activity().size());
    assertEquals(HexUint64.of(21000), parsed.gas());
    assertEquals(HexUint256.of(0), parsed.value());
    assertEquals(HexUint256.of(1), parsed.maxPriorityFeePerGas());
    assertEquals(HexUint256.of(2), parsed.maxFeePerGas());
    assertEquals(transactionId, parsed.transaction());
    assertEquals(TransactionType.PUBLIC, parsed.transactionType());
    assertEquals("alice", parsed.sender());
    assertEquals("0x2222222222222222222222222222222222222222", parsed.contractAddress());
    assertTrue(
        publicTx.toString().contains("alice") || publicTx.toString().contains(from.toString()));
  }

  @Test
  void defaultsAndZeroTimestampsAreNormalized() {
    final PublicTxWithBinding publicTx =
        new PublicTxWithBinding(
            null,
            null,
            null,
            null,
            null,
            Timestamp.ZERO,
            null,
            Timestamp.ZERO,
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
            null,
            null,
            null);

    assertNull(publicTx.created());
    assertNull(publicTx.completedAt());
    assertTrue(publicTx.submissions().isEmpty());
    assertTrue(publicTx.activity().isEmpty());
  }
}
