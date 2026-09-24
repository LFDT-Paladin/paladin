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
 * Thrown when a transaction definition is rejected client-side, before anything reaches the node.
 *
 * <p>This is the deferred-error carrier for the fluent transaction builder: a chaining call that
 * cannot be honoured (an unparseable address, malformed ABI JSON) records one of these instead of
 * throwing, and the builder replays it — along with any structural validation failure, such as a
 * missing transaction type — from {@code build()}, {@code submit()} and {@code send()}.
 *
 * <p>Unlike the transport-level failures in this package it never indicates a problem with the node
 * or the network; the transaction as described could not be assembled at all.
 */
public class PaladinInvalidTransactionException extends PaladinException {

  private static final long serialVersionUID = 1L;

  /**
   * Creates an exception with the given detail message.
   *
   * @param message the detail message
   */
  public PaladinInvalidTransactionException(final String message) {
    super(message);
  }

  /**
   * Creates an exception with the given detail message and underlying cause.
   *
   * @param message the detail message
   * @param cause the underlying cause
   */
  public PaladinInvalidTransactionException(final String message, final Throwable cause) {
    super(message, cause);
  }
}
