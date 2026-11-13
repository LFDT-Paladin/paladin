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
package org.lfdt.paladin.sdk.domains.zeto;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.client.tx.TxBuilder;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.types.EthAddress;

/**
 * A deployed Zeto token. Each operation submits one transaction and returns its {@link
 * SentTransaction} handle; wait for the outcome with {@link SentTransaction#waitForReceipt()}:
 *
 * <pre>{@code
 * TransactionReceipt receipt =
 *     token
 *         .transfer(
 *             "alice",
 *             new ZetoTransferParams(
 *                 List.of(new ZetoTransfer("bob", HexUint256.of(10), null))),
 *             Optional.empty())
 *         .waitForReceipt()
 *         .join();
 * }</pre>
 *
 * <p>Private operations are submitted through the Zeto domain; {@link #delegateLock} is a public
 * transaction against the base-ledger contract. {@link #balanceOf} is a call that submits nothing.
 *
 * <p>Instances are immutable and thread-safe except that {@link #setErc20} updates the remembered
 * ERC-20 address on this handle.
 */
public final class ZetoInstance {

  /** The name the Zeto domain is registered under by default. */
  public static final String DEFAULT_DOMAIN = "zeto";

  private final PaladinClient paladin;
  private final String domain;
  private final EthAddress address;
  private EthAddress erc20;

  /**
   * Binds to a token deployed through the {@value #DEFAULT_DOMAIN} domain.
   *
   * @param paladin the client used to submit transactions; must not be {@code null}
   * @param address the address of the token contract; must not be {@code null}
   */
  public ZetoInstance(final PaladinClient paladin, final EthAddress address) {
    this(paladin, DEFAULT_DOMAIN, address);
  }

  /**
   * Binds to a token deployed through the named domain.
   *
   * @param paladin the client used to submit transactions; must not be {@code null}
   * @param domain the name the Zeto domain is registered under; must not be {@code null}
   * @param address the address of the token contract; must not be {@code null}
   */
  public ZetoInstance(final PaladinClient paladin, final String domain, final EthAddress address) {
    this.paladin = Objects.requireNonNull(paladin, "paladin");
    this.domain = Objects.requireNonNull(domain, "domain");
    this.address = Objects.requireNonNull(address, "address");
  }

  private ZetoInstance(
      final PaladinClient paladin,
      final String domain,
      final EthAddress address,
      final EthAddress erc20) {
    this(paladin, domain, address);
    this.erc20 = erc20;
  }

  /**
   * The address of the token contract.
   *
   * @return the contract address
   */
  public EthAddress address() {
    return address;
  }

  /**
   * The name of the Zeto domain the token was deployed through.
   *
   * @return the domain name
   */
  public String domain() {
    return domain;
  }

  /**
   * The ERC-20 contract last set through {@link #setErc20}, if any.
   *
   * @return the bridged ERC-20 address
   */
  public Optional<EthAddress> erc20() {
    return Optional.ofNullable(erc20);
  }

  /**
   * Returns a handle on the same token that submits through another client, for example a different
   * node.
   *
   * @param paladin the client to submit through
   * @return a new instance
   */
  public ZetoInstance using(final PaladinClient paladin) {
    return new ZetoInstance(paladin, domain, address, erc20);
  }

  /**
   * Mints new Zeto UTXOs. The idempotency key prevents a retried submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@link Optional#empty()}
   * @return a handle on the submitted transaction
   */
  public SentTransaction mint(
      final String from, final ZetoMintParams params, final Optional<String> idempotencyKey) {
    return sendPrivate("mint", from, params, idempotencyKey);
  }

  /**
   * Transfers the sender's UTXOs to recipients. The idempotency key prevents a retried submission
   * from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@link Optional#empty()}
   * @return a handle on the submitted transaction
   */
  public SentTransaction transfer(
      final String from, final ZetoTransferParams params, final Optional<String> idempotencyKey) {
    return sendPrivate("transfer", from, params, idempotencyKey);
  }

  /**
   * Spends locked UTXOs and creates new outputs. The idempotency key prevents a retried submission
   * from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@link Optional#empty()}
   * @return a handle on the submitted transaction
   */
  public SentTransaction transferLocked(
      final String from,
      final ZetoTransferLockedParams params,
      final Optional<String> idempotencyKey) {
    return sendPrivate("transferLocked", from, params, idempotencyKey);
  }

