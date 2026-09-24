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
package org.lfdt.paladin.sdk.client.transport;

import com.fasterxml.jackson.core.type.TypeReference;
import java.util.List;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.transport.PeerInfo;
import org.lfdt.paladin.sdk.core.transport.ReliableMessage;
import org.lfdt.paladin.sdk.core.transport.ReliableMessageAck;

/**
 * Client for the {@code transport_*} RPC namespace (peer transport).
 *
 * <p>Each method maps one-to-one to a JSON-RPC call on the underlying {@link RpcClient} and returns
 * a {@link CompletableFuture}; failures complete it exceptionally with a {@code PaladinException}
 * subtype.
 */
public final class TransportClient {

  private final RpcClient rpc;

  /**
   * Creates a client over the given RPC transport.
   *
   * @param rpc the RPC client used to make calls; must not be {@code null}
   */
  public TransportClient(final RpcClient rpc) {
    this.rpc = Objects.requireNonNull(rpc, "rpc");
  }

  /**
   * Returns the name of this node ({@code transport_nodeName}).
   *
   * @return a future completing with this node's name
   */
  public CompletableFuture<String> nodeName() {
    return rpc.callRpc(String.class, "transport_nodeName");
  }

  /**
   * Lists the names of the transports configured on this node ({@code transport_localTransports}).
   *
   * @return a future completing with the local transport names
   */
  public CompletableFuture<List<String>> localTransports() {
    return rpc.callRpc(new TypeReference<List<String>>() {}, "transport_localTransports");
  }

  /**
   * Returns the connection details other nodes use to reach a local transport ({@code
   * transport_localTransportDetails}).
   *
   * @param transportName the local transport to describe
   * @return a future completing with the transport's connection details
   */
  public CompletableFuture<String> localTransportDetails(final String transportName) {
    return rpc.callRpc(String.class, "transport_localTransportDetails", transportName);
  }

  /**
   * Lists the peers this node is currently connected to ({@code transport_peers}).
   *
   * @return a future completing with the peer information
   */
  public CompletableFuture<List<PeerInfo>> peers() {
    return rpc.callRpc(new TypeReference<List<PeerInfo>>() {}, "transport_peers");
  }

  /**
   * Returns information about a single peer ({@code transport_peerInfo}).
   *
   * @param nodeName the peer node name to describe
   * @return a future completing with the peer information
   */
  public CompletableFuture<PeerInfo> peerInfo(final String nodeName) {
    return rpc.callRpc(PeerInfo.class, "transport_peerInfo", nodeName);
  }

  /**
   * Queries the reliable-message queue ({@code transport_queryReliableMessages}).
   *
   * @param query the query to run
   * @return a future completing with the matching reliable messages
   */
  public CompletableFuture<List<ReliableMessage>> queryReliableMessages(final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<ReliableMessage>>() {}, "transport_queryReliableMessages", query);
  }

  /**
   * Queries the acknowledgements of reliable messages ({@code transport_queryReliableMessageAcks}).
   *
   * @param query the query to run
   * @return a future completing with the matching acknowledgements
   */
  public CompletableFuture<List<ReliableMessageAck>> queryReliableMessageAcks(
      final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<ReliableMessageAck>>() {},
        "transport_queryReliableMessageAcks",
        query);
  }
}
