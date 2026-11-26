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
package org.lfdt.paladin.sdk.testing.mock;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JavaType;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.NullNode;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Deque;
import java.util.List;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.function.Function;
import java.util.function.Predicate;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.rpc.JsonRpcErrorCode;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;

/**
 * An in-memory {@link RpcClient} for unit tests: no network, no running node.
 *
 * <p>Stub a JSON-RPC method, hand the mock to code under test through {@link #paladinClient()},
 * then inspect what was called:
 *
 * <pre>{@code
 * MockRpcClient rpc = new MockRpcClient();
 * rpc.on("ptx_sendTransaction").thenReturn(txId);
 * rpc.on("ptx_getTransactionReceipt").thenReturn(null).thenReturn(Receipts.success(txId));
 *
 * TransactionReceipt receipt = rpc.paladinClient().newTx()...send().waitForReceipt().join();
 * assertEquals(1, rpc.calls("ptx_sendTransaction").size());
 * }</pre>
 *
 * <p>Results and parameters take the same JSON round trip through {@link PaladinObjectMapper} as
 * they would over HTTP, so a stub that would not deserialize from a real node fails here too.
 * Calling a method nothing matches fails with {@link JsonRpcErrorCode#METHOD_NOT_FOUND}, the code a
 * node returns for an unknown method.
 *
 * <p>Thread-safe: stubs may be registered and calls made from any thread.
 */
public final class MockRpcClient implements RpcClient {

  private final ObjectMapper mapper = PaladinObjectMapper.shared();
  private final List<Stub> stubs = new CopyOnWriteArrayList<>();
  private final List<RecordedCall> calls = new CopyOnWriteArrayList<>();
  private volatile boolean closed;

  /** Creates a mock with no stubs. */
  public MockRpcClient() {}

  /**
   * Starts a stub for every call to {@code method}, whatever its parameters.
   *
   * @param method the JSON-RPC method name, e.g. {@code ptx_sendTransaction}
   * @return the stub, to be given one or more responses
   */
  public Stub on(final String method) {
    return register(method, params -> true);
  }

  /**
   * Starts a stub for calls to {@code method} whose parameters equal {@code params} once both are
   * converted to JSON.
   *
   * @param method the JSON-RPC method name
   * @param params the expected positional parameters
   * @return the stub, to be given one or more responses
   */
  public Stub on(final String method, final Object... params) {
    final JsonNode expected =
        mapper.valueToTree(params == null ? List.of() : Arrays.asList(params));
    return register(method, actual -> expected.equals(mapper.valueToTree(actual)));
  }

  /**
   * Starts a stub for calls to {@code method} whose JSON parameters satisfy {@code matcher}.
   *
   * @param method the JSON-RPC method name
   * @param matcher a predicate over the positional parameters as JSON
   * @return the stub, to be given one or more responses
   */
  public Stub onMatching(final String method, final Predicate<List<JsonNode>> matcher) {
    return register(method, Objects.requireNonNull(matcher, "matcher"));
  }

  private Stub register(final String method, final Predicate<List<JsonNode>> matcher) {
    // Newest first, so a later, more specific stub overrides an earlier catch-all.
    final Stub stub = new Stub(Objects.requireNonNull(method, "method"), matcher);
    stubs.add(0, stub);
    return stub;
  }

  /**
   * Returns every call received, in order.
   *
   * @return an immutable snapshot of the recorded calls
   */
  public List<RecordedCall> calls() {
    return List.copyOf(calls);
  }

  /**
   * Returns the calls received for one method, in order.
   *
   * @param method the JSON-RPC method name
   * @return an immutable snapshot of the matching calls
   */
  public List<RecordedCall> calls(final String method) {
    return calls.stream().filter(call -> call.method().equals(method)).toList();
  }

  /**
   * Returns the most recent call to {@code method}.
   *
   * @param method the JSON-RPC method name
   * @return the last call
   * @throws AssertionError if the method was never called
   */
  public RecordedCall lastCall(final String method) {
    final List<RecordedCall> matching = calls(method);
    if (matching.isEmpty()) {
      throw new AssertionError("expected a call to " + method + " but got " + methodNames());
    }
    return matching.get(matching.size() - 1);
  }

  /** Clears every stub and recorded call. */
  public void reset() {
    stubs.clear();
    calls.clear();
  }

