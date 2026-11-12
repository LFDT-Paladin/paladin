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

import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.Consumer;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.testcontainers.containers.Network;
import org.testcontainers.lifecycle.Startables;
import org.testcontainers.utility.DockerImageName;

/**
 * A Besu base ledger plus one or more Paladin nodes on a private Docker network — the one place
 * tests get a real node from, so container wiring is never repeated per test class.
 *
 * <p>For most tests the JVM-wide {@link #shared()} network is enough: it starts on first use and is
 * reused by every test class after that.
 *
 * <pre>{@code
 * PaladinClient client = PaladinTestNetwork.shared().client("node1");
 * }</pre>
 *
 * <p>A test that needs more nodes or extra node configuration builds its own:
 *
 * <pre>{@code
 * try (PaladinTestNetwork network =
 *     PaladinTestNetwork.builder()
 *         .nodes("node1", "node2")
 *         .configure("node2", node -> node.withLogLevel("debug"))
 *         .start()) {
 *   PaladinClient node1 = network.client("node1");
 *   ...
 * }
 * }</pre>
 *
 * <p>Nodes share the base ledger, so each sees the others' public transactions. Private
 * transactions across nodes also need a transport and a registry, which a test adds per node with
 * {@link PaladinNodeContainer#withConfigSection(String)} through {@link Builder#configure}.
 */
public final class PaladinTestNetwork implements AutoCloseable {

  /** The node name {@link #shared()} and {@link Builder} use when none is given. */
  public static final String DEFAULT_NODE = "node1";

  private static volatile PaladinTestNetwork sharedNetwork;

  private final Network network;
  private final BesuContainer besu;
  private final Map<String, PaladinNodeContainer> nodes;
  private final Map<String, PaladinClient> clients = new ConcurrentHashMap<>();

  private PaladinTestNetwork(
      final Network network,
      final BesuContainer besu,
      final Map<String, PaladinNodeContainer> nodes) {
    this.network = network;
    this.besu = besu;
    this.nodes = Collections.unmodifiableMap(nodes);
  }

  /**
   * Starts a network definition.
   *
   * @return a builder
   */
  public static Builder builder() {
    return new Builder();
  }

  /**
   * Returns the JVM-wide single-node network ({@value #DEFAULT_NODE}), starting it on first call.
   * It stays up for the life of the JVM and is removed when the JVM exits; do not close it.
   *
   * @return the running shared network
   */
  public static PaladinTestNetwork shared() {
    PaladinTestNetwork current = sharedNetwork;
    if (current == null) {
      synchronized (PaladinTestNetwork.class) {
        current = sharedNetwork;
        if (current == null) {
          current = builder().start();
          final PaladinTestNetwork started = current;
          Runtime.getRuntime().addShutdownHook(new Thread(started::close, "paladin-test-network"));
          sharedNetwork = current;
        }
      }
    }
    return current;
  }

  /**
   * Returns the base ledger.
   *
   * @return the Besu container
   */
  public BesuContainer besu() {
    return besu;
  }

  /**
   * Returns the node names, in the order they were declared.
   *
   * @return the node names
   */
  public List<String> nodeNames() {
    return List.copyOf(nodes.keySet());
  }

  /**
   * Returns one node's container, for its URLs, logs or configuration.
   *
   * @param name the node name
   * @return the container
   * @throws IllegalArgumentException if there is no such node
   */
  public PaladinNodeContainer node(final String name) {
    final PaladinNodeContainer node = nodes.get(name);
    if (node == null) {
      throw new IllegalArgumentException("no node '" + name + "'; nodes are " + nodes.keySet());
    }
    return node;
  }

  /**
   * Returns a client to one node, shared by every caller and closed with the network. Do not close
   * it; use {@link PaladinNodeContainer#newClient()} for a client of your own.
   *
   * @param name the node name
   * @return the client
   * @throws IllegalArgumentException if there is no such node
   */
  public PaladinClient client(final String name) {
    final PaladinNodeContainer node = node(name);
    return clients.computeIfAbsent(name, ignored -> node.newClient());
  }

