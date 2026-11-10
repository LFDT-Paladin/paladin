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

import io.kaleido.paladin.pente.evmrunner.EVMRunner;
import io.kaleido.paladin.pente.evmrunner.EVMVersion;
import io.kaleido.paladin.pente.evmstate.PersistedAccount;
import io.kaleido.paladin.toolkit.JsonHex;
import org.apache.tuweni.bytes.Bytes;
import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.evm.frame.MessageFrame;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.junit.jupiter.api.Test;
import org.web3j.abi.TypeReference;
import org.web3j.abi.datatypes.generated.Uint256;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.IOException;
import java.io.InputStream;
import java.util.LinkedList;
import java.util.List;
import java.util.Optional;
import java.util.Random;

import static org.junit.jupiter.api.Assertions.*;

public class EVMRunnerTest {

    @Test
    void testNonceSmartContractAddress() throws Exception {
        var addr = Address.fromHexString("0x6ac7ea33f8831ea9dcc53393aaa88b25a785dbf0");
        assertEquals("0xcd234a471b72ba2f1ccf0a70fcaba648a5eecd8d", EVMRunner.nonceSmartContractAddress(addr, 0).toHexString());
        assertEquals("0x343c43a37d37dff08ae8c4a11544c718abb4fcf8", EVMRunner.nonceSmartContractAddress(addr, 1).toHexString());
    }

    private static Bytes loadBytecode(String resourcePath) throws IOException {
        try (InputStream is = Thread.currentThread().getContextClassLoader().getResourceAsStream(resourcePath)) {
            assertNotNull(is);
            JsonNode node = new ObjectMapper().readTree(is);
            return Bytes.fromHexString(node.get("bytecode").asText());
        }
    }

    @Test
    void testLogAddressFromDelegateCallEmitter() throws IOException {
        EVMVersion evmVersion = EVMVersion.Shanghai(new Random().nextLong(), EvmConfiguration.DEFAULT);
        EVMRunner evmRunner = new EVMRunner(evmVersion, address -> Optional.empty(), 0, 0);

        final Address sender = EVMRunner.randomAddress();
        final Address implementationAddress = EVMRunner.randomAddress();
        final Address wrapperAddress = EVMRunner.randomAddress();
        final var logs = new LinkedList<EVMRunner.JsonEVMLog>();

        MessageFrame implFrame = evmRunner.runContractDeployment(
                sender,
                implementationAddress,
                loadBytecode("contracts/testcontracts/SimpleStorage.sol/SimpleStorage.json"),
                Long.MAX_VALUE,
                logs,
                new Uint256(0)
        );
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, implFrame.getState());

        MessageFrame wrapperFrame = evmRunner.runContractDeployment(
                sender,
                wrapperAddress,
                loadBytecode("contracts/testcontracts/SimpleStorageCloned.sol/SimpleStorageCloned.json"),
                Long.MAX_VALUE,
                logs,
                new org.web3j.abi.datatypes.Address(implementationAddress.toHexString())
        );
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, wrapperFrame.getState());

        MessageFrame ssFrame = evmRunner.runContractInvoke(sender, wrapperAddress, "ss", Long.MAX_VALUE, logs);
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, ssFrame.getState());
        var cloneAddress = new JsonHex.Address(((org.web3j.abi.datatypes.Address)
                evmRunner.decodeReturn(ssFrame, List.of(new TypeReference<org.web3j.abi.datatypes.Address>() {})).getFirst()
        ).getValue());

        logs.clear();
        MessageFrame setFrame = evmRunner.runContractInvoke(sender, wrapperAddress, "set", Long.MAX_VALUE, logs, new Uint256(23456));
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, setFrame.getState());
        assertEquals(1, logs.size());
        assertEquals(cloneAddress, logs.getFirst().address());

        logs.clear();
        setFrame = evmRunner.runContractInvoke(sender, Address.fromHexString(cloneAddress.toString()), "set", Long.MAX_VALUE, logs, new Uint256(34567));
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, setFrame.getState());
        assertEquals(1, logs.size());
        assertEquals(cloneAddress, logs.getFirst().address());
    }
}
