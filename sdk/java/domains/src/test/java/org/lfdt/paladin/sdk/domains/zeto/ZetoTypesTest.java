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

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.EntryType;
import org.lfdt.paladin.sdk.core.abi.StateMutability;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;

class ZetoTypesTest {

  private static final ObjectMapper MAPPER = PaladinObjectMapper.shared();
  private static final HexUint256 AMOUNT = HexUint256.of(7);
  private static final HexBytes EMPTY = HexBytes.fromString("0x");
  private static final EthAddress ERC20 =
      EthAddress.fromString("0x0102030405060708090a0b0c0d0e0f1011121314");

  @Test
  void unsetTransferBytesDefaultToEmpty() {
    assertEquals(EMPTY, new ZetoTransfer("bob", AMOUNT, null).data());
  }

  @Test
  void suppliedBytesAreKept() {
    final HexBytes data = HexBytes.fromString("0xbeef");
    assertSame(data, new ZetoTransfer("bob", AMOUNT, data).data());
  }

  @Test
  void balanceOfResultReadsHexValues() throws Exception {
    final ZetoBalanceOfResult result =
        MAPPER.readValue(
            "{\"totalBalance\":\"0x64\",\"totalStates\":\"3\",\"overflow\":false,\"extra\":1}",
            ZetoBalanceOfResult.class);

    assertEquals(new ZetoBalanceOfResult(HexUint256.of(100), HexUint256.of(3), false), result);
  }

  @Test
  void verifierConstants() {
    assertEquals(
        "domain:zeto:snark:babyjubjub", ZetoVerifiers.algorithmZetoSnarkBabyJubjub("zeto"));
    assertEquals(
        "iden3_pubkey_babyjubjub_compressed_0x",
        ZetoVerifiers.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X);
    assertThrows(
        NullPointerException.class, () -> ZetoVerifiers.algorithmZetoSnarkBabyJubjub(null));
  }

  @Test
  void abiListsThePrivateAndPublicFunctions() {
    assertEquals(
        List.of(
            "mint",
            "transfer",
            "transferLocked",
            "lock",
            "deposit",
            "withdraw",
            "setERC20",
            "balanceOf"),
        ZetoAbi.PRIVATE.stream().map(AbiEntry::name).toList());
    assertEquals(List.of("delegateLock"), ZetoAbi.PUBLIC.stream().map(AbiEntry::name).toList());

    assertEquals(EntryType.CONSTRUCTOR, ZetoAbi.CONSTRUCTOR.type());
    assertEquals("tokenName", ZetoAbi.CONSTRUCTOR.inputs().get(0).name());

    final AbiEntry balanceOf = ZetoAbi.PRIVATE.get(ZetoAbi.PRIVATE.size() - 1);
    assertEquals(StateMutability.VIEW, balanceOf.stateMutability());
    assertEquals(3, balanceOf.outputs().size());

    final AbiEntry transferLocked = ZetoAbi.PRIVATE.get(2);
    assertEquals("uint256[]", transferLocked.inputs().get(0).type());
    assertEquals("tuple[]", transferLocked.inputs().get(2).type());
    assertEquals(3, transferLocked.inputs().get(2).components().size());
    assertTrue(
        ZetoAbi.PUBLIC.stream().allMatch(e -> e.stateMutability() == StateMutability.NONPAYABLE));
  }
}
