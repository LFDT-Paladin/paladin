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

import com.fasterxml.jackson.databind.JsonNode;
import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.testcontainers.containers.ContainerLaunchException;
import org.testcontainers.containers.wait.strategy.AbstractWaitStrategy;

/**
 * Waits until a container's JSON-RPC endpoint answers a call with a result.
 *
 * <p>A listening port is not enough for a Paladin node: the RPC server can accept connections
 * before every component is ready. A successful call to a cheap read method is the signal that the
 * node is serving.
 */
public final class JsonRpcWaitStrategy extends AbstractWaitStrategy {

  private static final Duration POLL_INTERVAL = Duration.ofMillis(500);
  private static final Duration REQUEST_TIMEOUT = Duration.ofSeconds(5);

  private final int port;
  private final String method;

  /**
   * Creates a strategy that calls {@code method} with no parameters on the container's {@code
   * port}.
   *
   * @param port the JSON-RPC port inside the container
   * @param method a read-only JSON-RPC method that needs no parameters
   */
  public JsonRpcWaitStrategy(final int port, final String method) {
    this.port = port;
    this.method = method;
  }

  @Override
  protected void waitUntilReady() {
    final URI uri =
        URI.create(
            "http://"
                + waitStrategyTarget.getHost()
                + ":"
                + waitStrategyTarget.getMappedPort(port));
    final String body =
        "{\"jsonrpc\":\"2.0\",\"id\":\"ready\",\"method\":\"" + method + "\",\"params\":[]}";
    final HttpRequest request =
        HttpRequest.newBuilder(uri)
            .timeout(REQUEST_TIMEOUT)
            .header("Content-Type", "application/json")
            .POST(HttpRequest.BodyPublishers.ofString(body))
            .build();
    final long deadline = System.nanoTime() + startupTimeout.toNanos();
    String lastProblem = "no response";
    try (HttpClient http = HttpClient.newBuilder().connectTimeout(REQUEST_TIMEOUT).build()) {
      while (System.nanoTime() < deadline) {
        if (!waitStrategyTarget.isRunning()) {
          throw new ContainerLaunchException(
              "container exited before " + method + " answered on port " + port);
        }
        try {
          final HttpResponse<String> response =
              http.send(request, HttpResponse.BodyHandlers.ofString());
          final JsonNode json = PaladinObjectMapper.shared().readTree(response.body());
          if (response.statusCode() == 200 && json.has("result")) {
            return;
          }
          lastProblem = "HTTP " + response.statusCode() + ": " + response.body();
        } catch (final IOException e) {
          lastProblem = e.toString();
        }
        Thread.sleep(POLL_INTERVAL.toMillis());
      }
    } catch (final InterruptedException e) {
      Thread.currentThread().interrupt();
      throw new ContainerLaunchException("interrupted waiting for " + method, e);
    }
    throw new ContainerLaunchException(
        "timed out after "
            + startupTimeout.toSeconds()
            + "s waiting for "
            + method
            + " on port "
            + port
            + "; last: "
            + lastProblem);
  }
}
