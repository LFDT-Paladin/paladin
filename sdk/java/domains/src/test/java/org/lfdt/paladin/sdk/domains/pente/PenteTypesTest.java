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
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupInput;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

class PenteTypesTest {

  @Test
  void groupParamsDefaultAndCopy() {
    final List<String> members = new ArrayList<>(List.of("alice"));
    final PentePrivacyGroupParams params = PentePrivacyGroupParams.of(members);
    members.add("bob");

    assertEquals(List.of("alice"), params.members());
    assertEquals(Map.of(), params.properties());
    assertNull(params.name());
    assertNull(params.evmVersion());
    assertNull(params.endorsementType());
    assertNull(params.externalCallsEnabled());
    assertThrows(
        NullPointerException.class,
        () -> new PentePrivacyGroupParams(null, null, null, null, null, null));
  }

  @Test
  void groupParamsWithersKeepOtherFields() {
    final PentePrivacyGroupParams params =
        PentePrivacyGroupParams.of("alice")
            .withProperty("a", "1")
            .withName("pg")
            .withEvmVersion("london")
            .withEndorsementType("group_scoped_identities")
            .withExternalCallsEnabled(false)
            .withProperty("b", "2");

    assertEquals(
        new PentePrivacyGroupParams(
            "pg",
            List.of("alice"),
            "london",
            "group_scoped_identities",
            false,
            Map.of("a", "1", "b", "2")),
        params);
  }

  @Test
  void groupParamsBuildTheCreateRequest() {
    final PrivacyGroupInput input =
        PentePrivacyGroupParams.of("alice", "bob")
            .withName("pg")
            .withEvmVersion("paris")
            .withProperty("k", "v")
            .toInput("pente");

    assertEquals("pente", input.domain());
    assertEquals("pg", input.name());
    assertEquals(List.of("alice", "bob"), input.members());
    assertEquals(Map.of("evmVersion", "paris"), input.configuration());
    assertEquals(Map.of("k", "v"), input.properties());
  }

  @Test
  void deployParamsDefaultTheAbiAndRequireBytecode() {
    final PenteDeployParams params =
        new PenteDeployParams(null, HexBytes.fromString("0x60"), "alice", null);

    assertTrue(params.abi().isEmpty());
    assertThrows(
        NullPointerException.class, () -> new PenteDeployParams(List.of(), null, "alice", null));
  }

  @Test
  void groupTransactionParamsRequireTargetAndMethod() {
    final EthAddress to = EthAddress.fromString("0x0102030405060708090a0b0c0d0e0f1011121314");
    assertThrows(
        NullPointerException.class,
        () -> new PenteGroupTransactionParams("a", null, PentePrivacyGroupTest.SET, null));
    assertThrows(
        NullPointerException.class, () -> new PenteGroupTransactionParams("a", to, null, null));
  }
}
