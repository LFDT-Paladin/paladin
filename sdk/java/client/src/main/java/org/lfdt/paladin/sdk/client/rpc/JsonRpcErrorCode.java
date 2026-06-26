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

/**
 * JSON-RPC 2.0 error codes used by Paladin, mirroring Go's {@code rpcclient.RPCCode} constants.
 *
 * <p>The {@code -32700}..{@code -32600} and {@code -32603} values are the protocol-reserved codes
 * from the JSON-RPC 2.0 specification. The {@code -32000}..{@code -32099} band is reserved by the
 * spec for "implementation-defined server errors"; Paladin uses {@link #UNAUTHORIZED} from that
 * band for authentication failures.
 */
public final class JsonRpcErrorCode {

  /** Invalid JSON was received by the server ({@code -32700}). */
  public static final long PARSE_ERROR = -32700;

  /** The JSON sent is not a valid request object ({@code -32600}). */
  public static final long INVALID_REQUEST = -32600;

  /** The requested method does not exist or is not available ({@code -32601}). */
  public static final long METHOD_NOT_FOUND = -32601;

  /** Invalid method parameters ({@code -32602}). */
  public static final long INVALID_PARAMS = -32602;

  /** Internal JSON-RPC error ({@code -32603}). */
  public static final long INTERNAL_ERROR = -32603;

  /** Paladin application error: the request failed authentication ({@code -32000}). */
  public static final long UNAUTHORIZED = -32000;

  private JsonRpcErrorCode() {}
}