  /** Closes the clients, stops every container and removes the network. */
  @Override
  public void close() {
    clients.values().forEach(PaladinClient::close);
    clients.clear();
    nodes.values().forEach(PaladinNodeContainer::stop);
    besu.stop();
    network.close();
  }

  /** Defines a {@link PaladinTestNetwork}. */
  public static final class Builder {

    private final List<String> nodeNames = new ArrayList<>();
    private final Map<String, List<Consumer<PaladinNodeContainer>>> customizers =
        new LinkedHashMap<>();
    private DockerImageName paladinImage = PaladinNodeContainer.defaultImage();
    private DockerImageName besuImage = BesuContainer.DEFAULT_IMAGE;

    private Builder() {}

    /**
     * Declares the Paladin nodes, in order; defaults to a single {@value
     * PaladinTestNetwork#DEFAULT_NODE}.
     *
     * @param names the node names
     * @return this builder
     */
    public Builder nodes(final String... names) {
      for (String name : names) {
        if (nodeNames.contains(name)) {
          throw new IllegalArgumentException("duplicate node name: " + name);
        }
        nodeNames.add(Objects.requireNonNull(name, "name"));
      }
      return this;
    }

    /**
     * Adjusts one node's container before it starts — log level, seed, extra config sections.
     *
     * @param name the node name, which must also be declared with {@link #nodes}
     * @param customizer applied to the container
     * @return this builder
     */
    public Builder configure(final String name, final Consumer<PaladinNodeContainer> customizer) {
      customizers
          .computeIfAbsent(name, ignored -> new ArrayList<>())
          .add(Objects.requireNonNull(customizer));
      return this;
    }

    /**
     * Adjusts every node's container before it starts.
     *
     * @param customizer applied to each container
     * @return this builder
     */
    public Builder configureAll(final Consumer<PaladinNodeContainer> customizer) {
      return configure("*", customizer);
    }

    /**
     * Sets the Paladin image; defaults to {@link PaladinNodeContainer#defaultImage()}.
     *
     * @param image the image
     * @return this builder
     */
    public Builder paladinImage(final DockerImageName image) {
      this.paladinImage = Objects.requireNonNull(image);
      return this;
    }

    /**
     * Sets the Besu image; defaults to {@link BesuContainer#DEFAULT_IMAGE}.
     *
     * @param image the image
     * @return this builder
     */
    public Builder besuImage(final DockerImageName image) {
      this.besuImage = Objects.requireNonNull(image);
      return this;
    }

    /**
     * Starts Besu, then every node in parallel, and waits until all of them serve JSON-RPC.
     *
     * @return the running network; the caller closes it
     * @throws IllegalArgumentException if a customizer names an undeclared node
     */
    public PaladinTestNetwork start() {
      final List<String> names = nodeNames.isEmpty() ? List.of(DEFAULT_NODE) : nodeNames;
      for (String name : customizers.keySet()) {
        if (!"*".equals(name) && !names.contains(name)) {
          throw new IllegalArgumentException("configure() names unknown node: " + name);
        }
      }
      final Network network = Network.newNetwork();
      final BesuContainer besu = new BesuContainer(besuImage).onNetwork(network);
      final Map<String, PaladinNodeContainer> nodes = new LinkedHashMap<>();
      for (String name : names) {
        final PaladinNodeContainer node = new PaladinNodeContainer(paladinImage, name, besu);
        customizers.getOrDefault("*", List.of()).forEach(c -> c.accept(node));
        customizers.getOrDefault(name, List.of()).forEach(c -> c.accept(node));
        nodes.put(name, node);
      }
      final PaladinTestNetwork result = new PaladinTestNetwork(network, besu, nodes);
      try {
        besu.start();
        Startables.deepStart(nodes.values()).join();
      } catch (final RuntimeException e) {
        result.close();
        throw e;
      }
      return result;
    }
  }
}
