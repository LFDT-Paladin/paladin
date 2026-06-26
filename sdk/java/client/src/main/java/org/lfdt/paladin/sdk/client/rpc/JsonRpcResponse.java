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
 * A JSON-RPC 2.0 response envelope, mirroring Go's {@code rpcclient.RPCResponse}.
 *
 * <p>Immutable. Exactly one of {@link #result()} or {@link #error()} is meaningful for a
 * well-formed response; use {@link #hasError()} to discriminate. The raw {@link #result()} node is
 * left undecoded so the transport can bind it to the caller's requested result type. The {@code id}
 * is kept as a raw node because a node may echo it back as either a string or a number.
 */
@JsonPropertyOrder({"jsonrpc", "id", "result", "error"})
public final class JsonRpcResponse {

  private final String jsonrpc;
  private final JsonNode id;
  private final JsonNode result;
  private final JsonRpcError error;

  @JsonCreator
  JsonRpcResponse(
      @JsonProperty("jsonrpc") String jsonrpc,
      @JsonProperty("id") JsonNode id,
      @JsonProperty("result") JsonNode result,
      @JsonProperty("error") JsonRpcError error) {
    this.jsonrpc = jsonrpc;
    this.id = id;
    this.result = result;
    this.error = error;
  }

  /**
   * The JSON-RPC protocol version reported by the node.
   *
   * @return the protocol version
   */
  @JsonProperty("jsonrpc")
  public String jsonrpc() {
    return jsonrpc;
  }

  /**
   * The request id echoed back by the node, as a raw node (string or number).
   *
   * @return the request id node
   */
  @JsonProperty("id")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode id() {
    return id;
  }

  /**
   * The raw result node on success, or {@code null} when the response carried an error.
   *
   * @return the raw result node, or {@code null}
   */
  @JsonProperty("result")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonNode result() {
    return result;
  }

  /**
   * The error member on failure, or {@code null} on success.
   *
   * @return the error member, or {@code null}
   */
  @JsonProperty("error")
  @JsonInclude(JsonInclude.Include.NON_NULL)
  public JsonRpcError error() {
    return error;
  }

  /**
   * Whether this response carries a JSON-RPC {@code error} member.
   *
   * @return {@code true} if an error member is present
   */
  public boolean hasError() {
    return error != null;
  }
}
