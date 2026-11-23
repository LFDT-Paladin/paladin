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
import org.hyperledger.besu.evm.gascalculator.OsakaGasCalculator;
import org.hyperledger.besu.evm.gascalculator.PragueGasCalculator;
import org.hyperledger.besu.evm.gascalculator.ShanghaiGasCalculator;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.hyperledger.besu.evm.precompile.KZGPointEvalPrecompiledContract;
import org.hyperledger.besu.evm.precompile.MainnetPrecompiledContracts;
import org.hyperledger.besu.evm.precompile.PrecompileContractRegistry;

import java.math.BigInteger;
import java.util.Map;
import java.util.function.BiFunction;

public record EVMVersion(GasCalculator gasCalculator, EvmConfiguration evmConfiguration, PrecompileContractRegistry precompileContractRegistry, EVM evm) {

    private static final Map<String, BiFunction<Long, EvmConfiguration, EVMVersion>> BY_NAME = Map.of(
            "london", EVMVersion::London,
            "paris", EVMVersion::Paris,
            "shanghai", EVMVersion::Shanghai,
            "cancun", EVMVersion::Cancun,
            "prague", EVMVersion::Prague,
            "osaka", EVMVersion::Osaka
    );

    // The KZG point evaluation precompile, from Cancun onwards, needs its native library and trusted setup loaded by
    // whatever runs the EVM. Besu allows that only once per class loader, so it is loaded when this class initializes.
    private static final class KZG {
        static {
            KZGPointEvalPrecompiledContract.init();
        }

        static void load() {
        }
    }

    public static void loadKZG() {
        KZG.load();
    }

    public static EVMVersion forName(String name, long chainId, EvmConfiguration evmConfiguration) {
        validateName(name);
        return BY_NAME.get(name).apply(chainId, evmConfiguration);
    }

    public static void validateName(String name) {
        if (name == null || !BY_NAME.containsKey(name)) {
            throw new IllegalArgumentException("unknown EVM version '%s'".formatted(name));
        }
    }

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

    public static EVMVersion Prague(long chainId, EvmConfiguration evmConfiguration) {
        var evm = MainnetEVMs.prague(BigInteger.valueOf(chainId), evmConfiguration);
        var gasCalculator = new PragueGasCalculator();
        var precompileContractRegistry = MainnetPrecompiledContracts.prague(gasCalculator);
        return new EVMVersion(gasCalculator, evmConfiguration, precompileContractRegistry, evm);
    }

    public static EVMVersion Osaka(long chainId, EvmConfiguration evmConfiguration) {
        var evm = MainnetEVMs.osaka(BigInteger.valueOf(chainId), evmConfiguration);
        var gasCalculator = new OsakaGasCalculator();
        var precompileContractRegistry = MainnetPrecompiledContracts.osaka(gasCalculator);
        return new EVMVersion(gasCalculator, evmConfiguration, precompileContractRegistry, evm);
    }
}
