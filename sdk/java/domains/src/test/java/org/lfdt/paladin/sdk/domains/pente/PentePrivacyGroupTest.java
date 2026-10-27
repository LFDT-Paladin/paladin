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
package org.lfdt.paladin.sdk.domains.pente;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNotSame;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import java.time.Duration;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.CompletionException;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.exception.PaladinException;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.AbiParameter;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroup;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

class PentePrivacyGroupTest {

  static final String GROUP_ID = "0x1234";
  static final String GROUP_ADDRESS = "0x0102030405060708090a0b0c0d0e0f1011121314";
  static final String PRIVATE_ADDRESS = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
  static final String TX_ID = "00000000-0000-0000-0000-0000000000dd";

  static final AbiEntry CONSTRUCTOR =
      AbiEntry.constructor().input(AbiParameter.builder("initial", "uint256").build()).build();
  static final AbiEntry SET =
      AbiEntry.function("set").input(AbiParameter.builder("x", "uint256").build()).build();
  static final AbiEntry GET =
      AbiEntry.function("get").output(AbiParameter.builder("x", "uint256").build()).build();

  private FakeRpcClient rpc;
  private PentePrivacyGroup group;

  static PrivacyGroup privacyGroup(final boolean deployed) {
    final Map<String, Object> json = new HashMap<>();
    json.put("id", GROUP_ID);
    json.put("domain", "pente");
    json.put("members", List.of("alice", "bob"));
    json.put("genesisTransaction", TX_ID);
    if (deployed) {
      json.put("contractAddress", GROUP_ADDRESS);
    }
    return PaladinObjectMapper.shared().convertValue(json, PrivacyGroup.class);
  }

  static PentePrivacyGroup deployedGroup(final FakeRpcClient rpc) {
    return new PentePrivacyGroup(PaladinClient.wrap(rpc), privacyGroup(true));
  }

  @BeforeEach
  void setUp() {
    rpc =
        new FakeRpcClient()
            .respond("pgroup_sendTransaction", "\"" + TX_ID + "\"")
            .respond("pgroup_call", "{\"x\":\"42\"}");
    group = deployedGroup(rpc);
  }

  private void receipts(final boolean success, final String domainReceipt) {
    rpc.respond(
        "ptx_getTransactionReceipt",
        "{\"id\":\""
            + TX_ID
            + "\",\"sequence\":1,\"success\":"
            + success
            + (success ? "" : ",\"failureMessage\":\"reverted\"")
            + "}");
    rpc.respond(
        "ptx_getTransactionReceiptFull",
        "{\"id\":\""
            + TX_ID
            + "\",\"sequence\":1,\"success\":true,\"domainReceipt\":"
            + domainReceipt
            + "}");
  }

  private static Throwable failure(final Runnable join) {
    return assertThrows(CompletionException.class, join::run).getCause();
  }

  @Test
  void exposesTheGroup() {
    assertEquals(HexBytes.fromString(GROUP_ID), group.id());
    assertEquals(EthAddress.fromString(GROUP_ADDRESS), group.address());
    assertEquals("pente", group.domain());
    assertEquals(List.of("alice", "bob"), group.members());
    assertEquals(privacyGroup(true), group.group());
  }

  @Test
  void rejectsAGroupWithoutAContractAddress() {
    final PaladinClient paladin = PaladinClient.wrap(rpc);
    final IllegalArgumentException e =
        assertThrows(
            IllegalArgumentException.class,
            () -> new PentePrivacyGroup(paladin, privacyGroup(false)));
    assertTrue(e.getMessage().contains(TX_ID), e.getMessage());
    assertThrows(NullPointerException.class, () -> new PentePrivacyGroup(paladin, null));
  }

  @Test
  void deploysThroughTheGroup() {
    final PentePrivateDeployment deployment =
        group.deploy(
            new PenteDeployParams(
                List.of(SET, CONSTRUCTOR),
                HexBytes.fromString("0x6000"),
                "alice",
                Map.of("initial", 7)),
            "deploy-1");

    assertEquals(UUID.fromString(TX_ID), deployment.id().join());
    assertSame(deployment.id(), deployment.transaction().id());
    final JsonNode body = rpc.lastCall("pgroup_sendTransaction").body();
    assertEquals("pente", body.get("domain").asText());
    assertEquals(GROUP_ID, body.get("group").asText());
    assertEquals("alice", body.get("from").asText());
    assertEquals("0x6000", body.get("bytecode").asText());
    assertEquals("deploy-1", body.get("idempotencyKey").asText());
    assertEquals("constructor", body.get("function").get("type").asText());
    assertEquals("initial", body.get("function").get("inputs").get(0).get("name").asText());
    assertEquals(7, body.get("input").get("initial").asInt());
    assertFalse(body.has("to"));
  }

  @Test
  void deployDefaultsToAnEmptyConstructor() {
    group.deploy(new PenteDeployParams(null, HexBytes.fromString("0x6000"), "alice", null));

    final JsonNode body = rpc.lastCall("pgroup_sendTransaction").body();
    assertEquals("constructor", body.get("function").get("type").asText());
    assertFalse(body.has("idempotencyKey"));
    assertFalse(body.has("input"));
  }

