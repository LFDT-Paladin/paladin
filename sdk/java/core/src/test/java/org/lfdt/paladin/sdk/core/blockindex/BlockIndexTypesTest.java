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
package org.lfdt.paladin.sdk.core.blockindex;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;

class BlockIndexTypesTest {

  private static final ObjectMapper MAPPER = PaladinObjectMapper.shared();

  private static final String HASH =
      "0x2222222222222222222222222222222222222222222222222222222222222222";
  private static final String SIG =
      "0x3333333333333333333333333333333333333333333333333333333333333333";

  private static final String BLOCK_JSON =
      "{\"number\":42,\"hash\":\"0x"
          + "1111111111111111111111111111111111111111111111111111111111111111\","
          + "\"timestamp\":\"2024-01-01T00:00:00Z\"}";

  private static final String TX_JSON =
      "{\"hash\":\""
          + HASH
          + "\",\"blockNumber\":42,\"transactionIndex\":1,"
          + "\"from\":\"0x1111111111111111111111111111111111111111\","
          + "\"to\":\"0x2222222222222222222222222222222222222222\","
          + "\"nonce\":9,"
          + "\"contractAddress\":\"0x3333333333333333333333333333333333333333\","
          + "\"result\":\"success\",\"block\":"
          + BLOCK_JSON
          + "}";

  @Test
  void ethTransactionResultRoundTripsAndRejectsUnknown() throws Exception {
    for (final EthTransactionResult r : EthTransactionResult.values()) {
      assertEquals(r, MAPPER.readValue(MAPPER.writeValueAsString(r), EthTransactionResult.class));
    }
    assertEquals("success", EthTransactionResult.SUCCESS.jsonValue());
    assertEquals(EthTransactionResult.FAILURE, EthTransactionResult.fromJson("FAILURE"));
    assertThrows(IllegalArgumentException.class, () -> EthTransactionResult.fromJson("bogus"));
    assertThrows(IllegalArgumentException.class, () -> EthTransactionResult.fromJson(null));
  }

  @Test
  void indexedBlockRoundTrips() throws Exception {
    final IndexedBlock block = MAPPER.readValue(BLOCK_JSON, IndexedBlock.class);
    assertEquals(42L, block.number());
    assertEquals(
        Bytes32.fromString("0x1111111111111111111111111111111111111111111111111111111111111111"),
        block.hash());
    assertTrue(block.timestamp().toString().startsWith("2024-01-01"));

    final IndexedBlock reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(block), IndexedBlock.class);
    assertEquals(block, reparsed);
    assertEquals(block.hashCode(), reparsed.hashCode());
    assertEquals(block, block);
    assertNotEquals(block, "not a block");
    assertTrue(block.toString().contains("number=42"));
  }

  @Test
  void indexedTransactionRoundTrips() throws Exception {
    final IndexedTransaction tx = MAPPER.readValue(TX_JSON, IndexedTransaction.class);
    assertEquals(Bytes32.fromString(HASH), tx.hash());
    assertEquals(42L, tx.blockNumber());
    assertEquals(1L, tx.transactionIndex());
    assertEquals(9L, tx.nonce());
    assertEquals(EthTransactionResult.SUCCESS, tx.result());
    assertEquals(EthAddress.fromString("0x1111111111111111111111111111111111111111"), tx.from());
    assertEquals(EthAddress.fromString("0x2222222222222222222222222222222222222222"), tx.to());
    assertEquals(
        EthAddress.fromString("0x3333333333333333333333333333333333333333"), tx.contractAddress());
    assertEquals(42L, tx.block().number());

    final IndexedTransaction reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(tx), IndexedTransaction.class);
    assertEquals(tx, reparsed);
    assertEquals(tx.hashCode(), reparsed.hashCode());
    assertEquals(tx, tx);
    assertNotEquals(tx, "not a tx");
    assertTrue(tx.toString().contains("blockNumber=42"));
  }

  @Test
  void indexedTransactionOmitsAbsentOptionalFields() throws Exception {
    final IndexedTransaction tx =
        MAPPER.readValue(
            "{\"hash\":\"" + HASH + "\",\"blockNumber\":42,\"transactionIndex\":1,\"nonce\":0}",
            IndexedTransaction.class);
    assertNull(tx.to());
    assertNull(tx.contractAddress());
    assertNull(tx.result());
    assertNull(tx.block());
  }

  @Test
  void indexedEventRoundTrips() throws Exception {
    final String json =
        "{\"blockNumber\":42,\"transactionIndex\":1,\"logIndex\":0,"
            + "\"transactionHash\":\""
            + HASH
            + "\",\"signature\":\""
            + SIG
            + "\",\"transaction\":"
            + TX_JSON
            + ",\"block\":"
            + BLOCK_JSON
            + "}";
    final IndexedEvent event = MAPPER.readValue(json, IndexedEvent.class);
    assertEquals(42L, event.blockNumber());
    assertEquals(1L, event.transactionIndex());
    assertEquals(0L, event.logIndex());
    assertEquals(Bytes32.fromString(HASH), event.transactionHash());
    assertEquals(Bytes32.fromString(SIG), event.signature());
    assertEquals(9L, event.transaction().nonce());
    assertEquals(42L, event.block().number());

    final IndexedEvent reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(event), IndexedEvent.class);
    assertEquals(event, reparsed);
    assertEquals(event.hashCode(), reparsed.hashCode());
    assertEquals(event, event);
    assertNotEquals(event, "not an event");
    assertTrue(event.toString().contains("logIndex=0"));
  }

  @Test
  void eventWithDataRoundTrips() throws Exception {
    final String json =
        "{\"blockNumber\":42,\"transactionIndex\":1,\"logIndex\":0,"
            + "\"transactionHash\":\""
            + HASH
            + "\",\"signature\":\""
            + SIG
            + "\",\"soliditySignature\":\"Transfer(address,address,uint256)\","
            + "\"address\":\"0x2222222222222222222222222222222222222222\","
            + "\"data\":{\"value\":\"100\"}}";
    final EventWithData event = MAPPER.readValue(json, EventWithData.class);
    assertEquals(42L, event.blockNumber());
    assertEquals(1L, event.transactionIndex());
    assertEquals(0L, event.logIndex());
    assertEquals(Bytes32.fromString(HASH), event.transactionHash());
    assertEquals(Bytes32.fromString(SIG), event.signature());
    assertNull(event.transaction());
    assertNull(event.block());
    assertEquals("Transfer(address,address,uint256)", event.soliditySignature());
    assertEquals(
        EthAddress.fromString("0x2222222222222222222222222222222222222222"), event.address());
    assertEquals("100", event.data().get("value").asText());

    final EventWithData reparsed =
        MAPPER.readValue(MAPPER.writeValueAsString(event), EventWithData.class);
    assertEquals(event, reparsed);
    assertEquals(event.hashCode(), reparsed.hashCode());
    assertEquals(event, event);
    assertNotEquals(event, "not an event");
    assertTrue(event.toString().contains("logIndex=0"));
  }
}
