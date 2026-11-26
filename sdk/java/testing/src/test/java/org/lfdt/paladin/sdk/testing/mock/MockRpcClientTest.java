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
package org.lfdt.paladin.sdk.testing.mock;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.lfdt.paladin.sdk.testing.assertions.PaladinAssertions.assertRpcError;
import static org.lfdt.paladin.sdk.testing.assertions.PaladinAssertions.assertSuccess;

import com.fasterxml.jackson.core.type.TypeReference;
import java.time.Duration;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CompletionException;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.rpc.JsonRpcErrorCode;
import org.lfdt.paladin.sdk.core.transaction.TransactionInput;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.testing.fixtures.PaladinFixtures;

class MockRpcClientTest {

  private static final String CONTRACT = "0x0102030405060708090a0b0c0d0e0f1011121314";

  @Test
  void sendAndWaitThroughTheClientFacadeWithNoNode() {
    final UUID txId = UUID.randomUUID();
    final MockRpcClient rpc = new MockRpcClient();
    rpc.on("ptx_sendTransaction").thenReturn(txId);
    rpc.on("ptx_getTransactionReceipt", txId)
        .thenReturn(null)
        .thenReturn(PaladinFixtures.successReceipt(txId).build());

    try (PaladinClient client = rpc.paladinClient()) {
      final TransactionReceipt receipt =
          assertSuccess(
              client
                  .newTx()
                  .publicTx()
                  .from("alice")
                  .to(CONTRACT)
                  .function("transfer")
                  .pollingInterval(Duration.ofMillis(5))
                  .send(),
              Duration.ofSeconds(5));
      assertEquals(txId, receipt.id());
    }

    final TransactionInput sent =
        rpc.lastCall("ptx_sendTransaction").param(0, TransactionInput.class);
    assertEquals("alice", sent.from());
    assertEquals(2, rpc.calls("ptx_getTransactionReceipt").size());
    // A wrapped transport belongs to the caller: closing the facade leaves the mock open.
    assertFalse(rpc.isClosed());
    rpc.close();
    assertTrue(rpc.isClosed());
  }

  @Test
  void lastResponseRepeats() {
    final MockRpcClient rpc = new MockRpcClient();
    rpc.on("bidx_getConfirmedBlockHeight").thenReturn("0x1").thenReturn("0x2");
    assertEquals("0x1", rpc.callRpc(String.class, "bidx_getConfirmedBlockHeight").join());
    assertEquals("0x2", rpc.callRpc(String.class, "bidx_getConfirmedBlockHeight").join());
    assertEquals("0x2", rpc.callRpc(String.class, "bidx_getConfirmedBlockHeight").join());
  }

  @Test
  void parameterStubsOverrideCatchAllAndConvertTypes() {
    final MockRpcClient rpc = new MockRpcClient();
    rpc.on("ptx_resolveVerifier").thenReturn("0xdefault");
    rpc.on("ptx_resolveVerifier", "bob", "ecdsa:secp256k1", "eth_address").thenReturn("0xbob");
    rpc.onMatching("keymgr_resolveEthAddress", params -> params.get(0).asText().startsWith("x"))
        .thenReturn(CONTRACT);

    final PaladinClient client = rpc.paladinClient();
    assertEquals(
        "0xbob", client.ptx().resolveVerifier("bob", "ecdsa:secp256k1", "eth_address").join());
    assertEquals(
        "0xdefault",
        client.ptx().resolveVerifier("carol", "ecdsa:secp256k1", "eth_address").join());
    assertEquals(
        EthAddress.fromString(CONTRACT), client.keyManager().resolveEthAddress("xkey").join());
    assertRpcError(
        client.keyManager().resolveEthAddress("ykey"), JsonRpcErrorCode.METHOD_NOT_FOUND);
  }

  @Test
  void genericResultsUseTheRequestedType() {
    final MockRpcClient rpc = new MockRpcClient();
    final UUID first = UUID.randomUUID();
    rpc.on("ptx_sendTransactions").thenReturn(List.of(first.toString()));
    final List<UUID> ids =
        rpc.callRpc(new TypeReference<List<UUID>>() {}, "ptx_sendTransactions", List.of()).join();
    assertEquals(List.of(first), ids);
  }

  @Test
  void failuresAndAnswers() {
    final MockRpcClient rpc = new MockRpcClient();
    rpc.on("ptx_getTransaction").thenFail(JsonRpcErrorCode.INVALID_PARAMS, "bad id");
    rpc.on("echo").thenAnswer(call -> call.param(0).asText() + "!");
    rpc.on("boom")
        .thenAnswer(
            call -> {
              throw new IllegalStateException("exploded");
            });
    rpc.on("silent");

    final PaladinRpcException error =
        assertRpcError(
            rpc.callRpc(String.class, "ptx_getTransaction", "x"), JsonRpcErrorCode.INVALID_PARAMS);
    assertEquals("bad id", error.getMessage());
    assertEquals("hi!", rpc.callRpc(String.class, "echo", "hi").join());
    final CompletionException boom =
        assertThrows(CompletionException.class, () -> rpc.callRpc(String.class, "boom").join());
    assertTrue(boom.getCause() instanceof IllegalStateException);
    assertRpcError(rpc.callRpc(String.class, "silent"), JsonRpcErrorCode.INTERNAL_ERROR);
  }

  @Test
  void recordsCallsWithNullParametersAndResets() {
    final MockRpcClient rpc = new MockRpcClient();
    rpc.on("ptx_getTransaction").thenReturn(null);
    assertNull(rpc.callRpc(String.class, "ptx_getTransaction", (Object) null).join());
    assertNull(rpc.callRpc(String.class, "ptx_getTransaction", (Object[]) null).join());

    assertEquals(2, rpc.calls().size());
    assertTrue(rpc.calls().get(0).param(0).isNull());
    assertTrue(rpc.calls().get(1).params().isEmpty());

    rpc.reset();
    assertTrue(rpc.calls().isEmpty());
    assertRpcError(rpc.callRpc(String.class, "ptx_getTransaction"), -32601);
    final AssertionError missing =
        assertThrows(AssertionError.class, () -> rpc.lastCall("ptx_sendTransaction"));
    assertTrue(missing.getMessage().contains("ptx_getTransaction"));
    assertFalse(rpc.isClosed());
  }

  @Test
  void rejectsNullArguments() {
    final MockRpcClient rpc = new MockRpcClient();
    assertThrows(NullPointerException.class, () -> rpc.on(null));
    assertThrows(NullPointerException.class, () -> rpc.onMatching("m", null));
    assertThrows(NullPointerException.class, () -> rpc.on("m").thenAnswer(null));
    // A null varargs array matches a call with no parameters.
    rpc.on("m", (Object[]) null).thenReturn("none");
    assertEquals("none", rpc.callRpc(String.class, "m").join());
  }
}
