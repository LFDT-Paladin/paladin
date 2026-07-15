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

/**
 * Client for the {@code keymgr_*} RPC namespace (key manager), mirroring Go's {@code
 * pldclient.KeyManager} ({@code sdk/go/pkg/pldclient/keymgr.go}).
 *
 * <p>Each method maps one-to-one to a JSON-RPC call on the underlying {@link RpcClient} and returns
 * a {@link CompletableFuture}; failures complete it exceptionally with a {@code PaladinException}
 * subtype.
 */
public final class KeyManagerClient {

  private final RpcClient rpc;

  /**
   * Creates a client over the given RPC transport.
   *
   * @param rpc the RPC client used to make calls; must not be {@code null}
   */
  public KeyManagerClient(final RpcClient rpc) {
    this.rpc = Objects.requireNonNull(rpc, "rpc");
  }

  /**
   * Lists the available wallets ({@code keymgr_wallets}).
   *
   * @return a future completing with the wallet names
   */
  public CompletableFuture<List<String>> wallets() {
    return rpc.callRpc(new TypeReference<List<String>>() {}, "keymgr_wallets");
  }

  /**
   * Resolves a key identifier to a key mapping and verifier ({@code keymgr_resolveKey}).
   *
   * @param keyIdentifier the key identifier to resolve
   * @param algorithm the signing algorithm to resolve the verifier under
   * @param verifierType the verifier type to produce
   * @return a future completing with the resolved key mapping and verifier
   */
  public CompletableFuture<KeyMappingAndVerifier> resolveKey(
      final String keyIdentifier, final String algorithm, final String verifierType) {
    return rpc.callRpc(
        KeyMappingAndVerifier.class, "keymgr_resolveKey", keyIdentifier, algorithm, verifierType);
  }

  /**
   * Resolves a key identifier to an Ethereum address ({@code keymgr_resolveEthAddress}).
   *
   * @param keyIdentifier the key identifier to resolve
   * @return a future completing with the resolved Ethereum address
   */
  public CompletableFuture<EthAddress> resolveEthAddress(final String keyIdentifier) {
    return rpc.callRpc(EthAddress.class, "keymgr_resolveEthAddress", keyIdentifier);
  }

  /**
   * Looks up the key mapping that produced a given verifier ({@code keymgr_reverseKeyLookup}).
   *
   * @param algorithm the signing algorithm the verifier was produced under
   * @param verifierType the verifier type
   * @param verifier the verifier value to look up
   * @return a future completing with the matching key mapping and verifier
   */
  public CompletableFuture<KeyMappingAndVerifier> reverseKeyLookup(
      final String algorithm, final String verifierType, final String verifier) {
    return rpc.callRpc(
        KeyMappingAndVerifier.class, "keymgr_reverseKeyLookup", algorithm, verifierType, verifier);
  }

  /**
   * Queries the key store ({@code keymgr_queryKeys}).
   *
   * @param query the query to run
   * @return a future completing with the matching key entries
   */
  public CompletableFuture<List<KeyQueryEntry>> queryKeys(final QueryJSON query) {
    return rpc.callRpc(new TypeReference<List<KeyQueryEntry>>() {}, "keymgr_queryKeys", query);
  }

  /**
   * Signs a payload with the identified key ({@code keymgr_sign}).
   *
   * @param keyIdentifier the key identifier to sign with
   * @param algorithm the signing algorithm
   * @param verifierType the verifier type
   * @param payloadType the payload type
   * @param payload the payload to sign
   * @return a future completing with the signature
   */
  public CompletableFuture<HexBytes> sign(
      final String keyIdentifier,
      final String algorithm,
      final String verifierType,
      final String payloadType,
      final HexBytes payload) {
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
