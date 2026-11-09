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
package org.lfdt.paladin.sdk.testing.wiremock;

import static com.github.tomakehurst.wiremock.core.WireMockConfiguration.options;

import com.github.tomakehurst.wiremock.WireMockServer;
import com.github.tomakehurst.wiremock.client.MappingBuilder;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.config.RetryPolicy;
import org.lfdt.paladin.sdk.client.config.RpcClientConfig;

/**
 * A WireMock server standing in for a Paladin node's HTTP JSON-RPC endpoint, on a free local port.
 *
 * <pre>{@code
 * try (PaladinWireMock node = PaladinWireMock.start()) {
 *   node.stub(PaladinRpcStubs.sendTransaction(txId));
 *   PaladinClient client = node.paladinClient();
 *   ...
 *   assertEquals(1, node.callCount("ptx_sendTransaction"));
 * }
 * }</pre>
 *
 * <p>Clients from {@link #paladinClient()} do not retry, so a missing stub (WireMock answers {@code
 * 404}) fails the call at once instead of after the default backoff.
 */
public final class PaladinWireMock implements AutoCloseable {

  private final WireMockServer server;

  private PaladinWireMock(final WireMockServer server) {
    this.server = server;
  }

  /**
   * Starts a server on a free port.
   *
   * @return the running server
   */
  public static PaladinWireMock start() {
    final WireMockServer server = new WireMockServer(options().dynamicPort());
    server.start();
    return new PaladinWireMock(server);
  }

  /**
   * Returns the URL to point a Paladin client at.
   *
   * @return the server's base URL, e.g. {@code http://localhost:51234}
   */
  public String url() {
    return server.baseUrl();
  }

  /**
   * Registers a stub, typically built with {@link PaladinRpcStubs}.
   *
   * @param mapping the stub
   * @return this server, so stubs can be chained
   */
  public PaladinWireMock stub(final MappingBuilder mapping) {
    server.stubFor(mapping);
    return this;
  }

  /**
   * Counts the requests received for one JSON-RPC method.
   *
   * @param method the JSON-RPC method name
   * @return how many calls were made
   */
  public int callCount(final String method) {
    return server.findAll(PaladinRpcStubs.called(method)).size();
  }

  /**
   * Creates a client for this server with retries disabled. The caller closes it.
   *
   * @return a new client
   */
  public PaladinClient paladinClient() {
    return PaladinClient.http(
        RpcClientConfig.builder(url())
            .retryPolicy(RetryPolicy.builder().maxAttempts(1).build())
            .build());
  }

  /**
   * Gives direct access to the WireMock server for anything these helpers do not cover.
   *
   * @return the underlying server
   */
  public WireMockServer server() {
    return server;
  }

  /** Removes every stub and recorded request. */
  public void reset() {
    server.resetAll();
  }

  /** Stops the server. */
  @Override
  public void close() {
    server.stop();
  }
}
