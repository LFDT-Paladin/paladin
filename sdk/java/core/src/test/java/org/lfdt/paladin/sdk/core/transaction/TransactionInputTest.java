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
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.AbiParameter;
import org.lfdt.paladin.sdk.core.abi.StateMutability;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;
import org.lfdt.paladin.sdk.core.types.HexUint64;

class TransactionInputTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Test
    void emptyInputSerializesToEmptyObject() throws Exception {
        // Every field is omitempty in the Go reference, so a bare input emits no keys.
        assertEquals("{}", MAPPER.writeValueAsString(TransactionInput.builder().build()));
    }

    @Test
    void omitsEmptyListsAndNulls() throws Exception {
        TransactionInput input = TransactionInput.builder()
                .type(TransactionType.PUBLIC)
                .from("key1")
                .build();
        String json = MAPPER.writeValueAsString(input);
        assertEquals("{\"type\":\"public\",\"from\":\"key1\"}", json);
        assertTrue(input.dependsOn().isEmpty());
        assertTrue(input.abi().isEmpty());
    }

    @Test
    void roundTripsFullInvoke() throws Exception {
        UUID dep = UUID.randomUUID();
        EthAddress to = EthAddress.fromString("0x05d936207F04D81a85881b72A0D17854Ee8BE45A");
        TransactionInput input = TransactionInput.builder()
                .idempotencyKey("idem-1")
                .type(TransactionType.PRIVATE)
                .domain("noto")
                .function("transfer")
                .from("alice@node1")
                .to(to)
                .data(MAPPER.readTree("{\"to\":\"bob\",\"amount\":\"100\"}"))
                .gas(HexUint64.of(0x5208))
                .value(HexUint256.of(1000))
                .maxPriorityFeePerGas(HexUint256.of(2))
                .maxFeePerGas(HexUint256.of(10))
                .dependsOn(dep)
                .abiEntry(AbiEntry.function("transfer")
                        .stateMutability(StateMutability.NONPAYABLE)
                        .input(AbiParameter.of("to", "string"))
                        .input(AbiParameter.of("amount", "uint256"))
                        .build())
                .build();

        TransactionInput parsed = MAPPER.readValue(MAPPER.writeValueAsString(input), TransactionInput.class);
        assertEquals(input, parsed);
        assertEquals(TransactionType.PRIVATE, parsed.type());
        assertEquals("transfer", parsed.function());
        assertEquals(to, parsed.to());
        assertEquals(1, parsed.dependsOn().size());
        assertEquals(dep, parsed.dependsOn().get(0));
        assertEquals(1, parsed.abi().size());
        assertEquals("100", parsed.data().get("amount").asText());
    }

    @Test
    void roundTripsDeployWithBytecode() throws Exception {
        TransactionInput input = TransactionInput.builder()
                .type(TransactionType.PUBLIC)
                .from("deployer")
                .bytecode(HexBytes.fromString("0x60806040"))
                .abiEntry(AbiEntry.constructor().build())
                .build();

        TransactionInput parsed = MAPPER.readValue(MAPPER.writeValueAsString(input), TransactionInput.class);
        assertEquals(input, parsed);
        assertEquals(HexBytes.fromString("0x60806040"), parsed.bytecode());
        assertNull(parsed.to()); // null for a deploy
        assertEquals(1, parsed.abi().size());
    }

    @Test
    void parsesNodeStyleJson() throws Exception {
        String json = "{"
                + "\"type\":\"public\","
                + "\"from\":\"key1\","
                + "\"to\":\"0x05d936207F04D81a85881b72A0D17854Ee8BE45A\","
                + "\"gas\":\"0x5208\","
                + "\"value\":\"0x0a\","
                + "\"data\":[\"0x1234\"]}";
        TransactionInput input = MAPPER.readValue(json, TransactionInput.class);
        assertEquals(TransactionType.PUBLIC, input.type());
        assertEquals(0x5208, input.gas().asUnsignedLong());
        assertEquals(HexUint256.of(10), input.value());
        assertTrue(input.data().isArray());
    }

    @Test
    void rejectsUnknownTransactionType() {
        assertThrows(Exception.class,
                () -> MAPPER.readValue("{\"type\":\"hybrid\"}", TransactionInput.class));
    }

    @Test
    void ignoresUnknownPropertiesIsNotDefaultOnPlainMapper() {
        // A plain ObjectMapper fails on unknown fields; the SDK-wide PaladinObjectMapper relaxes this.
        assertThrows(Exception.class,
                () -> MAPPER.readValue("{\"type\":\"public\",\"bogus\":1}", TransactionInput.class));
    }

    @Test
    void builderDefaultsAreEmpty() {
        TransactionInput input = TransactionInput.builder().build();
        assertTrue(input.dependsOn().isEmpty());
        assertTrue(input.abi().isEmpty());
        assertFalse(input.equals(TransactionInput.builder().from("x").build()));
    }
}
