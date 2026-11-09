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
package org.lfdt.paladin.sdk.testing.containers;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.List;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.Network;
import org.testcontainers.utility.DockerImageName;

/** Checks container wiring and node configuration without starting Docker. */
class ContainerConfigurationTest {

  private static final String SEED = "ab".repeat(32);

  @Test
  void rendersABootableNodeConfig() {
    final PaladinNodeConfig config =
        PaladinNodeConfig.builder("node1")
            .seed(SEED)
            .blockchain("http://besu:8545", "ws://besu:8546")
            .logLevel("debug")
            .section("domains:\n  noto: {}\n")
            .build();
    final String yaml = config.toYaml();

    assertEquals("node1", config.nodeName());
    assertEquals(SEED, config.seed());
    assertTrue(yaml.startsWith("nodeName: node1\nlog:\n  level: debug\n"));
    assertTrue(yaml.contains("migrationsDir: /app/db/migrations/sqlite"));
    assertTrue(yaml.contains("port: 8548"));
    assertTrue(yaml.contains("port: 8549"));
    assertTrue(yaml.contains("url: http://besu:8545"));
    assertTrue(yaml.contains("url: ws://besu:8546"));
    assertTrue(yaml.contains("inline: " + SEED));
    assertTrue(yaml.endsWith("domains:\n  noto: {}\n"));
  }

  @Test
  void generatesADistinctSeedPerNode() {
    final PaladinNodeConfig a = PaladinNodeConfig.builder("a").blockchain("h", "w").build();
    final PaladinNodeConfig b = PaladinNodeConfig.builder("b").blockchain("h", "w").build();
    assertTrue(a.seed().matches("[0-9a-f]{64}"));
    assertNotEquals(a.seed(), b.seed());
  }

  @Test
  void rejectsInvalidInput() {
    assertThrows(IllegalArgumentException.class, () -> PaladinNodeConfig.builder(null));
    assertThrows(IllegalArgumentException.class, () -> PaladinNodeConfig.builder("bad name"));
    final PaladinNodeConfig.Builder builder = PaladinNodeConfig.builder("node1");
    assertThrows(IllegalArgumentException.class, () -> builder.seed("0x" + SEED));
    assertThrows(IllegalArgumentException.class, () -> builder.seed(null));
    assertThrows(NullPointerException.class, () -> builder.logLevel(null));
    assertThrows(NullPointerException.class, () -> builder.section(null));
    final IllegalArgumentException owned =
        assertThrows(IllegalArgumentException.class, () -> builder.section("db:\n  type: x"));
    assertTrue(owned.getMessage().contains("'db'"));
    assertThrows(NullPointerException.class, builder::build);
  }

  @Test
  void besuIsWiredForPaladin() {
    final BesuContainer besu = new BesuContainer();
    assertEquals(List.of(8545, 8546), besu.getExposedPorts());
    assertTrue(besu.getNetworkAliases().contains("besu"));
    assertTrue(List.of(besu.getCommandParts()).contains("--genesis-file=/var/besu/genesis.json"));
    assertEquals("http://besu:8545", besu.internalHttpUrl());
    assertEquals("ws://besu:8546", besu.internalWsUrl());

    try (Network network = Network.newNetwork()) {
      assertSame(network, besu.onNetwork(network).getNetwork());
    }
  }

  @Test
  void nodeJoinsBesuAndCarriesItsConfig() {
    final BesuContainer besu = new BesuContainer();
    final PaladinNodeContainer node =
        new PaladinNodeContainer(DockerImageName.parse("paladin:test"), "node2", besu)
            .withSeed(SEED)
            .withLogLevel("debug")
            .withConfigSection("transports: []");

    // Besu had no network, so the node gave it one and joined it.
    assertNotNull(besu.getNetwork());
    assertSame(besu.getNetwork(), node.getNetwork());
    assertEquals("node2", node.nodeName());
    assertTrue(node.getNetworkAliases().contains("node2"));
    assertEquals(List.of(8548, 8549), node.getExposedPorts());
    assertEquals(List.of(PaladinNodeConfig.CONFIG_PATH, "engine"), List.of(node.getCommandParts()));
    assertTrue(node.getDependencies().contains(besu));

    node.configure();
    final PaladinNodeConfig config = node.nodeConfig();
    assertSame(config, node.nodeConfig());
    assertEquals(SEED, config.seed());
    assertTrue(config.toYaml().contains("url: http://besu:8545"));
    assertTrue(config.toYaml().endsWith("transports: []\n"));
    besu.getNetwork().close();
  }

  @Test
  void nodeImageFollowsTheSystemProperty() {
    final String previous = System.getProperty(PaladinNodeContainer.IMAGE_PROPERTY);
    try {
      System.clearProperty(PaladinNodeContainer.IMAGE_PROPERTY);
      assertEquals(
          PaladinNodeContainer.DEFAULT_IMAGE,
          PaladinNodeContainer.defaultImage().asCanonicalNameString());
      System.setProperty(PaladinNodeContainer.IMAGE_PROPERTY, "paladin:local");
      assertEquals("paladin:local", PaladinNodeContainer.defaultImage().asCanonicalNameString());
      final PaladinNodeContainer node = new PaladinNodeContainer("node1", new BesuContainer());
      assertTrue(node.getNetworkAliases().contains("node1"));
    } finally {
      if (previous == null) {
        System.clearProperty(PaladinNodeContainer.IMAGE_PROPERTY);
      } else {
        System.setProperty(PaladinNodeContainer.IMAGE_PROPERTY, previous);
      }
    }
  }
}
