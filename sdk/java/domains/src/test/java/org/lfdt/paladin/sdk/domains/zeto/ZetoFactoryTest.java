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
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import java.time.Duration;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.CompletionException;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.exception.PaladinException;
import org.lfdt.paladin.sdk.core.types.EthAddress;

class ZetoFactoryTest {

  private static final String TX_ID = "00000000-0000-0000-0000-0000000000bb";
  private static final String CONTRACT = "0x0102030405060708090a0b0c0d0e0f1011121314";

  private FakeRpcClient rpc;
  private ZetoFactory factory;

  @BeforeEach
  void setUp() {
    rpc = new FakeRpcClient().respond("ptx_sendTransaction", "\"" + TX_ID + "\"");
    factory = new ZetoFactory(PaladinClient.wrap(rpc), "zeto");
  }

  private static String receipt(final boolean success) {
    return "{\"id\":\""
        + TX_ID
        + "\",\"sequence\":1,\"success\":"
        + success
        + (success ? ",\"contractAddress\":\"" + CONTRACT + "\"" : ",\"failureMessage\":\"boom\"")
        + "}";
  }

  @Test
  void deploysAToken() {
    final ZetoDeployment deployment =
        factory.newZeto("alice", new ZetoConstructorParams("Zeto_Anon"), Optional.empty());

    assertEquals(UUID.fromString(TX_ID), deployment.id().join());
    assertSame(deployment.id(), deployment.transaction().id());
    final JsonNode body = rpc.lastCall("ptx_sendTransaction").body();
    assertEquals("private", body.get("type").asText());
    assertEquals("zeto", body.get("domain").asText());
    assertEquals("alice", body.get("from").asText());
    assertFalse(body.has("function"));
    assertFalse(body.has("to"));
    assertFalse(body.has("idempotencyKey"));

    final JsonNode constructor = body.get("abi").get(0);
    assertEquals("constructor", constructor.get("type").asText());
    assertEquals("tokenName", constructor.get("inputs").get(0).get("name").asText());
    assertEquals("Zeto_Anon", body.get("data").get("tokenName").asText());
  }

  @Test
  void forwardsAnIdempotencyKey() {
    factory.newZeto("alice", new ZetoConstructorParams("Zeto_Anon"), Optional.of("deploy-1"));

    assertEquals(
        "deploy-1", rpc.lastCall("ptx_sendTransaction").body().get("idempotencyKey").asText());
  }

  @Test
  void waitForDeployReturnsTheToken() {
    rpc.respond("ptx_getTransactionReceipt", receipt(true));

    final ZetoInstance token =
        factory
            .newZeto("alice", new ZetoConstructorParams("Zeto_Anon"), Optional.empty())
            .waitForDeploy()
            .join();

    assertEquals(EthAddress.fromString(CONTRACT), token.address());
    assertEquals("zeto", token.domain());
    assertEquals(TX_ID, rpc.lastCall("ptx_getTransactionReceipt").params().get(0).asText());
  }

  @Test
  void waitForDeployWithATimeoutReturnsTheToken() {
    rpc.respond("ptx_getTransactionReceipt", receipt(true));

    final ZetoInstance token =
        factory
            .newZeto("alice", new ZetoConstructorParams("Zeto_Anon"), Optional.empty())
            .waitForDeploy(Duration.ofSeconds(5))
            .join();

    assertEquals(EthAddress.fromString(CONTRACT), token.address());
  }

  @Test
  void waitForDeployFailsWhenTheDeploymentFails() {
    rpc.respond("ptx_getTransactionReceipt", receipt(false));
    final ZetoDeployment deployment =
        factory.newZeto("alice", new ZetoConstructorParams("Zeto_Anon"), Optional.empty());

    final CompletionException failure =
        assertThrows(CompletionException.class, () -> deployment.waitForDeploy().join());

    final PaladinException cause = assertInstanceOf(PaladinException.class, failure.getCause());
    assertTrue(cause.getMessage().contains(TX_ID), cause.getMessage());
    assertTrue(cause.getMessage().contains("boom"), cause.getMessage());
  }

  @Test
  void usingDeploysThroughAnotherClient() {
    final FakeRpcClient other =
        new FakeRpcClient().respond("ptx_sendTransaction", "\"" + TX_ID + "\"");

    final ZetoFactory moved = factory.using(PaladinClient.wrap(other));
    moved.newZeto("alice", new ZetoConstructorParams("Zeto_Anon"), Optional.empty());

    assertEquals("zeto", moved.domain());
    assertEquals(1, other.calls().size());
    assertTrue(rpc.calls().isEmpty());
  }

  @Test
  void rejectsMissingArguments() {
    final PaladinClient paladin = PaladinClient.wrap(rpc);
    assertThrows(NullPointerException.class, () -> new ZetoFactory(null, "zeto"));
    assertThrows(NullPointerException.class, () -> new ZetoFactory(paladin, null));
    assertThrows(
        NullPointerException.class, () -> factory.newZeto("alice", null, Optional.empty()));
    assertTrue(rpc.calls().isEmpty());
  }
}
