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
package org.lfdt.paladin.sdk.testing.wiremock;

import static com.github.tomakehurst.wiremock.client.WireMock.aResponse;
import static com.github.tomakehurst.wiremock.client.WireMock.anyUrl;
import static com.github.tomakehurst.wiremock.client.WireMock.equalTo;
import static com.github.tomakehurst.wiremock.client.WireMock.equalToJson;
import static com.github.tomakehurst.wiremock.client.WireMock.matchingJsonPath;
import static com.github.tomakehurst.wiremock.client.WireMock.post;
import static com.github.tomakehurst.wiremock.client.WireMock.postRequestedFor;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.github.tomakehurst.wiremock.client.MappingBuilder;
import com.github.tomakehurst.wiremock.client.ResponseDefinitionBuilder;
import com.github.tomakehurst.wiremock.matching.RequestPatternBuilder;
import java.util.Arrays;
import java.util.List;
import java.util.Locale;
import java.util.UUID;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;

/**
 * WireMock request matchers and responses for a Paladin node's HTTP JSON-RPC endpoint.
 *
 * <p>Every JSON-RPC call is a {@code POST} to the same URL, so stubs are told apart by the {@code
 * method} (and optionally {@code params}) in the request body. Responses echo the request's {@code
 * id} through WireMock response templating.
 *
 * <pre>{@code
 * wireMock.stubFor(rpc("ptx_sendTransaction").willReturn(result(txId)));
 * wireMock.stubFor(rpc("ptx_getTransactionReceipt", txId).willReturn(result(receipt)));
 * wireMock.stubFor(rpc("ptx_getTransaction").willReturn(error(-32602, "bad id")));
 * wireMock.verify(1, called("ptx_sendTransaction"));
 * }</pre>
 *
 * <p>The named helpers ({@link #sendTransaction}, {@link #transactionReceipt}, ...) cover the
 * common send-and-wait happy path; anything else is a {@link #rpc(String)} plus {@link
 * #result(Object)}.
 */
public final class PaladinRpcStubs {

  private static final ObjectMapper MAPPER = PaladinObjectMapper.shared();
  private static final String RESPONSE_TEMPLATE = "response-template";
  private static final String ECHO_ID = "{{jsonPath request.body '$.id'}}";

  private PaladinRpcStubs() {}

  /**
   * Matches any call to {@code method}.
   *
   * @param method the JSON-RPC method name
   * @return a WireMock mapping to complete with {@code willReturn(...)}
   */
  public static MappingBuilder rpc(final String method) {
    return post(anyUrl()).withRequestBody(matchingJsonPath("$.method", equalTo(method)));
  }

  /**
   * Matches calls to {@code method} with exactly these positional parameters, compared as JSON.
   *
   * @param method the JSON-RPC method name
   * @param params the expected parameters
   * @return a WireMock mapping to complete with {@code willReturn(...)}
   */
  public static MappingBuilder rpc(final String method, final Object... params) {
    return rpc(method)
        .withRequestBody(matchingJsonPath("$.params", equalToJson(paramsJson(params))));
  }

  /**
   * Matches the requests a call to {@code method} made, for {@code WireMock.verify(...)}.
   *
   * @param method the JSON-RPC method name
   * @return a request pattern
   */
  public static RequestPatternBuilder called(final String method) {
    return postRequestedFor(anyUrl())
        .withRequestBody(matchingJsonPath("$.method", equalTo(method)));
  }

  /**
   * Matches the requests a call to {@code method} with these parameters made.
   *
   * @param method the JSON-RPC method name
   * @param params the expected parameters
   * @return a request pattern
   */
  public static RequestPatternBuilder called(final String method, final Object... params) {
    return called(method)
        .withRequestBody(matchingJsonPath("$.params", equalToJson(paramsJson(params))));
  }

