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
 * Thrown when the request never reached the node, or no usable response came back, due to a
 * transport-level failure: connection refused, DNS resolution failure, socket reset, TLS handshake
 * failure, or a malformed/unparseable response body.
 *
 * <p>Like timeouts, connection failures are treated as transient and retried per the configured retry
 * policy; this exception surfaces only once retries are exhausted.
 */
public class PaladinConnectionException extends PaladinException {

    private static final long serialVersionUID = 1L;

    public PaladinConnectionException(String message) {
        super(message);
    }

    public PaladinConnectionException(String message, Throwable cause) {
        super(message, cause);
    }
}
