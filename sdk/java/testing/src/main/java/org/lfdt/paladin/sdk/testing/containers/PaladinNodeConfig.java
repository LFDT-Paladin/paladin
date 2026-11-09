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

import java.security.SecureRandom;
import java.util.ArrayList;
import java.util.HexFormat;
import java.util.List;
import java.util.Objects;
import java.util.regex.Pattern;

/**
 * The configuration file for a Paladin node running in a container.
 *
 * <p>The helper owns the keys a node needs to boot — {@code nodeName}, {@code db}, {@code
 * rpcServer}, {@code blockchain}, {@code wallets}, {@code log} — and renders them for an in-memory
 * SQLite database and a single HD wallet. Anything else (domains, transports, registries, tuning)
 * is added with {@link Builder#section(String)} as further top-level YAML, which keeps this class
 * independent of how those plugins are configured.
 */
public final class PaladinNodeConfig {

  /** Port of the node's HTTP JSON-RPC server inside the container. */
  public static final int RPC_HTTP_PORT = 8548;

  /** Port of the node's WebSocket JSON-RPC server inside the container. */
  public static final int RPC_WS_PORT = 8549;

  /** Where the rendered file is placed in the container. */
  public static final String CONFIG_PATH = "/app/config/pldconf.paladin.yaml";

  private static final Pattern NODE_NAME = Pattern.compile("[a-zA-Z0-9][a-zA-Z0-9_-]*");
  private static final Pattern SEED = Pattern.compile("[0-9a-fA-F]{64}");
  private static final List<String> OWNED_KEYS =
      List.of("nodeName", "db", "rpcServer", "blockchain", "wallets", "log");

  private final String nodeName;
  private final String seed;
  private final String blockchainHttpUrl;
  private final String blockchainWsUrl;
  private final String logLevel;
  private final List<String> sections;

  private PaladinNodeConfig(final Builder builder) {
    this.nodeName = builder.nodeName;
    this.seed = builder.seed == null ? randomSeed() : builder.seed;
    this.blockchainHttpUrl = Objects.requireNonNull(builder.blockchainHttpUrl, "blockchain url");
    this.blockchainWsUrl = Objects.requireNonNull(builder.blockchainWsUrl, "blockchain ws url");
    this.logLevel = builder.logLevel;
    this.sections = List.copyOf(builder.sections);
  }

  /**
   * Starts a configuration for one node.
   *
   * @param nodeName the node name; letters, digits, {@code _} and {@code -}
   * @return a builder
   * @throws IllegalArgumentException if the name is not usable as a node name and network alias
   */
  public static Builder builder(final String nodeName) {
    return new Builder(nodeName);
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
   * Returns the wallet's HD seed, so a test can derive the same keys elsewhere.
   *
   * @return 64 hex characters
   */
  public String seed() {
    return seed;
  }

  /**
   * Renders the configuration file.
   *
   * @return the YAML document
   */
  public String toYaml() {
    final StringBuilder yaml = new StringBuilder();
    yaml.append("nodeName: ").append(nodeName).append('\n');
    yaml.append("log:\n  level: ").append(logLevel).append('\n');
    yaml.append(
        """
        db:
          type: sqlite
          sqlite:
            dsn: ":memory:"
            autoMigrate: true
            migrationsDir: /app/db/migrations/sqlite
        rpcServer:
          http:
            address: 0.0.0.0
            port: %d
          ws:
            address: 0.0.0.0
            port: %d
        blockchain:
          http:
            url: %s
          ws:
            url: %s
            initialConnectAttempts: 25
        wallets:
          - name: wallet1
            keySelector: .*
            signer:
              keyDerivation:
                type: bip32
              keyStore:
                type: static
                static:
                  keys:
                    seed:
                      encoding: none
                      inline: %s
        """
            .formatted(RPC_HTTP_PORT, RPC_WS_PORT, blockchainHttpUrl, blockchainWsUrl, seed));
    for (String section : sections) {
      yaml.append(section.strip()).append('\n');
    }
    return yaml.toString();
  }

  private static String randomSeed() {
    final byte[] bytes = new byte[32];
    new SecureRandom().nextBytes(bytes);
    return HexFormat.of().formatHex(bytes);
  }

  /** Builds a {@link PaladinNodeConfig}. */
  public static final class Builder {

    private final String nodeName;
    private String seed;
    private String blockchainHttpUrl;
    private String blockchainWsUrl;
    private String logLevel = "info";
    private final List<String> sections = new ArrayList<>();

    private Builder(final String nodeName) {
      if (nodeName == null || !NODE_NAME.matcher(nodeName).matches()) {
        throw new IllegalArgumentException("invalid node name: " + nodeName);
      }
      this.nodeName = nodeName;
    }

    /**
     * Fixes the wallet's HD seed; by default each node gets a fresh random one.
     *
     * @param seed 32 bytes as 64 hex characters, without {@code 0x}
     * @return this builder
     * @throws IllegalArgumentException if the seed is not 64 hex characters
     */
    public Builder seed(final String seed) {
      if (seed == null || !SEED.matcher(seed).matches()) {
        throw new IllegalArgumentException("seed must be 64 hex characters without 0x");
      }
      this.seed = seed;
      return this;
    }

    /**
     * Sets the base ledger endpoints, as reachable from inside the node's container.
     *
     * @param httpUrl the Ethereum HTTP JSON-RPC URL
     * @param wsUrl the Ethereum WebSocket JSON-RPC URL
     * @return this builder
     */
    public Builder blockchain(final String httpUrl, final String wsUrl) {
      this.blockchainHttpUrl = httpUrl;
      this.blockchainWsUrl = wsUrl;
      return this;
    }

    /**
     * Sets the node's log level; defaults to {@code info}.
     *
     * @param logLevel e.g. {@code debug}, {@code info}, {@code error}
     * @return this builder
     */
    public Builder logLevel(final String logLevel) {
      this.logLevel = Objects.requireNonNull(logLevel, "logLevel");
      return this;
    }

    /**
     * Appends further top-level configuration, such as {@code domains:}, {@code transports:} or
     * {@code registries:}. The fragment is written verbatim, so it must be valid YAML starting at
     * column zero.
     *
     * @param yaml the fragment
     * @return this builder
     * @throws IllegalArgumentException if the fragment sets a key this helper already owns
     */
    public Builder section(final String yaml) {
      Objects.requireNonNull(yaml, "yaml");
      for (String line : yaml.strip().split("\n")) {
        for (String key : OWNED_KEYS) {
          if (line.startsWith(key + ":")) {
            throw new IllegalArgumentException(
                "'" + key + "' is set by the helper; use the builder method instead");
          }
        }
      }
      sections.add(yaml);
      return this;
    }

    /**
     * Builds the configuration.
     *
     * @return the configuration
     * @throws NullPointerException if the blockchain endpoints were not set
     */
    public PaladinNodeConfig build() {
      return new PaladinNodeConfig(this);
    }
  }
}
