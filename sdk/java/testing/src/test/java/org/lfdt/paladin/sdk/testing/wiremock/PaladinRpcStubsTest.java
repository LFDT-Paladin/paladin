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
package org.lfdt.paladin.sdk.testing.wiremock;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.lfdt.paladin.sdk.testing.assertions.PaladinAssertions.assertRpcError;
import static org.lfdt.paladin.sdk.testing.assertions.PaladinAssertions.assertSuccess;
import static org.lfdt.paladin.sdk.testing.wiremock.PaladinRpcStubs.called;
import static org.lfdt.paladin.sdk.testing.wiremock.PaladinRpcStubs.error;
import static org.lfdt.paladin.sdk.testing.wiremock.PaladinRpcStubs.result;
import static org.lfdt.paladin.sdk.testing.wiremock.PaladinRpcStubs.rpc;

import com.fasterxml.jackson.databind.JsonNode;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.rpc.JsonRpcErrorCode;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.testing.fixtures.PaladinFixtures;

class PaladinRpcStubsTest {

  private static final String CONTRACT = "0x0102030405060708090a0b0c0d0e0f1011121314";

  private PaladinWireMock node;
  private PaladinClient client;

  @BeforeEach
  void start() {
    node = PaladinWireMock.start();
    client = node.paladinClient();
  }

  @AfterEach
  void stop() {
    client.close();
    node.close();
  }

  @Test
  void sendAndWaitHappyPathOverHttp() {
    final UUID txId = UUID.randomUUID();
    final EthAddress deployed = PaladinFixtures.randomAddress();
    node.stub(PaladinRpcStubs.sendTransaction(txId))
        .stub(
            PaladinRpcStubs.transactionReceipt(
                PaladinFixtures.deployReceipt(txId, deployed).build()));

    final TransactionReceipt receipt =
        assertSuccess(
            client.newTx().publicTx().from("alice").to(CONTRACT).function("transfer").send(),
            Duration.ofSeconds(5));

    assertEquals(deployed, receipt.contractAddress());
    assertEquals(1, node.callCount("ptx_sendTransaction"));
    node.server().verify(1, called("ptx_getTransactionReceipt", txId));
  }

  @Test
  void pendingReceiptIsNull() {
    final UUID txId = UUID.randomUUID();
    node.stub(PaladinRpcStubs.receiptPending(txId));
    assertNull(client.ptx().getTransactionReceipt(txId).join());
  }

  @Test
  void namedHappyPathStubs() {
    node.stub(
            PaladinRpcStubs.resolveVerifier("bob@node1", "ecdsa:secp256k1", "eth_address", "0xb0b"))
        .stub(PaladinRpcStubs.call(List.of("42")))
        .stub(PaladinRpcStubs.confirmedBlockHeight(255));

    assertEquals(
        "0xb0b",
        client.ptx().resolveVerifier("bob@node1", "ecdsa:secp256k1", "eth_address").join());
    final JsonNode output =
        client.newTx().publicTx().from("alice").to(CONTRACT).function("balanceOf").call().join();
    assertEquals("42", output.get(0).asText());
    assertEquals(255L, client.blockIndex().getConfirmedBlockHeight().join().asUnsignedLong());
  }

  @Test
  void errorStubSurfacesAsRpcException() {
    node.stub(rpc("ptx_getTransaction").willReturn(error(JsonRpcErrorCode.INVALID_PARAMS, "nope")));
    final PaladinRpcException e =
        assertRpcError(
            client.ptx().getTransaction(UUID.randomUUID()), JsonRpcErrorCode.INVALID_PARAMS);
    assertEquals("nope", e.getMessage());
    assertEquals(200, e.httpStatus());
  }

  @Test
  void unmatchedCallFailsFast() {
    node.stub(PaladinRpcStubs.resolveVerifier("bob", "a", "b", "0xb0b"));
    // WireMock answers an unmatched request with a non-JSON 404, which the client maps to code 0.
    final PaladinRpcException e =
        assertRpcError(client.ptx().resolveVerifier("carol", "a", "b"), 0);
    assertEquals(404, e.httpStatus());
    node.reset();
    assertEquals(0, node.callCount("ptx_resolveVerifier"));
  }

  @Test
  void responseEchoesTheRequestId() throws Exception {
    node.stub(rpc("anything").willReturn(result(Map.of("ok", true))));
    try (HttpClient http = HttpClient.newHttpClient()) {
      final HttpResponse<String> response =
          http.send(
              HttpRequest.newBuilder(URI.create(node.url()))
                  .POST(
                      HttpRequest.BodyPublishers.ofString(
                          "{\"jsonrpc\":\"2.0\",\"id\":\"abc-7\",\"method\":\"anything\"}"))
                  .build(),
              HttpResponse.BodyHandlers.ofString());
      final JsonNode body = PaladinObjectMapper.shared().readTree(response.body());
      assertEquals("abc-7", body.get("id").asText());
      assertTrue(body.get("result").get("ok").asBoolean());
      assertEquals("application/json", response.headers().firstValue("Content-Type").get());
    }
  }

  @Test
  void unserializableParametersAreRejected() {
    final Object selfReferencing =
        new Object() {
          @SuppressWarnings("unused")
          public Object getSelf() {
            return this;
          }
        };
    assertThrows(IllegalArgumentException.class, () -> rpc("m", selfReferencing));
    assertTrue(node.url().startsWith("http://"));
  }
}
