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

import com.fasterxml.jackson.databind.JsonNode;
import java.util.Optional;

/**
 * Thrown when the node accepted and answered the request but the answer was an error: either a
 * JSON-RPC {@code error} object in a 200 response, or a non-2xx HTTP status.
 *
 * <p>For a JSON-RPC error the {@link #code()} is the protocol code (e.g. {@code -32700} parse error,
 * {@code -32603} internal error — see the reserved range) and {@link #data()} carries any structured
 * detail the node attached. For a transport-level non-2xx with no JSON-RPC body, {@code code} is
 * {@code 0} and only {@link #httpStatus()} is meaningful. {@link #httpStatus()} is always the HTTP
 * status of the response that produced this error.
 */
public class PaladinRpcException extends PaladinException {

    private static final long serialVersionUID = 1L;

    private final long code;
    private final transient JsonNode data;
    private final int httpStatus;

    public PaladinRpcException(long code, String message, JsonNode data, int httpStatus) {
        super(message);
        this.code = code;
        this.data = data;
        this.httpStatus = httpStatus;
    }

    /** The JSON-RPC error code, or {@code 0} if the failure was a bare non-2xx HTTP status. */
    public long code() {
        return code;
    }

    /** Structured error detail attached by the node ({@code error.data}), if any. */
    public Optional<JsonNode> data() {
        return Optional.ofNullable(data);
    }

    /** The HTTP status code of the response that produced this error. */
    public int httpStatus() {
        return httpStatus;
    }
}
