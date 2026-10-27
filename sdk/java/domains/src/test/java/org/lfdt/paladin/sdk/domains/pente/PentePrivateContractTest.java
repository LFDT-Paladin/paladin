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
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNotSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.lfdt.paladin.sdk.domains.pente.PentePrivacyGroupTest.GET;
import static org.lfdt.paladin.sdk.domains.pente.PentePrivacyGroupTest.PRIVATE_ADDRESS;
import static org.lfdt.paladin.sdk.domains.pente.PentePrivacyGroupTest.SET;
import static org.lfdt.paladin.sdk.domains.pente.PentePrivacyGroupTest.TX_ID;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.CompletionException;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.exception.PaladinInvalidTransactionException;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.types.EthAddress;

class PentePrivateContractTest {

  private static final EthAddress ADDRESS = EthAddress.fromString(PRIVATE_ADDRESS);
  private static final AbiEntry SET_EVENT = AbiEntry.event("set").build();

  private FakeRpcClient rpc;
  private PentePrivateContract contract;

  @BeforeEach
  void setUp() {
    rpc =
        new FakeRpcClient()
            .respond("pgroup_sendTransaction", "\"" + TX_ID + "\"")
            .respond("pgroup_call", "[\"42\"]");
    contract =
        new PentePrivateContract(
            PentePrivacyGroupTest.deployedGroup(rpc), List.of(SET_EVENT, SET, GET), ADDRESS);
  }

  @Test
  void exposesTheContract() {
    assertEquals(ADDRESS, contract.address());
    assertEquals(List.of(SET_EVENT, SET, GET), contract.abi());
    assertEquals("pente", contract.group().domain());
  }

  @Test
  void sendsByFunctionName() {
    final SentTransaction sent = contract.sendTransaction("alice", "set", Map.of("x", 5), "k");

    assertEquals(UUID.fromString(TX_ID), sent.id().join());
    final JsonNode body = rpc.lastCall("pgroup_sendTransaction").body();
    assertEquals(PRIVATE_ADDRESS, body.get("to").asText());
    assertEquals("function", body.get("function").get("type").asText());
    assertEquals("set", body.get("function").get("name").asText());
    assertEquals(5, body.get("input").get("x").asInt());
    assertEquals("k", body.get("idempotencyKey").asText());
  }

  @Test
  void missingDataIsAnEmptyArgumentList() {
    contract.sendTransaction("alice", "set", null);

    final JsonNode input = rpc.lastCall("pgroup_sendTransaction").body().get("input");
    assertTrue(input.isArray() && input.isEmpty(), input.toString());
  }

  @Test
  void callsByFunctionName() {
    final JsonNode result = contract.call(null, "get", null).join();

    assertEquals("42", result.get(0).asText());
    assertEquals("get", rpc.lastCall("pgroup_call").body().get("function").get("name").asText());
  }

  @Test
  void unknownFunctionsFailWithoutAnRpc() {
    final SentTransaction sent = contract.sendTransaction("alice", "nope", null);
    final Throwable sendFailure =
        assertThrows(CompletionException.class, () -> sent.id().join()).getCause();
    final Throwable callFailure =
        assertThrows(CompletionException.class, () -> contract.call("alice", "nope", null).join())
            .getCause();

    assertInstanceOf(PaladinInvalidTransactionException.class, sendFailure);
    assertTrue(sendFailure.getMessage().contains("'nope'"), sendFailure.getMessage());
    assertInstanceOf(PaladinInvalidTransactionException.class, callFailure);
    assertTrue(rpc.calls().isEmpty());
  }

  @Test
  void usingSubmitsThroughAnotherClient() {
    final FakeRpcClient other =
        new FakeRpcClient().respond("pgroup_sendTransaction", "\"" + TX_ID + "\"");
    final PentePrivateContract moved = contract.using(PaladinClient.wrap(other));

    moved.sendTransaction("bob", "set", List.of(1));

    assertNotSame(contract, moved);
    assertEquals(ADDRESS, moved.address());
    assertEquals(1, other.calls().size());
    assertTrue(rpc.calls().isEmpty());
  }

  @Test
  void rejectsNullArguments() {
    final PentePrivacyGroup group = contract.group();
    assertThrows(
        NullPointerException.class, () -> new PentePrivateContract(null, List.of(), ADDRESS));
    assertThrows(NullPointerException.class, () -> new PentePrivateContract(group, null, ADDRESS));
    assertThrows(
        NullPointerException.class, () -> new PentePrivateContract(group, List.of(), null));
  }
}
