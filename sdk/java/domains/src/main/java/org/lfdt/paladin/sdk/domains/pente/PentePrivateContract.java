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
import java.util.Optional;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.exception.PaladinInvalidTransactionException;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.EntryType;
import org.lfdt.paladin.sdk.core.types.EthAddress;

/**
 * A private contract deployed inside a Pente privacy group, bound to its ABI so functions can be
 * invoked by name. Extend it to give a contract typed methods; override {@link
 * #using(PaladinClient)} so the copy keeps the subclass type.
 *
 * <pre>{@code
 * PentePrivateContract storage = new PentePrivateContract(group, storageAbi, address);
 * storage.sendTransaction("alice@node1", "store", Map.of("num", 42)).waitForReceipt().join();
 * JsonNode value = storage.call("alice@node1", "retrieve", null).join();
 * }</pre>
 */
public class PentePrivateContract {

  private final PentePrivacyGroup group;
  private final List<AbiEntry> abi;
  private final EthAddress address;

  /**
   * Binds a private contract to its ABI.
   *
   * @param group the privacy group the contract is deployed in; must not be {@code null}
   * @param abi the contract ABI; must not be {@code null}
   * @param address the private contract address; must not be {@code null}
   */
  public PentePrivateContract(
      final PentePrivacyGroup group, final List<AbiEntry> abi, final EthAddress address) {
    this.group = Objects.requireNonNull(group, "group");
    this.abi = List.copyOf(Objects.requireNonNull(abi, "abi"));
    this.address = Objects.requireNonNull(address, "address");
  }

  /**
   * The privacy group the contract is deployed in.
   *
   * @return the group
   */
  public PentePrivacyGroup group() {
    return group;
  }

  /**
   * The contract ABI.
   *
   * @return the ABI entries
   */
  public List<AbiEntry> abi() {
    return abi;
  }

  /**
   * The private contract address.
   *
   * @return the address
   */
  public EthAddress address() {
    return address;
  }

  /**
   * Returns a handle on the same contract that submits through another client.
   *
   * @param paladin the client to submit through
   * @return a new contract handle
   */
  public PentePrivateContract using(final PaladinClient paladin) {
    return new PentePrivateContract(group.using(paladin), abi, address);
  }

  /**
   * Submits a transaction invoking a function of the contract.
   *
   * @param from lookup string of the signing identity
   * @param function the function name
   * @param data the function arguments; an empty argument list when {@code null}
   * @return a handle on the submitted transaction; its futures fail with a {@link
   *     PaladinInvalidTransactionException} if the ABI has no such function
   */
  public SentTransaction sendTransaction(
      final String from, final String function, final Object data) {
    return sendTransaction(from, function, data, null);
  }

  /**
   * Submits a transaction invoking a function of the contract, with an idempotency key.
   *
   * @param from lookup string of the signing identity
   * @param function the function name
   * @param data the function arguments; an empty argument list when {@code null}
   * @param idempotencyKey a key unique to this transaction, or {@code null}
   * @return a handle on the submitted transaction; its futures fail with a {@link
   *     PaladinInvalidTransactionException} if the ABI has no such function
   */
  public SentTransaction sendTransaction(
      final String from, final String function, final Object data, final String idempotencyKey) {
    return method(function)
        .map(m -> group.sendTransaction(params(from, m, data), idempotencyKey))
        .orElseGet(
            () ->
                SentTransaction.track(
                    group.paladin().ptx(), CompletableFuture.failedFuture(notFound(function))));
  }

  /**
   * Makes a read-only call to a function of the contract.
   *
   * @param from lookup string of the calling identity; optional
   * @param function the function name
   * @param data the function arguments; an empty argument list when {@code null}
   * @return a future completing with the decoded result, or failing with a {@link
   *     PaladinInvalidTransactionException} if the ABI has no such function
   */
  public CompletableFuture<JsonNode> call(
      final String from, final String function, final Object data) {
    return method(function)
        .map(m -> group.call(params(from, m, data)))
        .orElseGet(() -> CompletableFuture.failedFuture(notFound(function)));
  }

  private Optional<AbiEntry> method(final String function) {
    return abi.stream()
        .filter(entry -> entry.type() == EntryType.FUNCTION)
        .filter(entry -> entry.name().equals(function))
        .findFirst();
  }

  private PenteGroupTransactionParams params(
      final String from, final AbiEntry method, final Object data) {
    return new PenteGroupTransactionParams(from, address, method, data == null ? List.of() : data);
  }

  private static PaladinInvalidTransactionException notFound(final String function) {
    return new PaladinInvalidTransactionException("method '" + function + "' not found");
  }
}
