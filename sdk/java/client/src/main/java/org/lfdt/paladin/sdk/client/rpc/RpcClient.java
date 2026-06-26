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

import com.fasterxml.jackson.core.type.TypeReference;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.exception.PaladinException;

/**
 * Low-level JSON-RPC transport, mirroring Go's {@code rpcclient.Client}.
 *
 * <p>Every call is asynchronous and returns a {@link CompletableFuture} whose result is the
 * deserialized JSON-RPC {@code result}. Failures complete the future exceptionally with a {@link
 * PaladinException} subtype (observed wrapped in a {@link java.util.concurrent.CompletionException}
 * when the future is joined): {@link org.lfdt.paladin.sdk.client.exception.PaladinRpcException} for
 * a JSON-RPC error or non-2xx status, {@link
 * org.lfdt.paladin.sdk.client.exception.PaladinTimeoutException} for a deadline, and {@link
 * org.lfdt.paladin.sdk.client.exception.PaladinConnectionException} for a transport-level failure.
 *
 * <p>This is the foundation every RPC-namespace client (PTX, KeyManager, …) is built on; namespace
 * clients call {@code callRpc} with the appropriate method name and result type.
 */
public interface RpcClient extends AutoCloseable {

  /**
   * Invokes a JSON-RPC method and binds the {@code result} to the given class.
   *
   * @param resultType the type to deserialize the result into
   * @param method the JSON-RPC method name
   * @param params the positional parameters, serialized through the SDK-wide mapper
   * @param <T> the result type
   * @return a future completing with the deserialized result, or exceptionally with a {@link
   *     PaladinException}
   */
  <T> CompletableFuture<T> callRpc(Class<T> resultType, String method, Object... params);

  /**
   * Invokes a JSON-RPC method and binds the {@code result} to the given generic type, for results
   * that are parameterized (e.g. {@code List<TransactionReceipt>}).
   *
   * @param resultType the generic type to deserialize the result into
   * @param method the JSON-RPC method name
   * @param params the positional parameters, serialized through the SDK-wide mapper
   * @param <T> the result type
   * @return a future completing with the deserialized result, or exceptionally with a {@link
   *     PaladinException}
   */
  <T> CompletableFuture<T> callRpc(TypeReference<T> resultType, String method, Object... params);

  /** Releases any resources held by the transport (connection pool, executor). */
  @Override
  void close();
}