  /**
   * Locks tokens under a base-ledger delegate. The idempotency key prevents a retried submission
   * from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@link Optional#empty()}
   * @return a handle on the submitted transaction
   */
  public SentTransaction lock(
      final String from, final ZetoLockParams params, final Optional<String> idempotencyKey) {
    return sendPrivate("lock", from, params, idempotencyKey);
  }

  /**
   * Delegates spend authority for on-chain UTXOs with a public transaction on the base ledger. The
   * idempotency key prevents a retried submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@link Optional#empty()}
   * @return a handle on the submitted transaction
   */
  public SentTransaction delegateLock(
      final String from,
      final ZetoDelegateLockParams params,
      final Optional<String> idempotencyKey) {
    Objects.requireNonNull(params, "params");
    final Map<String, Object> inputs =
        Map.of(
            "utxos",
            params.utxos(),
            "delegate",
            params.delegate(),
            "data",
            ZetoDefaults.EMPTY_BYTES);
    return invoke("delegateLock", from, inputs)
        .publicTx()
        .abi(ZetoAbi.PUBLIC)
        .idempotencyKey(Objects.requireNonNull(idempotencyKey, "idempotencyKey").orElse(null))
        .send();
  }

  /**
   * Records the ERC-20 contract bridged to this token and submits a public {@code setERC20}
   * transaction. The idempotency key prevents a retried submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@link Optional#empty()}
   * @return a handle on the submitted transaction
   */
  public SentTransaction setErc20(
      final String from, final ZetoSetErc20Params params, final Optional<String> idempotencyKey) {
    Objects.requireNonNull(params, "params");
    erc20 = params.erc20();
    return invoke("setERC20", from, params)
        .publicTx()
        .abi(ZetoAbi.PRIVATE)
        .idempotencyKey(Objects.requireNonNull(idempotencyKey, "idempotencyKey").orElse(null))
        .send();
  }

  /**
   * Deposits ERC-20 tokens into Zeto UTXOs. The idempotency key prevents a retried submission from
   * executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@link Optional#empty()}
   * @return a handle on the submitted transaction
   */
  public SentTransaction deposit(
      final String from, final ZetoDepositParams params, final Optional<String> idempotencyKey) {
    return sendPrivate("deposit", from, params, idempotencyKey);
  }

  /**
   * Withdraws Zeto UTXOs to ERC-20. The idempotency key prevents a retried submission from
   * executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@link Optional#empty()}
   * @return a handle on the submitted transaction
   */
  public SentTransaction withdraw(
      final String from, final ZetoWithdrawParams params, final Optional<String> idempotencyKey) {
    return sendPrivate("withdraw", from, params, idempotencyKey);
  }

  /**
   * Reads the balance of an account, as visible to the calling identity.
   *
   * @param from lookup string of the identity making the call
   * @param account lookup string of the account to read
   * @return a future completing with the balance
   */
  public CompletableFuture<ZetoBalanceOfResult> balanceOf(final String from, final String account) {
    return call("balanceOf", from, Map.of("account", Objects.requireNonNull(account, "account")))
        .thenApply(
            result -> PaladinObjectMapper.shared().convertValue(result, ZetoBalanceOfResult.class));
  }

  private SentTransaction sendPrivate(
      final String function,
      final String from,
      final Object params,
      final Optional<String> idempotencyKey) {
    return invoke(function, from, params)
        .privateTx()
        .domain(domain)
        .abi(ZetoAbi.PRIVATE)
        .idempotencyKey(Objects.requireNonNull(idempotencyKey, "idempotencyKey").orElse(null))
        .send();
  }

  private CompletableFuture<JsonNode> call(
      final String function, final String from, final Object params) {
    return invoke(function, from, params).privateTx().domain(domain).abi(ZetoAbi.PRIVATE).call();
  }

  private TxBuilder invoke(final String function, final String from, final Object params) {
    return paladin
        .newTx()
        .function(function)
        .to(address)
        .from(from)
        .inputs(Objects.requireNonNull(params, "params"));
  }
}
