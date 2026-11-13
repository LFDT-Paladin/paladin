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
package org.lfdt.paladin.sdk.domains.zeto;

import java.util.List;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.AbiParameter;
import org.lfdt.paladin.sdk.core.abi.StateMutability;

/**
 * The ABIs used to transact with Zeto: the constructor understood by the Zeto factory, the private
 * interface implemented by the domain, and the public functions of the base-ledger contract.
 */
public final class ZetoAbi {

  private static final AbiParameter TRANSFER_LIST =
      AbiParameter.builder("transfers", "tuple[]")
          .internalType("struct IZetoFungible.TransferParam[]")
          .component(param("to", "string"))
          .component(param("amount", "uint256"))
          .component(bytes("data"))
          .build();

  private static final AbiParameter MINT_LIST =
      AbiParameter.builder("mints", "tuple[]")
          .internalType("struct IZetoFungible.TransferParam[]")
          .component(param("to", "string"))
          .component(param("amount", "uint256"))
          .component(bytes("data"))
          .build();

  /** The constructor accepted by the Zeto factory. */
  public static final AbiEntry CONSTRUCTOR =
      AbiEntry.constructor().input(param("tokenName", "string")).build();

  /** The private Zeto interface, invoked through the domain. */
  public static final List<AbiEntry> PRIVATE =
      List.of(
          function("mint", MINT_LIST),
          function("transfer", TRANSFER_LIST),
          function(
              "transferLocked",
              param("lockedInputs", "uint256[]"),
              param("delegate", "string"),
              TRANSFER_LIST),
          function("lock", param("amount", "uint256"), param("delegate", "address")),
          function("deposit", param("amount", "uint256")),
          function("withdraw", param("amount", "uint256")),
          function("setERC20", param("erc20", "address")),
          view(
              "balanceOf",
              List.of(param("account", "string")),
              List.of(
                  param("totalStates", "uint256"),
                  param("totalBalance", "uint256"),
                  param("overflow", "bool"))));

  /** The public functions of the Zeto contract on the base ledger. */
  public static final List<AbiEntry> PUBLIC =
      List.of(
          function(
              "delegateLock",
              param("utxos", "uint256[]"),
              param("delegate", "address"),
              bytes("data")));

  private ZetoAbi() {}

  private static AbiEntry function(final String name, final AbiParameter... inputs) {
    return AbiEntry.function(name)
        .stateMutability(StateMutability.NONPAYABLE)
        .inputs(List.of(inputs))
        .build();
  }

  private static AbiEntry view(
      final String name, final List<AbiParameter> inputs, final List<AbiParameter> outputs) {
    return AbiEntry.function(name)
        .stateMutability(StateMutability.VIEW)
        .inputs(inputs)
        .outputs(outputs)
        .build();
  }

  private static AbiParameter param(final String name, final String type) {
    return AbiParameter.of(name, type);
  }

  private static AbiParameter bytes(final String name) {
    return AbiParameter.of(name, "bytes");
  }
}
