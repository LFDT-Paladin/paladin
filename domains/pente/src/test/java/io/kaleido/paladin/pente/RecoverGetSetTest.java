/*
 * Copyright © 2024 Kaleido, Inc.
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

package io.kaleido.paladin.pente;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.logging.PaladinLogging;
import io.kaleido.paladin.pente.domain.PenteDomain;
import io.kaleido.paladin.pente.evmrunner.EVMRunner;
import io.kaleido.paladin.pente.evmrunner.EVMVersion;
import io.kaleido.paladin.toolkit.JsonHex;
import org.apache.logging.log4j.Logger;
import org.apache.tuweni.bytes.Bytes;
import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.evm.fluent.EVMExecutor;
import org.hyperledger.besu.evm.frame.MessageFrame;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.junit.jupiter.api.Test;
import org.web3j.abi.TypeReference;
import org.web3j.abi.datatypes.Type;
import org.web3j.abi.datatypes.Bool;

import java.io.IOException;
import java.io.InputStream;
import java.math.BigInteger;
import java.util.LinkedList;
import java.util.List;
import java.util.Optional;
import java.util.Random;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

public class RecoverGetSetTest {

    private static final Logger LOGGER = PaladinLogging.getLogger(RecoverGetSetTest.class);

    @Test
    void runAnEVM() throws IOException {

        // Generate a shiny new EVM
        EVMVersion evmVersion = EVMVersion.Cancun(new Random().nextLong(), EvmConfiguration.DEFAULT);
        EVMRunner evmRunner = new EVMRunner(evmVersion, address -> Optional.empty(), 0);

        // Load some bytecode for our first contract deploy
        String resourcePath = "contracts/testcontracts/Recover.sol/Recover.json";
        String hexByteCode;
        try (InputStream is = Thread.currentThread().getContextClassLoader().getResourceAsStream(resourcePath)) {
            assertNotNull(is);
            JsonNode node = new ObjectMapper().readTree(is);
            hexByteCode = node.get("bytecode").asText();
        }
        final Address smartContractAddress = EVMRunner.randomAddress();
        final Address sender = EVMRunner.randomAddress();
        final var logs = new LinkedList<EVMRunner.JsonEVMLog>();

        MessageFrame deployFrame = evmRunner.runContractDeployment(
                sender,
                smartContractAddress,
                Bytes.fromHexString(hexByteCode),
                Long.MAX_VALUE,
                logs
        );
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, deployFrame.getState());
        final var signedMessage = JsonHex.from("0xacaf3289d7b601cbd114fb36c4d29c85bbfd5e133f14cb355c3fd8d99367964f");
        final var signature = JsonHex.from("0xe76d4a6f194440ca1b19695e41538b960afc9c27c69722ef93cbf0134cbc6fd317481bff0ca56883b81fd37dcf009d7b9c98c793a67826602b8d8eb83a8b94c51b");
        final var expectedAddress = JsonHex.from("0x78826125b6be403ea159876f5a32a3eac7cd0fe5");
        MessageFrame verifySignatureFrame = evmRunner.runContractInvoke(
                sender,
                smartContractAddress,
                "verifySignature",
                Long.MAX_VALUE,
                logs,
                new org.web3j.abi.datatypes.generated.Bytes32(signedMessage.getBytes()),
                new org.web3j.abi.datatypes.DynamicBytes(signature.getBytes()),
                new org.web3j.abi.datatypes.Address(expectedAddress.toString())
        );
        if (verifySignatureFrame.getRevertReason().isPresent()) {
            LOGGER.error("revert reason: {}", verifySignatureFrame.getRevertReason().get());
        }
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, verifySignatureFrame.getState());
        List<Type<?>> returns = evmRunner.decodeReturn(verifySignatureFrame, List.of(
                new TypeReference<Bool>() {},
                new TypeReference<org.web3j.abi.datatypes.Address>() {}
        ));
        assertEquals("true", returns.get(0).getValue().toString());
        assertEquals("0x78826125b6be403ea159876f5a32a3eac7cd0fe5", returns.get(1).getValue().toString());

    }

}
