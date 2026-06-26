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
import java.util.List;

/**
 * A JSON-RPC 2.0 request envelope, mirroring Go's {@code rpcclient.RPCRequest}.
 *
 * <p>Immutable and self-serializing. The {@code params} list holds arbitrary argument objects that are
 * serialized through the SDK-wide mapper, so SDK value types serialize through their own representation.
 * Following Go's {@code omitempty} convention, {@code params} is omitted from the wire form when empty.
 */
@JsonPropertyOrder({"jsonrpc", "id", "method", "params"})
public final class JsonRpcRequest {

    /** The JSON-RPC protocol version string, always {@code "2.0"}. */
    public static final String JSON_RPC_VERSION = "2.0";

    private final String jsonrpc;
    private final String id;
    private final String method;
    private final List<Object> params;

    @JsonCreator
    JsonRpcRequest(
            @JsonProperty("jsonrpc") String jsonrpc,
            @JsonProperty("id") String id,
            @JsonProperty("method") String method,
            @JsonProperty("params") List<Object> params) {
        this.jsonrpc = jsonrpc == null ? JSON_RPC_VERSION : jsonrpc;
        this.id = id;
        this.method = method;
        this.params = params == null ? List.of() : List.copyOf(params);
    }

    /**
     * Builds a request with the protocol version defaulted to {@code "2.0"}.
     *
     * @param id the request id (the transport allocates a monotonic, zero-padded value)
     * @param method the JSON-RPC method name
     * @param params the positional parameters; may be empty
     */
    public JsonRpcRequest(String id, String method, List<Object> params) {
        this(JSON_RPC_VERSION, id, method, params);
    }

    /** The JSON-RPC protocol version, always {@code "2.0"}. */
    @JsonProperty("jsonrpc")
    public String jsonrpc() {
        return jsonrpc;
    }

    /** The request id echoed back by the node in its response. */
    @JsonProperty("id")
    public String id() {
        return id;
    }

    /** The JSON-RPC method name. */
    @JsonProperty("method")
    public String method() {
        return method;
    }

    /** The positional parameters; never null, omitted from the wire form when empty. */
    @JsonProperty("params")
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public List<Object> params() {
        return params;
    }
}
