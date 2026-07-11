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
package org.lfdt.paladin.sdk.client.rpc;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.sun.net.httpserver.Headers;
import com.sun.net.httpserver.HttpServer;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * A tiny in-process JSON-RPC server backed by the JDK's {@link HttpServer}, used to exercise {@link
 * HttpRpcClient} without any third-party test dependency. A {@link Responder} decides the reply for
 * each request, keyed by the 1-based request number, so retry scenarios can return different
 * statuses per attempt.
 */
final class MockJsonRpcServer implements AutoCloseable {

  /** Decides the canned reply for a single request. */
  @FunctionalInterface
  interface Responder {
    Response respond(int requestNumber, JsonNode requestBody) throws IOException;
  }

  /** A canned HTTP reply: status, body, and an optional artificial delay (to provoke timeouts). */
  static final class Response {
    private final int status;
    private final String body;
    private final long delayMillis;

    private Response(final int status, final String body, final long delayMillis) {
      this.status = status;
      this.body = body;
      this.delayMillis = delayMillis;
    }

    static Response of(final int status, final String body) {
      return new Response(status, body, 0);
    }

    Response withDelayMillis(final long millis) {
      return new Response(this.status, this.body, millis);
    }
  }

  private final HttpServer server;
  private final ObjectMapper mapper = new ObjectMapper();
  private final AtomicInteger requestCount = new AtomicInteger();
  private final List<JsonNode> requests = new CopyOnWriteArrayList<>();
  private volatile Headers lastRequestHeaders;

  MockJsonRpcServer(final Responder responder) throws IOException {
    server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
    server.createContext(
        "/",
        exchange -> {
          try {
            final byte[] requestBytes = exchange.getRequestBody().readAllBytes();
            final JsonNode request =
                requestBytes.length == 0 ? null : mapper.readTree(requestBytes);
            requests.add(request);
            lastRequestHeaders = exchange.getRequestHeaders();
            final int number = requestCount.incrementAndGet();

            final Response response = responder.respond(number, request);
            if (response.delayMillis > 0) {
              try {
                Thread.sleep(response.delayMillis);
              } catch (final InterruptedException e) {
                Thread.currentThread().interrupt();
              }
            }

            final byte[] out =
                response.body == null
                    ? new byte[0]
                    : response.body.getBytes(StandardCharsets.UTF_8);
            exchange.getResponseHeaders().add("Content-Type", "application/json");
            try {
              exchange.sendResponseHeaders(response.status, out.length == 0 ? -1 : out.length);
              if (out.length > 0) {
                exchange.getResponseBody().write(out);
              }
            } catch (final IOException ignored) {
              // The client may have disconnected (e.g. on a deliberate timeout); nothing to do.
            }
          } finally {
            exchange.close();
          }
        });
    server.setExecutor(Executors.newCachedThreadPool());
    server.start();
  }

  String baseUrl() {
    return "http://127.0.0.1:" + server.getAddress().getPort();
  }

  int requestCount() {
    return requestCount.get();
  }

  List<JsonNode> requests() {
    return requests;
  }

  String requestHeader(final String name) {
    final Headers headers = lastRequestHeaders;
    return headers == null ? null : headers.getFirst(name);
  }

  @Override
  public void close() {
    server.stop(0);
  }
}
