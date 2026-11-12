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

import java.time.Duration;
import java.util.Objects;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.containers.Network;
import org.testcontainers.images.builder.Transferable;
import org.testcontainers.utility.DockerImageName;

/**
 * One Paladin node in a container, attached to a {@link BesuContainer} on the same network.
 *
 * <p>The node boots on in-memory SQLite with a single HD wallet (see {@link PaladinNodeConfig}),
 * and counts as started once its JSON-RPC server answers. Extra configuration — domains,
 * transports, registries — goes in through {@link #withConfigSection(String)} before start.
 *
 * <p>Most tests should not build these by hand: {@link PaladinTestNetwork} wires Besu, the network
 * and any number of nodes together.
 */
public class PaladinNodeContainer extends GenericContainer<PaladinNodeContainer> {

  /** System property that overrides {@link #DEFAULT_IMAGE}, e.g. {@code paladin:local}. */
  public static final String IMAGE_PROPERTY = "paladin.image";

  /** The published Paladin image used unless {@link #IMAGE_PROPERTY} or a constructor says not. */
  public static final String DEFAULT_IMAGE = "ghcr.io/lfdt-paladin/paladin:main";

  private final PaladinNodeConfig.Builder config;
  private final String nodeName;
  private PaladinNodeConfig rendered;

  /**
   * Creates a node from {@link #defaultImage()}.
   *
   * @param nodeName the node name, also its alias on the network
   * @param besu the base ledger; the node joins its network and starts after it
   */
  public PaladinNodeContainer(final String nodeName, final BesuContainer besu) {
    this(defaultImage(), nodeName, besu);
  }

  /**
   * Creates a node from a specific Paladin image.
   *
   * @param image the image
   * @param nodeName the node name, also its alias on the network
   * @param besu the base ledger; the node joins its network and starts after it
   */
  public PaladinNodeContainer(
      final DockerImageName image, final String nodeName, final BesuContainer besu) {
    super(image);
    Objects.requireNonNull(besu, "besu");
    if (besu.getNetwork() == null) {
      // The node finds Besu by its network alias, so the two must share a network.
      besu.withNetwork(Network.newNetwork());
    }
    this.config =
        PaladinNodeConfig.builder(nodeName)
            .blockchain(besu.internalHttpUrl(), besu.internalWsUrl());
    this.nodeName = nodeName;
    withNetwork(besu.getNetwork());
    withNetworkAliases(nodeName);
    dependsOn(besu);
    withExposedPorts(PaladinNodeConfig.RPC_HTTP_PORT, PaladinNodeConfig.RPC_WS_PORT);
    withCommand(PaladinNodeConfig.CONFIG_PATH, "engine");
    waitingFor(
        new JsonRpcWaitStrategy(PaladinNodeConfig.RPC_HTTP_PORT, "keymgr_wallets")
            .withStartupTimeout(Duration.ofMinutes(3)));
  }

  /**
   * Resolves the image to use: the {@link #IMAGE_PROPERTY} system property, else {@link
   * #DEFAULT_IMAGE}.
   *
   * @return the image name
   */
  public static DockerImageName defaultImage() {
    return DockerImageName.parse(System.getProperty(IMAGE_PROPERTY, DEFAULT_IMAGE));
  }

  /**
   * Fixes the wallet's HD seed; by default each node gets a fresh random one.
   *
   * @param seed 32 bytes as 64 hex characters, without {@code 0x}
   * @return this container
   */
  public PaladinNodeContainer withSeed(final String seed) {
    config.seed(seed);
    return this;
  }

  /**
   * Sets the node's log level; defaults to {@code info}.
   *
   * @param logLevel e.g. {@code debug}
   * @return this container
   */
  public PaladinNodeContainer withLogLevel(final String logLevel) {
    config.logLevel(logLevel);
    return this;
  }

  /**
   * Appends top-level configuration such as {@code domains:} or {@code transports:}; see {@link
   * PaladinNodeConfig.Builder#section(String)}.
   *
   * @param yaml the fragment
   * @return this container
   */
  public PaladinNodeContainer withConfigSection(final String yaml) {
    config.section(yaml);
    return this;
  }

  /**
   * Returns the node name.
   *
   * @return the node name
   */
  public String nodeName() {
    return nodeName;
  }

  /**
   * Returns the configuration the node runs with, rendering it if the node has not started yet.
   * After the first call, further configuration changes have no effect.
   *
   * @return the configuration
   */
  public synchronized PaladinNodeConfig nodeConfig() {
    if (rendered == null) {
      rendered = config.build();
    }
    return rendered;
  }

  @Override
  protected void configure() {
    super.configure();
    withCopyToContainer(Transferable.of(nodeConfig().toYaml()), PaladinNodeConfig.CONFIG_PATH);
  }

  /**
   * Returns the HTTP JSON-RPC URL as seen by the test JVM. Only valid once started.
   *
   * @return the mapped URL
   */
  public String rpcUrl() {
    return "http://" + getHost() + ":" + getMappedPort(PaladinNodeConfig.RPC_HTTP_PORT);
  }

  /**
   * Returns the WebSocket JSON-RPC URL as seen by the test JVM. Only valid once started.
   *
   * @return the mapped URL
   */
  public String wsUrl() {
    return "ws://" + getHost() + ":" + getMappedPort(PaladinNodeConfig.RPC_WS_PORT);
  }

  /**
   * Opens a new client to this node. The caller closes it.
   *
   * @return a client over HTTP
   */
  public PaladinClient newClient() {
    return PaladinClient.http(rpcUrl());
  }
}