  /**
   * Wraps this mock in the SDK's top-level client, exactly as an HTTP connection would be.
   *
   * @return a {@link PaladinClient} backed by this mock
   */
  public PaladinClient paladinClient() {
    return PaladinClient.wrap(this);
  }

  /**
   * Reports whether {@link #close()} has been called, so tests can check the code under test
   * released its client.
   *
   * @return {@code true} once closed
   */
  public boolean isClosed() {
    return closed;
  }

  @Override
  public <T> CompletableFuture<T> callRpc(
      final Class<T> resultType, final String method, final Object... params) {
    return call(mapper.getTypeFactory().constructType(resultType), method, params);
  }

  @Override
  public <T> CompletableFuture<T> callRpc(
      final TypeReference<T> resultType, final String method, final Object... params) {
    return call(mapper.getTypeFactory().constructType(resultType), method, params);
  }

  @Override
  public void close() {
    closed = true;
  }

  private <T> CompletableFuture<T> call(
      final JavaType resultType, final String method, final Object[] params) {
    final List<JsonNode> jsonParams = new ArrayList<>();
    if (params != null) {
      for (Object param : params) {
        final JsonNode node = mapper.valueToTree(param);
        jsonParams.add(node == null ? NullNode.getInstance() : node);
      }
    }
    final RecordedCall call = new RecordedCall(method, jsonParams);
    calls.add(call);
    final Stub stub =
        stubs.stream()
            .filter(candidate -> candidate.matches(method, call.params()))
            .findFirst()
            .orElse(null);
    if (stub == null) {
      return CompletableFuture.failedFuture(
          new PaladinRpcException(
              JsonRpcErrorCode.METHOD_NOT_FOUND, "no stub for method " + method, null, 200));
    }
    try {
      final JsonNode result = mapper.valueToTree(stub.nextAnswer().apply(call));
      if (result == null || result.isNull()) {
        return CompletableFuture.completedFuture(null);
      }
      return CompletableFuture.completedFuture(mapper.convertValue(result, resultType));
    } catch (final RuntimeException e) {
      return CompletableFuture.failedFuture(e);
    }
  }

  private List<String> methodNames() {
    return calls.stream().map(RecordedCall::method).toList();
  }

  /**
   * The responses for one stubbed method. Responses are used in the order added; the last one
   * repeats for every later call.
   */
  public static final class Stub {

    private final String method;
    private final Predicate<List<JsonNode>> matcher;
    private final Deque<Function<RecordedCall, Object>> answers = new ArrayDeque<>();

    private Stub(final String method, final Predicate<List<JsonNode>> matcher) {
      this.method = method;
      this.matcher = matcher;
    }

    /**
     * Answers with a fixed result. {@code null} answers with a JSON {@code null} result, which is
     * how a node reports "not found yet" for receipts and lookups.
     *
     * @param result the value to return; serialized to JSON and back into the requested type
     * @return this stub, so further responses can be chained
     */
    public Stub thenReturn(final Object result) {
      return thenAnswer(call -> result);
    }

    /**
     * Answers with a JSON-RPC error, as a node would.
     *
     * @param code the JSON-RPC error code, e.g. {@link JsonRpcErrorCode#INVALID_PARAMS}
     * @param message the error message
     * @return this stub, so further responses can be chained
     */
    public Stub thenFail(final long code, final String message) {
      return thenAnswer(
          call -> {
            throw new PaladinRpcException(code, message, null, 200);
          });
    }

    /**
     * Answers by computing the result from the call.
     *
     * @param answer computes the result; a thrown runtime exception fails the call's future
     * @return this stub, so further responses can be chained
     */
    public Stub thenAnswer(final Function<RecordedCall, Object> answer) {
      synchronized (answers) {
        answers.add(Objects.requireNonNull(answer, "answer"));
      }
      return this;
    }

    private boolean matches(final String calledMethod, final List<JsonNode> params) {
      return method.equals(calledMethod) && matcher.test(params);
    }

    private Function<RecordedCall, Object> nextAnswer() {
      synchronized (answers) {
        if (answers.isEmpty()) {
          throw new PaladinRpcException(
              JsonRpcErrorCode.INTERNAL_ERROR, "stub for " + method + " has no response", null, 0);
        }
        return answers.size() > 1 ? answers.poll() : answers.peek();
      }
    }
  }
}
