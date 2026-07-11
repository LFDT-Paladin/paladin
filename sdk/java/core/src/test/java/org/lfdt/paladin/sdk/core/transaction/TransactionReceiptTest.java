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
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.Timestamp;

class TransactionReceiptTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Test
    void omitsUnsetOnchainFieldsButKeepsSequence() throws Exception {
        // A receipt that has not been finalized on-chain has no hash/block/index, but sequence is always emitted.
        final TransactionReceipt receipt = TransactionReceipt.builder()
                .id(UUID.fromString("3e6b4f1c-0000-0000-0000-000000000001"))
                .sequence(7)
                .build();
        assertEquals("{\"id\":\"3e6b4f1c-0000-0000-0000-000000000001\",\"sequence\":7}",
                MAPPER.writeValueAsString(receipt));
    }

    @Test
    void roundTripsSuccessfulOnchainReceipt() throws Exception {
        final UUID id = UUID.randomUUID();
        final TransactionReceipt receipt = TransactionReceipt.builder()
                .id(id)
                .indexed(Timestamp.fromString("2024-06-18T12:00:00Z"))
                .sequence(42)
                .domain("noto")
                .success(true)
                .transactionHash(Bytes32.fromString(
                        "0x1111111111111111111111111111111111111111111111111111111111111111"))
                .blockNumber(100L)
                .transactionIndex(3L)
                .contractAddress(EthAddress.fromString("0x05d936207F04D81a85881b72A0D17854Ee8BE45A"))
                .build();

        final TransactionReceipt parsed = MAPPER.readValue(MAPPER.writeValueAsString(receipt), TransactionReceipt.class);
        assertEquals(receipt, parsed);
        assertEquals(id, parsed.id());
        assertTrue(parsed.success());
        assertEquals(42, parsed.sequence());
        assertEquals(100L, parsed.blockNumber());
        assertEquals(3L, parsed.transactionIndex());
        assertNull(parsed.logIndex());
    }

    @Test
    void roundTripsFailedReceipt() throws Exception {
        final TransactionReceipt receipt = TransactionReceipt.builder()
                .id(UUID.randomUUID())
                .sequence(1)
                .success(false)
                .failureMessage("execution reverted")
                .revertData(HexBytes.fromString("0x08c379a0"))
                .build();

        final String json = MAPPER.writeValueAsString(receipt);
        assertFalse(json.contains("\"success\"")); // false is omitted (NON_DEFAULT)
        assertTrue(json.contains("\"failureMessage\":\"execution reverted\""));

        final TransactionReceipt parsed = MAPPER.readValue(json, TransactionReceipt.class);
        assertEquals(receipt, parsed);
        assertFalse(parsed.success());
        assertEquals("execution reverted", parsed.failureMessage());
        assertEquals(HexBytes.fromString("0x08c379a0"), parsed.revertData());
    }

    @Test
    void distinguishesZeroBlockFromAbsent() throws Exception {
        final TransactionReceipt onchainZero = TransactionReceipt.builder().blockNumber(0L).transactionIndex(0L).build();
        final String json = MAPPER.writeValueAsString(onchainZero);
        assertTrue(json.contains("\"blockNumber\":0"));
        assertTrue(json.contains("\"transactionIndex\":0"));

        final TransactionReceipt offchain = TransactionReceipt.builder().build();
        assertNull(offchain.blockNumber());
        assertFalse(MAPPER.writeValueAsString(offchain).contains("blockNumber"));
    }

    @Test
    void parsesEventFinalizedReceipt() throws Exception {
        final String json = "{"
                + "\"sequence\":5,"
                + "\"success\":true,"
                + "\"logIndex\":2,"
                + "\"source\":\"0x05d936207F04D81a85881b72A0D17854Ee8BE45A\"}";
        final TransactionReceipt receipt = MAPPER.readValue(json, TransactionReceipt.class);
        assertEquals(2L, receipt.logIndex());
        assertEquals(EthAddress.fromString("0x05d936207F04D81a85881b72A0D17854Ee8BE45A"), receipt.source());
        assertNull(receipt.transactionHash());
    }
}
