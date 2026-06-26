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

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import com.fasterxml.jackson.databind.JsonNode;

/**
 * The {@code error} member of a JSON-RPC 2.0 response, mirroring Go's {@code rpcclient.RPCError}.
 *
 * <p>Immutable and self-serializing. {@link #data()} carries any structured detail the node attached and is
 * omitted from the wire form when absent. See {@link JsonRpcErrorCode} for the codes Paladin uses.
 */
@JsonPropertyOrder({"code", "message", "data"})
public final class JsonRpcError {

    private final long code;
    private final String message;
    private final JsonNode data;

    @JsonCreator
    public JsonRpcError(
            @JsonProperty("code") long code,
            @JsonProperty("message") String message,
            @JsonProperty("data") JsonNode data) {
        this.code = code;
        this.message = message;
        this.data = data;
    }

    /** The JSON-RPC error code; see {@link JsonRpcErrorCode}. */
    @JsonProperty("code")
    public long code() {
        return code;
    }

    /** The human-readable error message. */
    @JsonProperty("message")
    public String message() {
        return message;
    }

    /** Structured error detail attached by the node, or {@code null} if none. */
    @JsonProperty("data")
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public JsonNode data() {
        return data;
    }
}
