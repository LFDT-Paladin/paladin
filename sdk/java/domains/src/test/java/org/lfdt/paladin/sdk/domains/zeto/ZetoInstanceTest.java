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
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Optional;
import java.util.UUID;
import java.util.function.Function;
import java.util.stream.Stream;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;

class ZetoInstanceTest {

  private static final String TX_ID = "00000000-0000-0000-0000-0000000000aa";
  private static final EthAddress ADDRESS =
      EthAddress.fromString("0x0102030405060708090a0b0c0d0e0f1011121314");
  private static final EthAddress DELEGATE =
      EthAddress.fromString("0x1111111111111111111111111111111111111111");
  private static final EthAddress ERC20 =
      EthAddress.fromString("0x2222222222222222222222222222222222222222");
  private static final HexUint256 AMOUNT = HexUint256.of(10);
  private static final HexBytes DATA = HexBytes.fromString("0x1234");
  private static final List<ZetoTransfer> TRANSFERS =
      List.of(new ZetoTransfer("bob", AMOUNT, DATA));

  private FakeRpcClient rpc;
  private ZetoInstance token;

  @BeforeEach
  void setUp() {
    rpc = new FakeRpcClient().respond("ptx_sendTransaction", "\"" + TX_ID + "\"");
    token = new ZetoInstance(PaladinClient.wrap(rpc), "zeto-test", ADDRESS);
  }

  /** One transaction operation: its ABI function, inputs, whether it is public, and the call. */
  private record Op(
      String function,
      Object wireData,
      boolean isPublic,
      Function<ZetoInstance, SentTransaction> send) {
    @Override
    public String toString() {
      return function;
    }
  }

  static Stream<Arguments> operations() {
    final ZetoMintParams mint = new ZetoMintParams(TRANSFERS);
    final ZetoTransferParams transfer = new ZetoTransferParams(TRANSFERS);
    final ZetoTransferLockedParams transferLocked =
        new ZetoTransferLockedParams(List.of(HexUint256.of(1)), "delegate@node1", TRANSFERS);
    final ZetoLockParams lock = new ZetoLockParams(AMOUNT, DELEGATE);
    final ZetoDelegateLockParams delegateLock =
        new ZetoDelegateLockParams(List.of(HexUint256.of(2)), DELEGATE);
    final ZetoSetErc20Params setErc20 = new ZetoSetErc20Params(ERC20);
    final ZetoDepositParams deposit = new ZetoDepositParams(AMOUNT);
    final ZetoWithdrawParams withdraw = new ZetoWithdrawParams(AMOUNT);

    final Object delegateLockWire =
        PaladinObjectMapper.shared()
            .valueToTree(
                java.util.Map.of(
                    "utxos",
                    delegateLock.utxos(),
                    "delegate",
                    delegateLock.delegate(),
                    "data",
                    HexBytes.fromString("0x")));

    return Stream.of(
            new Op("mint", mint, false, t -> t.mint("alice", mint, Optional.empty())),
            new Op(
                "transfer", transfer, false, t -> t.transfer("alice", transfer, Optional.empty())),
            new Op(
                "transferLocked",
                transferLocked,
                false,
                t -> t.transferLocked("alice", transferLocked, Optional.empty())),
            new Op("lock", lock, false, t -> t.lock("alice", lock, Optional.empty())),
            new Op(
                "delegateLock",
                delegateLockWire,
                true,
                t -> t.delegateLock("alice", delegateLock, Optional.empty())),
            new Op(
                "setERC20", setErc20, true, t -> t.setErc20("alice", setErc20, Optional.empty())),
            new Op("deposit", deposit, false, t -> t.deposit("alice", deposit, Optional.empty())),
            new Op(
                "withdraw", withdraw, false, t -> t.withdraw("alice", withdraw, Optional.empty())))
        .map(Arguments::of);
  }

  @ParameterizedTest
  @MethodSource("operations")
  void submitsEachOperation(final Op op) {
    final SentTransaction sent = op.send().apply(token);

    assertEquals(UUID.fromString(TX_ID), sent.id().join());
    final JsonNode body = rpc.lastCall("ptx_sendTransaction").body();
    assertEquals(op.isPublic() ? "public" : "private", body.get("type").asText());
    if (op.isPublic()) {
      assertFalse(body.has("domain"));
    } else {
      assertEquals("zeto-test", body.get("domain").asText());
    }
    assertEquals(op.function(), body.get("function").asText());
    assertEquals(ADDRESS.toString(), body.get("to").asText());
    assertEquals("alice", body.get("from").asText());
    assertFalse(body.has("idempotencyKey"));
    assertEquals(PaladinObjectMapper.shared().valueToTree(op.wireData()), body.get("data"));
    assertTrue(hasFunction(body.get("abi"), op.function()), "ABI lacks " + op.function());
  }

