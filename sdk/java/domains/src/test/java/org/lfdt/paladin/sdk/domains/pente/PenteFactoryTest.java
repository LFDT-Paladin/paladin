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
import java.util.List;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.CompletionException;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.exception.PaladinException;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

class PenteFactoryTest {

  private static final String GROUP_ID = "0x1234";
  private static final String TX_ID = "00000000-0000-0000-0000-0000000000cc";
  private static final String CONTRACT = "0x0102030405060708090a0b0c0d0e0f1011121314";

  private FakeRpcClient rpc;
  private PenteFactory factory;

  @BeforeEach
  void setUp() {
    rpc = new FakeRpcClient().respond("pgroup_createGroup", group(true, false));
    factory = new PenteFactory(PaladinClient.wrap(rpc), "pente");
  }

  private static String group(final boolean genesis, final boolean deployed) {
    return "{\"id\":\""
        + GROUP_ID
        + "\",\"domain\":\"pente\",\"members\":[\"alice\",\"bob\"]"
        + (genesis ? ",\"genesisTransaction\":\"" + TX_ID + "\"" : "")
        + (deployed ? ",\"contractAddress\":\"" + CONTRACT + "\"" : "")
        + "}";
  }

  private static String receipt(final boolean success) {
    return "{\"id\":\""
        + TX_ID
        + "\",\"sequence\":1,\"success\":"
        + success
        + (success ? ",\"contractAddress\":\"" + CONTRACT + "\"" : ",\"failureMessage\":\"boom\"")
        + "}";
  }

  private static Throwable failure(final Runnable join) {
    return assertThrows(CompletionException.class, join::run).getCause();
  }

  @Test
  void createsAGroupWithTheDomainDefaults() {
    final PentePrivacyGroupDeployment deployment =
        factory.newPrivacyGroup(PentePrivacyGroupParams.of("alice", "bob"));

    assertEquals(HexBytes.fromString(GROUP_ID), deployment.group().join().id());
    assertEquals(UUID.fromString(TX_ID), deployment.id().join());
    assertSame(deployment.id(), deployment.transaction().id());
    final JsonNode body = rpc.lastCall("pgroup_createGroup").body();
    assertEquals("pente", body.get("domain").asText());
    assertEquals("bob", body.get("members").get(1).asText());
    assertFalse(body.has("name"));
    assertFalse(body.has("configuration"));
    assertFalse(body.has("properties"));
  }

  @Test
  void createsAGroupWithExplicitConfiguration() {
    factory.newPrivacyGroup(
        PentePrivacyGroupParams.of(List.of("alice"))
            .withName("pg1")
            .withEvmVersion("shanghai")
            .withEndorsementType("group_scoped_identities")
            .withExternalCallsEnabled(true)
            .withProperty("team", "blue"));

    final JsonNode body = rpc.lastCall("pgroup_createGroup").body();
    assertEquals("pg1", body.get("name").asText());
    final JsonNode config = body.get("configuration");
    assertEquals("shanghai", config.get("evmVersion").asText());
    assertEquals("group_scoped_identities", config.get("endorsementType").asText());
    assertEquals("true", config.get("externalCallsEnabled").asText());
    assertEquals("blue", body.get("properties").get("team").asText());
  }

  @Test
  void disabledExternalCallsAreSentExplicitly() {
    factory.newPrivacyGroup(PentePrivacyGroupParams.of("alice").withExternalCallsEnabled(false));

    final JsonNode config = rpc.lastCall("pgroup_createGroup").body().get("configuration");
    assertEquals("false", config.get("externalCallsEnabled").asText());
    assertFalse(config.has("evmVersion"));
  }

  @Test
  void waitForDeployReturnsTheDeployedGroup() {
    rpc.respond("ptx_getTransactionReceipt", receipt(true));

    final PentePrivacyGroup group =
        factory.newPrivacyGroup(PentePrivacyGroupParams.of("alice")).waitForDeploy().join();

    assertEquals(EthAddress.fromString(CONTRACT), group.address());
    assertEquals(HexBytes.fromString(GROUP_ID), group.id());
    assertEquals("pente", group.domain());
    assertEquals(List.of("alice", "bob"), group.members());
    assertEquals(TX_ID, rpc.lastCall("ptx_getTransactionReceipt").body().asText());
  }

  @Test
  void waitForDeployAcceptsATimeout() {
    rpc.respond("ptx_getTransactionReceipt", receipt(true));

    final PentePrivacyGroup group =
        factory
            .newPrivacyGroup(PentePrivacyGroupParams.of("alice"))
            .waitForDeploy(Duration.ofSeconds(5))
            .join();

    assertEquals(EthAddress.fromString(CONTRACT), group.address());
  }

  @Test
  void waitForDeployFailsWhenTheGenesisTransactionFails() {
    rpc.respond("ptx_getTransactionReceipt", receipt(false));
    final PentePrivacyGroupDeployment deployment =
        factory.newPrivacyGroup(PentePrivacyGroupParams.of("alice"));

    final Throwable cause = failure(() -> deployment.waitForDeploy().join());

    assertInstanceOf(PaladinException.class, cause);
    assertTrue(cause.getMessage().contains("boom"), cause.getMessage());
  }

  @Test
  void waitForDeployFailsWithoutAGenesisTransaction() {
    rpc.respond("pgroup_createGroup", group(false, false));
    final PentePrivacyGroupDeployment deployment =
        factory.newPrivacyGroup(PentePrivacyGroupParams.of("alice"));

    final Throwable cause = failure(() -> deployment.waitForDeploy().join());

    assertInstanceOf(PaladinException.class, cause);
    assertTrue(cause.getMessage().contains("no genesis transaction"), cause.getMessage());
  }

  @Test
  void resumesADeployedGroup() {
    rpc.respond("pgroup_getGroupById", group(true, true));

    final Optional<PentePrivacyGroup> group = factory.resumePrivacyGroup(GROUP_ID).join();

    assertTrue(group.isPresent());
    assertEquals(EthAddress.fromString(CONTRACT), group.get().address());
    final JsonNode params = rpc.lastCall("pgroup_getGroupById").params();
    assertEquals("pente", params.get(0).asText());
    assertEquals(GROUP_ID, params.get(1).asText());
  }

  @Test
  void resumeIsEmptyForAnUndeployedOrMissingGroup() {
    rpc.respond("pgroup_getGroupById", group(true, false));
    assertTrue(factory.resumePrivacyGroup(HexBytes.fromString(GROUP_ID)).join().isEmpty());

    rpc.respond("pgroup_getGroupById", "null");
    assertTrue(factory.resumePrivacyGroup(GROUP_ID).join().isEmpty());
  }

  @Test
  void usingKeepsTheDomain() {
    final PenteFactory other = factory.using(PaladinClient.wrap(new FakeRpcClient()));

    assertNotSame(factory, other);
    assertEquals("pente", other.domain());
  }

  @Test
  void rejectsNullArguments() {
    final PaladinClient paladin = PaladinClient.wrap(rpc);
    assertThrows(NullPointerException.class, () -> new PenteFactory(null, "pente"));
    assertThrows(NullPointerException.class, () -> new PenteFactory(paladin, null));
    assertThrows(NullPointerException.class, () -> factory.newPrivacyGroup(null));
  }
}
