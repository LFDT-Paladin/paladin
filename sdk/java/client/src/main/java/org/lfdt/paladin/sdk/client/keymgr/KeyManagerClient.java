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

package org.lfdt.paladin.sdk.client.keymgr;

import com.fasterxml.jackson.core.type.TypeReference;
import java.util.List;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.core.key.KeyMappingAndVerifier;
import org.lfdt.paladin.sdk.core.key.KeyQueryEntry;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

// Client for the keymgr_* RPC namespace, mirroring Go's pldclient.KeyManager (sdk/go/pkg/pldclient
// /keymgr.go). Each method maps one-to-one to a JSON-RPC call on the underlying RpcClient and
// returns a CompletableFuture; failures complete it exceptionally with a PaladinException subtype.
public final class KeyManagerClient {

  private final RpcClient rpc;

  public KeyManagerClient(RpcClient rpc) {
    this.rpc = Objects.requireNonNull(rpc, "rpc");
  }

  public CompletableFuture<List<String>> wallets() {
    return rpc.callRpc(new TypeReference<List<String>>() {}, "keymgr_wallets");
  }

  public CompletableFuture<KeyMappingAndVerifier> resolveKey(
      String keyIdentifier, String algorithm, String verifierType) {
    return rpc.callRpc(
        KeyMappingAndVerifier.class,
        "keymgr_resolveKey",
        keyIdentifier,
        algorithm,
        verifierType);
  }

  public CompletableFuture<EthAddress> resolveEthAddress(String keyIdentifier) {
    return rpc.callRpc(EthAddress.class, "keymgr_resolveEthAddress", keyIdentifier);
  }

  public CompletableFuture<KeyMappingAndVerifier> reverseKeyLookup(
      String algorithm, String verifierType, String verifier) {
    return rpc.callRpc(
        KeyMappingAndVerifier.class,
        "keymgr_reverseKeyLookup",
        algorithm,
        verifierType,
        verifier);
  }

  public CompletableFuture<List<KeyQueryEntry>> queryKeys(QueryJSON query) {
    return rpc.callRpc(new TypeReference<List<KeyQueryEntry>>() {}, "keymgr_queryKeys", query);
  }

  public CompletableFuture<HexBytes> sign(
      String keyIdentifier,
      String algorithm,
      String verifierType,
      String payloadType,
      HexBytes payload) {
    return rpc.callRpc(
        HexBytes.class,
        "keymgr_sign",
        keyIdentifier,
        algorithm,
        verifierType,
        payloadType,
        payload);
  }
}
