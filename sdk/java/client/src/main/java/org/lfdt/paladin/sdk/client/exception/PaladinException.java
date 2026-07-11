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
package org.lfdt.paladin.sdk.client.exception;

/**
 * Base of the Paladin SDK's <em>unchecked</em> exception hierarchy.
 *
 * <p>Every failure surfaced by the transport layer is a subclass of this type, so callers can catch
 * {@code PaladinException} to handle everything the SDK throws without being forced to declare
 * checked exceptions on each call. Because the public API is asynchronous, these are most often
 * observed wrapped in a {@link java.util.concurrent.CompletionException} when a {@link
 * java.util.concurrent.CompletableFuture} is joined — unwrap with {@link Throwable#getCause()} to
 * recover the concrete subtype:
 *
 * <ul>
 *   <li>{@link PaladinRpcException} — the node returned a JSON-RPC error object, or a non-2xx HTTP
 *       status.
 *   <li>{@link PaladinTimeoutException} — the connect or request deadline elapsed.
 *   <li>{@link PaladinConnectionException} — the request never reached the node (connection
 *       refused, DNS failure, socket reset, or other transport-level I/O error).
 * </ul>
 */
public class PaladinException extends RuntimeException {

  private static final long serialVersionUID = 1L;

  /**
   * Creates an exception with the given detail message.
   *
   * @param message the detail message
   */
  public PaladinException(final String message) {
    super(message);
  }

  /**
   * Creates an exception with the given detail message and underlying cause.
   *
   * @param message the detail message
   * @param cause the underlying cause
   */
  public PaladinException(final String message, final Throwable cause) {
    super(message, cause);
  }
}
