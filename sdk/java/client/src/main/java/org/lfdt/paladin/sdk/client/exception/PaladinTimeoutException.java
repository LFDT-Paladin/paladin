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
 * Thrown when a request does not complete within the configured deadline — either the connect timeout
 * while establishing the TCP/TLS connection, or the per-request timeout while awaiting the response.
 *
 * <p>Timeouts are treated as transient by the transport and are retried according to the configured
 * retry policy; this exception surfaces only once retries are exhausted.
 */
public class PaladinTimeoutException extends PaladinException {

    private static final long serialVersionUID = 1L;

    public PaladinTimeoutException(String message) {
        super(message);
    }

    public PaladinTimeoutException(String message, Throwable cause) {
        super(message, cause);
    }
}