  @Test
  void setErc20RemembersTheContract() {
    token.setErc20("alice", new ZetoSetErc20Params(ERC20), Optional.empty());

    assertEquals(Optional.of(ERC20), token.erc20());

    final ZetoInstance moved = token.using(PaladinClient.wrap(rpc));
    assertEquals(Optional.of(ERC20), moved.erc20());
  }

  @Test
  void forwardsTheIdempotencyKey() {
    token.mint("alice", new ZetoMintParams(TRANSFERS), Optional.of("mint-1"));
    token.delegateLock(
        "alice",
        new ZetoDelegateLockParams(List.of(HexUint256.of(1)), DELEGATE),
        Optional.of("d-1"));

    final List<FakeRpcClient.Call> sends =
        rpc.calls().stream().filter(c -> c.method().equals("ptx_sendTransaction")).toList();
    assertEquals("mint-1", sends.get(0).body().get("idempotencyKey").asText());
    assertEquals("d-1", sends.get(1).body().get("idempotencyKey").asText());
  }

  @Test
  void readsBalance() {
    rpc.respond(
        "ptx_call", "{\"totalBalance\":\"0x64\",\"totalStates\":\"0x2\",\"overflow\":true}");

    final ZetoBalanceOfResult balance = token.balanceOf("alice", "bob").join();

    assertEquals(HexUint256.of(100), balance.totalBalance());
    assertEquals(HexUint256.of(2), balance.totalStates());
    assertTrue(balance.overflow());
    final JsonNode body = assertCall("balanceOf");
    assertEquals("bob", body.get("data").get("account").asText());
  }

  @Test
  void exposesAddressAndDomain() {
    final PaladinClient paladin = PaladinClient.wrap(rpc);
    final ZetoInstance defaulted = new ZetoInstance(paladin, ADDRESS);

    assertEquals(ADDRESS, defaulted.address());
    assertEquals(ZetoInstance.DEFAULT_DOMAIN, defaulted.domain());
    assertEquals("zeto-test", token.domain());
  }

  @Test
  void usingSubmitsThroughAnotherClient() {
    final FakeRpcClient other =
        new FakeRpcClient().respond("ptx_sendTransaction", "\"" + TX_ID + "\"");

    final ZetoInstance moved = token.using(PaladinClient.wrap(other));
    moved.withdraw("alice", new ZetoWithdrawParams(AMOUNT), Optional.empty());

    assertEquals(ADDRESS, moved.address());
    assertEquals("zeto-test", moved.domain());
    assertEquals(1, other.calls().size());
    assertTrue(rpc.calls().isEmpty());
  }

  @Test
  void rejectsMissingArguments() {
    final PaladinClient paladin = PaladinClient.wrap(rpc);
    assertThrows(NullPointerException.class, () -> new ZetoInstance(null, ADDRESS));
    assertThrows(NullPointerException.class, () -> new ZetoInstance(paladin, null, ADDRESS));
    assertThrows(NullPointerException.class, () -> new ZetoInstance(paladin, "zeto", null));
    assertThrows(NullPointerException.class, () -> token.mint("alice", null, Optional.empty()));
    assertThrows(NullPointerException.class, () -> token.balanceOf("alice", null));
    assertTrue(rpc.calls().isEmpty());
  }

  @Test
  void surfacesRpcFailures() {
    final FakeRpcClient failing = new FakeRpcClient();
    final ZetoInstance broken = token.using(PaladinClient.wrap(failing));

    final SentTransaction sent =
        broken.transfer("alice", new ZetoTransferParams(TRANSFERS), Optional.empty());

    assertSame(
        IllegalStateException.class,
        assertThrows(Exception.class, () -> sent.id().join()).getCause().getClass());
  }

  private JsonNode assertCall(final String function) {
    final JsonNode body = rpc.lastCall("ptx_call").body();
    assertEquals("private", body.get("type").asText());
    assertEquals("zeto-test", body.get("domain").asText());
    assertEquals(function, body.get("function").asText());
    assertEquals(ADDRESS.toString(), body.get("to").asText());
    assertEquals("alice", body.get("from").asText());
    return body;
  }

  private static boolean hasFunction(final JsonNode abi, final String function) {
    for (final JsonNode entry : abi) {
      if (function.equals(entry.path("name").asText())) {
        return true;
      }
    }
    return false;
  }
}
