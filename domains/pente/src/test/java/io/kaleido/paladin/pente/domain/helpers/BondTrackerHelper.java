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

package io.kaleido.paladin.pente.domain.helpers;

import io.kaleido.paladin.toolkit.*;
import org.web3j.abi.datatypes.Address;
import org.web3j.abi.datatypes.Function;
import org.web3j.abi.datatypes.Utf8String;

import java.io.IOException;
import java.util.Arrays;
import java.util.HashMap;

public class BondTrackerHelper {
    final PenteHelper pente;
    final JsonABI abi;
    final JsonHex.Address address;

    public static BondTrackerHelper deploy(PenteHelper pente, String sender, String name, String symbol, String custodian, String publicTracker) throws IOException {
        String bytecode = ResourceLoader.jsonResourceEntryText(
                BondTrackerHelper.class.getClassLoader(),
                "contracts/private/BondTracker.sol/BondTracker.json",
                "bytecode"
        );
        JsonABI abi = JsonABI.fromJSONResourceEntry(
                BondTrackerHelper.class.getClassLoader(),
                "contracts/private/BondTracker.sol/BondTracker.json",
                "abi"
        );
        var address = pente.deployProxy(sender, bytecode, new Function(
                "initialize",
                Arrays.asList(
                        new Utf8String(name),
                        new Utf8String(symbol),
                        new Address(custodian),
                        new Address(publicTracker)
                ),
                Arrays.asList()
        ));
        return new BondTrackerHelper(pente, abi, address);
    }

    private BondTrackerHelper(PenteHelper pente, JsonABI abi, JsonHex.Address address) {
        this.pente = pente;
        this.abi = abi;
        this.address = address;
    }

    public JsonHex.Address address() {
        return address;
    }

    public InvestorListHelper investorList(String sender) throws IOException {
        var method = abi.getABIEntry("function", "investorList");
        var output = pente.call(
                method.name(),
                method.inputs(),
                JsonABI.newParameters(
                        JsonABI.newParameter("output", "address")
                ),
                sender,
                address,
                new HashMap<>()
        );
        return new InvestorListHelper(pente, JsonHex.addressFrom(output.output()));
    }

    public String balanceOf(String sender, String account) throws IOException {
        var method = abi.getABIEntry("function", "balanceOf");
        var output = pente.call(
                method.name(),
                method.inputs(),
                JsonABI.newParameters(
                        JsonABI.newParameter("output", "uint256")
                ),
                sender,
                address,
                new HashMap<>() {{
                    put("account", account);
                }}
        );
        return output.output();
    }

    public void beginDistribution(String sender, int discountPrice, int minimumDenomination) throws IOException {
        var method = abi.getABIEntry("function", "beginDistribution");
        pente.invoke(
                method.name(),
                method.inputs(),
                sender,
                address,
                new HashMap<>() {{
                    put("discountPrice", discountPrice);
                    put("minimumDenomination", minimumDenomination);
                }}
        );
    }
}
