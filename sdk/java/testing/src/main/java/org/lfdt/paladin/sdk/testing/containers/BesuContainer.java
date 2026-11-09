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
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.containers.Network;
import org.testcontainers.containers.wait.strategy.Wait;
import org.testcontainers.utility.DockerImageName;
import org.testcontainers.utility.MountableFile;

/**
 * A single-validator, free-gas Hyperledger Besu node: the base ledger for Paladin nodes under test.
 *
 * <p>The chain is QBFT with chain id {@code 1337}, a zero base fee and a 200ms block period. Its
 * genesis and validator key are fixed test fixtures shipped with this module, so the chain needs no
 * bootstrap step; the key guards nothing and must never be used outside tests.
 *
 * <p>Other containers on the same {@link Network} reach it at {@link #internalHttpUrl()} and {@link
 * #internalWsUrl()}; the test JVM uses {@link #httpUrl()}.
 */
public class BesuContainer extends GenericContainer<BesuContainer> {

  /** The Besu image used unless another is given. */
  public static final DockerImageName DEFAULT_IMAGE =
      DockerImageName.parse("hyperledger/besu:25.8.0");

  /** The host name other containers on the network use for this node. */
  public static final String NETWORK_ALIAS = "besu";

  /** Ethereum HTTP JSON-RPC port inside the container. */
  public static final int HTTP_PORT = 8545;

  /** Ethereum WebSocket JSON-RPC port inside the container. */
  public static final int WS_PORT = 8546;

  private static final String FIXTURES = "org/lfdt/paladin/sdk/testing/besu/";
  private static final int READ_ONLY = 0644;

  /** Creates a node from {@link #DEFAULT_IMAGE}. */
  public BesuContainer() {
    this(DEFAULT_IMAGE);
  }

  /**
   * Creates a node from a specific Besu image.
   *
   * @param image the image
   */
  public BesuContainer(final DockerImageName image) {
    super(image);
    withNetworkAliases(NETWORK_ALIAS);
    withExposedPorts(HTTP_PORT, WS_PORT);
    withCopyToContainer(
        MountableFile.forClasspathResource(FIXTURES + "genesis.json", READ_ONLY),
        "/var/besu/genesis.json");
    withCopyToContainer(
        MountableFile.forClasspathResource(FIXTURES + "key", READ_ONLY), "/var/besu/key");
    withCommand(
        "--genesis-file=/var/besu/genesis.json",
        "--node-private-key-file=/var/besu/key",
        "--data-path=/tmp/besu-data",
        "--rpc-http-enabled",
        "--rpc-http-host=0.0.0.0",
        "--rpc-http-api=ETH,QBFT,WEB3,DEBUG",
        "--rpc-ws-enabled",
        "--rpc-ws-host=0.0.0.0",
        "--rpc-ws-api=ETH,QBFT,WEB3,DEBUG",
        "--host-allowlist=*",
        "--min-gas-price=0",
        "--revert-reason-enabled",
        "--tx-pool=SEQUENCED",
        "--tx-pool-limit-by-account-percentage=1.0",
        "--target-gas-limit=30000000");
    waitingFor(
        Wait.forHttp("/liveness")
            .forPort(HTTP_PORT)
            .forStatusCode(200)
            .withStartupTimeout(Duration.ofMinutes(2)));
  }

  /**
   * Places the node on a network so Paladin containers can reach it by {@link #NETWORK_ALIAS}.
   *
   * @param network the shared network
   * @return this container
   */
  public BesuContainer onNetwork(final Network network) {
    return withNetwork(network);
  }

  /**
   * Returns the HTTP JSON-RPC URL as seen by other containers on the network.
   *
   * @return e.g. {@code http://besu:8545}
   */
  public String internalHttpUrl() {
    return "http://" + NETWORK_ALIAS + ":" + HTTP_PORT;
  }

  /**
   * Returns the WebSocket JSON-RPC URL as seen by other containers on the network.
   *
   * @return e.g. {@code ws://besu:8546}
   */
  public String internalWsUrl() {
    return "ws://" + NETWORK_ALIAS + ":" + WS_PORT;
  }

  /**
   * Returns the HTTP JSON-RPC URL as seen by the test JVM. Only valid once started.
   *
   * @return the mapped URL
   */
  public String httpUrl() {
    return "http://" + getHost() + ":" + getMappedPort(HTTP_PORT);
  }
}