  @Test
  void waitForDeployReadsTheDomainReceipt() {
    receipts(true, "{\"receipt\":{\"contractAddress\":\"" + PRIVATE_ADDRESS + "\"}}");
    final PentePrivateDeployment deployment =
        group.deploy(new PenteDeployParams(List.of(), HexBytes.fromString("0x6000"), "a", null));

    assertEquals(EthAddress.fromString(PRIVATE_ADDRESS), deployment.waitForDeploy().join());
    assertEquals(
        EthAddress.fromString(PRIVATE_ADDRESS),
        deployment.waitForDeploy(Duration.ofSeconds(5)).join());
    assertEquals(TX_ID, rpc.lastCall("ptx_getTransactionReceiptFull").body().asText());
  }

  @Test
  void waitForDeployFailsForAFailedDeployment() {
    receipts(false, "null");
    final PentePrivateDeployment deployment =
        group.deploy(new PenteDeployParams(List.of(), HexBytes.fromString("0x6000"), "a", null));

    final Throwable cause = failure(() -> deployment.waitForDeploy().join());

    assertInstanceOf(PaladinException.class, cause);
    assertTrue(cause.getMessage().contains("reverted"), cause.getMessage());
  }

  @Test
  void waitForDeployFailsWithoutAContractAddress() {
    for (final String domainReceipt :
        List.of("null", "{}", "{\"receipt\":{}}", "{\"receipt\":{\"contractAddress\":1}}")) {
      receipts(true, domainReceipt);
      final PentePrivateDeployment deployment =
          group.deploy(new PenteDeployParams(List.of(), HexBytes.fromString("0x6000"), "a", null));

      final Throwable cause = failure(() -> deployment.waitForDeploy().join());

      assertInstanceOf(PaladinException.class, cause, domainReceipt);
      assertTrue(cause.getMessage().contains("no contract address"), cause.getMessage());
    }
  }

  @Test
  void sendsATransactionToAPrivateContract() {
    group.sendTransaction(
        new PenteGroupTransactionParams(
            "alice", EthAddress.fromString(PRIVATE_ADDRESS), SET, Map.of("x", 1)),
        "tx-1");

    final JsonNode body = rpc.lastCall("pgroup_sendTransaction").body();
    assertEquals("alice", body.get("from").asText());
    assertEquals(PRIVATE_ADDRESS, body.get("to").asText());
    assertEquals("set", body.get("function").get("name").asText());
    assertEquals(1, body.get("input").get("x").asInt());
    assertEquals("tx-1", body.get("idempotencyKey").asText());
  }

  @Test
  void sendTransactionWithoutDataOrKey() {
    assertEquals(
        UUID.fromString(TX_ID),
        group
            .sendTransaction(
                new PenteGroupTransactionParams(
                    "alice", EthAddress.fromString(PRIVATE_ADDRESS), SET, null))
            .id()
            .join());

    final JsonNode body = rpc.lastCall("pgroup_sendTransaction").body();
    assertFalse(body.has("input"));
    assertFalse(body.has("idempotencyKey"));
  }

  @Test
  void callsAPrivateContract() {
    final JsonNode result =
        group
            .call(
                new PenteGroupTransactionParams(
                    "bob", EthAddress.fromString(PRIVATE_ADDRESS), GET, List.of()))
            .join();

    assertEquals("42", result.get("x").asText());
    final JsonNode body = rpc.lastCall("pgroup_call").body();
    assertEquals("bob", body.get("from").asText());
    assertEquals("get", body.get("function").get("name").asText());
    assertEquals(GROUP_ID, body.get("group").asText());
  }

  @Test
  void callsWithoutAnIdentityUseTheDefault() {
    for (final String from : new String[] {null, " "}) {
      group
          .call(
              new PenteGroupTransactionParams(
                  from, EthAddress.fromString(PRIVATE_ADDRESS), GET, null))
          .join();

      assertEquals(
          PentePrivacyGroup.DEFAULT_CALL_FROM,
          rpc.lastCall("pgroup_call").body().get("from").asText());
    }
  }

  @Test
  void usingSubmitsThroughAnotherClient() {
    final FakeRpcClient other =
        new FakeRpcClient().respond("pgroup_sendTransaction", "\"" + TX_ID + "\"");
    final PentePrivacyGroup moved = group.using(PaladinClient.wrap(other));

    moved.sendTransaction(
        new PenteGroupTransactionParams("bob", EthAddress.fromString(PRIVATE_ADDRESS), SET, null));

    assertNotSame(group, moved);
    assertEquals(group.address(), moved.address());
    assertEquals(1, other.calls().size());
    assertTrue(rpc.calls().isEmpty());
  }

  @Test
  void rejectsNullParams() {
    assertThrows(NullPointerException.class, () -> group.deploy(null));
    assertThrows(NullPointerException.class, () -> group.sendTransaction(null));
    assertThrows(NullPointerException.class, () -> group.call(null));
  }
}
