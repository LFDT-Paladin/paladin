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
package org.lfdt.paladin.sdk.domains.pente;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.client.tx.TxBuilder;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.EntryType;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroup;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

/**
 * A deployed Pente privacy group: deploys private contracts into the group, submits transactions to
 * them, and makes read-only calls against them.
 *
 * <pre>{@code
 * EthAddress contract =
 *     group.deploy(new PenteDeployParams(abi, bytecode, "alice@node1", Map.of())).waitForDeploy().join();
 * group.sendTransaction(new PenteGroupTransactionParams("alice@node1", contract, setAbi, Map.of("x", 1)))
 *     .waitForReceipt()
 *     .join();
 * }</pre>
 */
public final class PentePrivacyGroup {

  /** The signing identity used for {@link #call} when none is supplied. */
  public static final String DEFAULT_CALL_FROM = "sdk.default";

  private final PaladinClient paladin;
  private final PrivacyGroup group;
  private final EthAddress address;

  /**
   * Wraps a privacy group the node has already deployed.
   *
   * @param paladin the client used to submit transactions; must not be {@code null}
   * @param group the group; must not be {@code null}
   * @throws IllegalArgumentException if the group has no contract address yet
   */
  public PentePrivacyGroup(final PaladinClient paladin, final PrivacyGroup group) {
    this(paladin, group, requireAddress(group));
  }

  PentePrivacyGroup(
      final PaladinClient paladin, final PrivacyGroup group, final EthAddress address) {
    this.paladin = Objects.requireNonNull(paladin, "paladin");
    this.group = Objects.requireNonNull(group, "group");
    this.address = address;
  }

  private static EthAddress requireAddress(final PrivacyGroup group) {
    Objects.requireNonNull(group, "group");
    if (group.contractAddress() == null) {
      throw new IllegalArgumentException(
          "privacy group "
              + group.id()
              + " is missing a contract address; check transaction "
              + group.genesisTransaction());
    }
    return group.contractAddress();
  }

  /**
   * The underlying privacy group record.
   *
   * @return the group
   */
  public PrivacyGroup group() {
    return group;
  }

  /**
   * The base-ledger address of the group's privacy contract.
   *
   * @return the group contract address
   */
  public EthAddress address() {
    return address;
  }

  /**
   * The group identifier, which is also the salt to use when building Pente private transactions
   * directly.
   *
   * @return the group identifier
   */
  public HexBytes id() {
    return group.id();
  }

  /**
   * The name of the Pente domain the group belongs to.
   *
   * @return the domain name
   */
  public String domain() {
    return group.domain();
  }

  /**
   * Lookup strings of the identities in the group.
   *
   * @return the group members
   */
  public List<String> members() {
    return group.members();
  }

  /**
   * Returns a handle on the same group that submits through another client, for example a different
   * member's node.
   *
   * @param paladin the client to submit through
   * @return a new group handle
   */
  public PentePrivacyGroup using(final PaladinClient paladin) {
    return new PentePrivacyGroup(paladin, group, address);
  }

  /**
   * Deploys a private contract into the group.
   *
   * @param params the contract and its constructor arguments
   * @return a handle on the in-flight deployment
   */
  public PentePrivateDeployment deploy(final PenteDeployParams params) {
    return deploy(params, null);
  }

  /**
   * Deploys a private contract into the group with an idempotency key, so that a retried submission
   * is not deployed twice.
   *
   * @param params the contract and its constructor arguments
   * @param idempotencyKey a key unique to this deployment, or {@code null}
   * @return a handle on the in-flight deployment
   */
  public PentePrivateDeployment deploy(
      final PenteDeployParams params, final String idempotencyKey) {
    Objects.requireNonNull(params, "params");
    final AbiEntry constructor =
        params.abi().stream()
            .filter(entry -> entry.type() == EntryType.CONSTRUCTOR)
            .findFirst()
            .orElseGet(() -> AbiEntry.constructor().build());
    final TxBuilder tx =
        newTx()
            .from(params.from())
            .abiEntry(constructor)
            .bytecode(params.bytecode())
            .idempotencyKey(idempotencyKey);
    if (params.inputs() != null) {
      tx.inputs(params.inputs());
    }
    return new PentePrivateDeployment(paladin.ptx(), tx.send());
  }

  /**
   * Submits a transaction to a private contract in the group.
   *
   * @param params the function invocation
   * @return a handle on the submitted transaction
   */
  public SentTransaction sendTransaction(final PenteGroupTransactionParams params) {
    return sendTransaction(params, null);
  }

  /**
   * Submits a transaction to a private contract in the group with an idempotency key.
   *
   * @param params the function invocation
   * @param idempotencyKey a key unique to this transaction, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction sendTransaction(
      final PenteGroupTransactionParams params, final String idempotencyKey) {
    Objects.requireNonNull(params, "params");
    return invoke(params, params.from()).idempotencyKey(idempotencyKey).send();
  }

  /**
   * Makes a read-only call to a private contract in the group, without submitting a transaction.
   * Calls without a signing identity use {@link #DEFAULT_CALL_FROM}.
   *
   * @param params the function invocation
   * @return a future completing with the decoded result
   */
  public CompletableFuture<JsonNode> call(final PenteGroupTransactionParams params) {
    Objects.requireNonNull(params, "params");
    final String from =
        params.from() == null || params.from().isBlank() ? DEFAULT_CALL_FROM : params.from();
    return invoke(params, from).call();
  }

  PaladinClient paladin() {
    return paladin;
  }

  private TxBuilder newTx() {
    return paladin.newTx().privacyGroup(group);
  }

  private TxBuilder invoke(final PenteGroupTransactionParams params, final String from) {
    final TxBuilder tx =
        newTx()
            .from(from)
            .to(params.to())
            .abiEntry(params.methodAbi())
            .function(params.methodAbi().name());
    if (params.data() != null) {
      tx.inputs(params.data());
    }
    return tx;
  }
}
