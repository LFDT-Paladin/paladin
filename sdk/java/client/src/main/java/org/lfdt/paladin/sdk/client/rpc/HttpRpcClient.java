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

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JavaType;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.io.IOException;
import java.net.ConnectException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.net.http.HttpTimeoutException;
import java.time.Duration;
import java.util.Arrays;
import java.util.List;
import java.util.Locale;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CompletionException;
import java.util.concurrent.Executor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import org.lfdt.paladin.sdk.client.config.RetryPolicy;
import org.lfdt.paladin.sdk.client.config.RpcClientConfig;
import org.lfdt.paladin.sdk.client.exception.PaladinConnectionException;
import org.lfdt.paladin.sdk.client.exception.PaladinException;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.exception.PaladinTimeoutException;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;

/**
 * Asynchronous JSON-RPC client over {@code java.net.http}, mirroring Go's {@code rpcclient} HTTP
 * transport.
 *
 * <p>Built entirely on the JDK's {@link HttpClient} — no third-party HTTP dependency. Every call is
 * non-blocking ({@link HttpClient#sendAsync}); the returned {@link CompletableFuture} completes
 * with the deserialized {@code result} or, exceptionally, with a {@link PaladinException} subtype.
 * Request ids are a monotonic, 9-digit zero-padded counter, mirroring Go's {@code %.9d}.
 *
 * <p>Transient failures — connection errors, timeouts, and transient HTTP statuses ({@code 429} and
 * {@code 5xx}) — are retried with exponential backoff per the configured {@link RetryPolicy}.
 * JSON-RPC application errors and other {@code 4xx} responses are returned to the caller without
 * retry.
 *
 * <p>Thread-safe and intended to be shared; {@link #close()} releases the underlying client.
 */
public final class HttpRpcClient implements RpcClient {

  private static final String CONTENT_TYPE = "Content-Type";
  private static final String APPLICATION_JSON = "application/json";

  private final RpcClientConfig config;
  private final HttpClient httpClient;
  private final ObjectMapper mapper;
  private final URI uri;
  private final AtomicLong requestCounter = new AtomicLong();

  /**
   * Creates a client for the given configuration.
   *
   * @param config the connection, timeout, header, and retry configuration
   */
  public HttpRpcClient(RpcClientConfig config) {
    this.config = config;
    this.mapper = PaladinObjectMapper.shared();
    this.uri = URI.create(config.url());
    this.httpClient = HttpClient.newBuilder().connectTimeout(config.connectTimeout()).build();
  }

  @Override
  public <T> CompletableFuture<T> callRpc(Class<T> resultType, String method, Object... params) {
    return call(mapper.getTypeFactory().constructType(resultType), method, params);
  }

  @Override
  public <T> CompletableFuture<T> callRpc(
      TypeReference<T> resultType, String method, Object... params) {
    return call(mapper.getTypeFactory().constructType(resultType), method, params);
  }

  @Override
  public void close() {
    httpClient.close();
  }

  private <T> CompletableFuture<T> call(JavaType resultType, String method, Object[] params) {
    final HttpRequest request;
    try {
      request = buildRequest(method, params);
    } catch (JsonProcessingException e) {
      return CompletableFuture.failedFuture(
          new PaladinRpcException(
              JsonRpcErrorCode.INVALID_REQUEST,
              "failed to serialize request: " + e.getMessage(),
              null,
              0));
    }
    return sendWithRetry(request, 1).thenApply(response -> processResponse(response, resultType));
  }

  private HttpRequest buildRequest(String method, Object[] params) throws JsonProcessingException {
    String id = String.format(Locale.ROOT, "%09d", requestCounter.incrementAndGet());
    List<Object> args = params == null ? List.of() : Arrays.asList(params);
    byte[] body = mapper.writeValueAsBytes(new JsonRpcRequest(id, method, args));
    HttpRequest.Builder builder =
        HttpRequest.newBuilder(uri)
            .timeout(config.requestTimeout())
            .header(CONTENT_TYPE, APPLICATION_JSON)
            .header("Accept", APPLICATION_JSON)
            .POST(HttpRequest.BodyPublishers.ofByteArray(body));
    config.headers().forEach(builder::header);
    return builder.build();
  }