  /**
   * A successful JSON-RPC response carrying {@code result}, serialized with the SDK's mapper.
   *
   * @param result the result; {@code null} gives a JSON {@code null} result
   * @return a WireMock response
   */
  public static ResponseDefinitionBuilder result(final Object result) {
    final ObjectNode body = envelope();
    body.set("result", MAPPER.valueToTree(result));
    return respond(body);
  }

  /**
   * A JSON-RPC error response. Like a node, it uses HTTP {@code 200}: the error is in the body.
   *
   * @param code the JSON-RPC error code
   * @param message the error message
   * @return a WireMock response
   */
  public static ResponseDefinitionBuilder error(final long code, final String message) {
    final ObjectNode body = envelope();
    body.putObject("error").put("code", code).put("message", message);
    return respond(body);
  }

  /**
   * {@code ptx_sendTransaction} accepting any transaction and returning {@code id}.
   *
   * @param id the transaction id the node assigns
   * @return a complete WireMock mapping
   */
  public static MappingBuilder sendTransaction(final UUID id) {
    return rpc("ptx_sendTransaction").willReturn(result(id));
  }

  /**
   * {@code ptx_getTransactionReceipt} for the receipt's own id, returning that receipt.
   *
   * @param receipt the receipt to return; its {@code id} selects the request
   * @return a complete WireMock mapping
   */
  public static MappingBuilder transactionReceipt(final TransactionReceipt receipt) {
    return rpc("ptx_getTransactionReceipt", receipt.id()).willReturn(result(receipt));
  }

  /**
   * {@code ptx_getTransactionReceipt} for {@code id} returning {@code null}: still in flight.
   *
   * @param id the transaction id
   * @return a complete WireMock mapping
   */
  public static MappingBuilder receiptPending(final UUID id) {
    return rpc("ptx_getTransactionReceipt", id).willReturn(result(null));
  }

  /**
   * {@code ptx_resolveVerifier} for one key, algorithm and verifier type.
   *
   * @param keyIdentifier the key, e.g. {@code alice@node1}
   * @param algorithm the algorithm, e.g. {@code ecdsa:secp256k1}
   * @param verifierType the verifier type, e.g. {@code eth_address}
   * @param verifier the resolved verifier to return
   * @return a complete WireMock mapping
   */
  public static MappingBuilder resolveVerifier(
      final String keyIdentifier,
      final String algorithm,
      final String verifierType,
      final String verifier) {
    return rpc("ptx_resolveVerifier", keyIdentifier, algorithm, verifierType)
        .willReturn(result(verifier));
  }

  /**
   * {@code ptx_call} returning {@code output} for any call.
   *
   * @param output the decoded call output
   * @return a complete WireMock mapping
   */
  public static MappingBuilder call(final Object output) {
    return rpc("ptx_call").willReturn(result(output));
  }

  /**
   * {@code bidx_getConfirmedBlockHeight} returning {@code height}.
   *
   * @param height the confirmed block height
   * @return a complete WireMock mapping
   */
  public static MappingBuilder confirmedBlockHeight(final long height) {
    return rpc("bidx_getConfirmedBlockHeight")
        .willReturn(result(String.format(Locale.ROOT, "0x%x", height)));
  }

  private static ObjectNode envelope() {
    final ObjectNode body = MAPPER.createObjectNode();
    body.put("jsonrpc", "2.0");
    body.put("id", ECHO_ID);
    return body;
  }

  private static ResponseDefinitionBuilder respond(final ObjectNode body) {
    return aResponse()
        .withStatus(200)
        .withHeader("Content-Type", "application/json")
        .withBody(body.toString())
        .withTransformers(RESPONSE_TEMPLATE);
  }

  private static String paramsJson(final Object[] params) {
    final List<Object> list = params == null ? List.of() : Arrays.asList(params);
    try {
      return MAPPER.writeValueAsString(list);
    } catch (final JsonProcessingException e) {
      throw new IllegalArgumentException("parameters are not serializable: " + e.getMessage(), e);
    }
  }
}
