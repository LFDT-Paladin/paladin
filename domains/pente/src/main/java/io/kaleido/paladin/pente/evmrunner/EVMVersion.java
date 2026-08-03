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

package io.kaleido.paladin.pente.evmrunner;

import org.hyperledger.besu.evm.EVM;
import org.hyperledger.besu.evm.MainnetEVMs;
import org.hyperledger.besu.evm.gascalculator.CancunGasCalculator;
import org.hyperledger.besu.evm.gascalculator.GasCalculator;
import org.hyperledger.besu.evm.gascalculator.LondonGasCalculator;
import org.hyperledger.besu.evm.gascalculator.ShanghaiGasCalculator;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.hyperledger.besu.evm.precompile.MainnetPrecompiledContracts;
import org.hyperledger.besu.evm.precompile.PrecompileContractRegistry;

import java.math.BigInteger;

public record EVMVersion(GasCalculator gasCalculator, EvmConfiguration evmConfiguration, PrecompileContractRegistry precompileContractRegistry, EVM evm) {

    public static EVMVersion London(long chainId, EvmConfiguration evmConfiguration) {
        var evm = MainnetEVMs.london(BigInteger.valueOf(chainId), evmConfiguration);
        var gasCalculator = new LondonGasCalculator();
        var precompileContractRegistry = MainnetPrecompiledContracts.istanbul(gasCalculator);
        return new EVMVersion(gasCalculator, evmConfiguration, precompileContractRegistry, evm);
    }

    public static EVMVersion Paris(long chainId, EvmConfiguration evmConfiguration) {
        var evm = MainnetEVMs.paris(BigInteger.valueOf(chainId), evmConfiguration);
        var gasCalculator = new LondonGasCalculator();
        var precompileContractRegistry = MainnetPrecompiledContracts.istanbul(gasCalculator);
        return new EVMVersion(gasCalculator, evmConfiguration, precompileContractRegistry, evm);
    }

    public static EVMVersion Shanghai(long chainId, EvmConfiguration evmConfiguration) {
        var evm = MainnetEVMs.shanghai(BigInteger.valueOf(chainId), evmConfiguration);
        var gasCalculator = new ShanghaiGasCalculator();
        var precompileContractRegistry = MainnetPrecompiledContracts.istanbul(gasCalculator);
        return new EVMVersion(gasCalculator, evmConfiguration, precompileContractRegistry, evm);
    }

    public static EVMVersion Cancun(long chainId, EvmConfiguration evmConfiguration) {
        var evm = MainnetEVMs.cancun(BigInteger.valueOf(chainId), evmConfiguration);
        var gasCalculator = new CancunGasCalculator();
        var precompileContractRegistry = MainnetPrecompiledContracts.cancun(gasCalculator);
        return new EVMVersion(gasCalculator, evmConfiguration, precompileContractRegistry, evm);
    }
}