  private CompletableFuture<HttpResponse<byte[]>> sendWithRetry(HttpRequest request, int attempt) {
    return httpClient
        .sendAsync(request, HttpResponse.BodyHandlers.ofByteArray())
        .handle((response, throwable) -> new Attempt(response, throwable))
        .thenCompose(
            result -> {
              if (result.throwable != null) {
                PaladinException mapped = mapTransportError(result.throwable);
                if (config.retryPolicy().shouldRetry(attempt)) {
                  return retryAfterDelay(request, attempt);
                }
                return CompletableFuture.failedFuture(mapped);
              }
              if (isRetryableStatus(result.response.statusCode())
                  && config.retryPolicy().shouldRetry(attempt)) {
                return retryAfterDelay(request, attempt);
              }
              return CompletableFuture.completedFuture(result.response);
            });
  }

  private CompletableFuture<HttpResponse<byte[]>> retryAfterDelay(
      HttpRequest request, int attempt) {
    Duration delay = config.retryPolicy().delayForAttempt(attempt);
    Executor delayed = CompletableFuture.delayedExecutor(delay.toMillis(), TimeUnit.MILLISECONDS);
    return CompletableFuture.supplyAsync(() -> null, delayed)
        .thenCompose(ignored -> sendWithRetry(request, attempt + 1));
  }

  private <T> T processResponse(HttpResponse<byte[]> response, JavaType resultType) {
    int status = response.statusCode();
    byte[] body = response.body();

    JsonRpcResponse rpcResponse = null;
    if (body != null && body.length > 0) {
      try {
        rpcResponse = mapper.readValue(body, JsonRpcResponse.class);
      } catch (IOException e) {
        // A non-JSON body on a failure status is a transport-level error, not a JSON-RPC error.
        if (!isSuccess(status)) {
          throw new PaladinRpcException(
              0, "HTTP request failed with status " + status, null, status);
        }
        throw new PaladinConnectionException("failed to parse JSON-RPC response body", e);
      }
    }

    if (rpcResponse != null && rpcResponse.hasError()) {
      JsonRpcError error = rpcResponse.error();
      throw new PaladinRpcException(error.code(), error.message(), error.data(), status);
    }
    if (!isSuccess(status)) {
      throw new PaladinRpcException(0, "HTTP request failed with status " + status, null, status);
    }

    JsonNode result = rpcResponse == null ? null : rpcResponse.result();
    if (result == null || result.isNull()) {
      return null;
    }
    try {
      return mapper.convertValue(result, resultType);
    } catch (IllegalArgumentException e) {
      throw new PaladinRpcException(
          JsonRpcErrorCode.PARSE_ERROR, "failed to parse result: " + e.getMessage(), null, status);
    }
  }

  private PaladinException mapTransportError(Throwable throwable) {
    Throwable cause = unwrap(throwable);
    if (cause instanceof HttpTimeoutException) {
      return new PaladinTimeoutException(
          "request to " + uri + " timed out after " + config.requestTimeout(), cause);
    }
    if (cause instanceof ConnectException) {
      return new PaladinConnectionException(
          "failed to connect to " + uri + ": " + cause.getMessage(), cause);
    }
    if (cause instanceof IOException) {
      return new PaladinConnectionException(
          "transport error calling " + uri + ": " + cause.getMessage(), cause);
    }
    return new PaladinConnectionException(
        "unexpected error calling " + uri + ": " + cause.getMessage(), cause);
  }

  private static Throwable unwrap(Throwable throwable) {
    Throwable cause = throwable;
    while ((cause instanceof CompletionException) && cause.getCause() != null) {
      cause = cause.getCause();
    }
    return cause;
  }

  private static boolean isSuccess(int status) {
    return status >= 200 && status < 300;
  }

  private static boolean isRetryableStatus(int status) {
    return status == 429 || (status >= 500 && status < 600);
  }

  /**
   * Holds the outcome of a single {@code sendAsync} so success and failure can be branched
   * together.
   */
  private static final class Attempt {
    private final HttpResponse<byte[]> response;
    private final Throwable throwable;

    private Attempt(HttpResponse<byte[]> response, Throwable throwable) {
      this.response = response;
      this.throwable = throwable;
    }
  }
}
