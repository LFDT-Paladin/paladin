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

import java.time.Duration;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.exception.PaladinException;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroup;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;

/**
 * An in-flight privacy group creation, returned by {@link PenteFactory#newPrivacyGroup}. Wait for
 * the group with {@link #waitForDeploy()}, or inspect the node's answer with {@link #group()}.
 */
public final class PentePrivacyGroupDeployment {

  private final PaladinClient paladin;
  private final CompletableFuture<PrivacyGroup> group;
  private final SentTransaction transaction;

  PentePrivacyGroupDeployment(
      final PaladinClient paladin, final CompletableFuture<PrivacyGroup> group) {
    this.paladin = paladin;
    this.group = group;
    this.transaction =
        SentTransaction.track(paladin.ptx(), group.thenApply(PentePrivacyGroupDeployment::genesis));
  }

  private static UUID genesis(final PrivacyGroup group) {
    if (group.genesisTransaction() == null) {
      throw new PaladinException("privacy group " + group.id() + " has no genesis transaction");
    }
    return group.genesisTransaction();
  }

  /**
   * The group as returned by the node on creation, before its genesis transaction is mined, so
   * without a contract address.
   *
   * @return a future completing with the created group
   */
  public CompletableFuture<PrivacyGroup> group() {
    return group;
  }

  /**
   * The genesis transaction the node submitted for the group.
   *
   * @return the transaction handle
   */
  public SentTransaction transaction() {
    return transaction;
  }

  /**
   * The node-assigned id of the genesis transaction.
   *
   * @return a future completing with the transaction id
   */
  public CompletableFuture<UUID> id() {
    return transaction.id();
  }

  /**
   * Waits for the group to be deployed, using the default receipt timeout.
   *
   * @return a future completing with the deployed group; failing with a {@link PaladinException} if
   *     the genesis transaction failed, or with the errors described on {@link
   *     SentTransaction#waitForReceipt()}
   */
  public CompletableFuture<PentePrivacyGroup> waitForDeploy() {
    return transaction.waitForReceipt().thenCombine(group, this::toGroup);
  }

  /**
   * Waits for the group to be deployed, with an explicit timeout.
   *
   * @param timeout the total time to wait; must be positive
   * @return a future completing with the deployed group; failing with a {@link PaladinException} if
   *     the genesis transaction failed, or with the errors described on {@link
   *     SentTransaction#waitForReceipt(Duration)}
   */
  public CompletableFuture<PentePrivacyGroup> waitForDeploy(final Duration timeout) {
    return transaction.waitForReceipt(timeout).thenCombine(group, this::toGroup);
  }

  private PentePrivacyGroup toGroup(final TransactionReceipt receipt, final PrivacyGroup created) {
    if (!receipt.success() || receipt.contractAddress() == null) {
      throw new PaladinException(
          "privacy group "
              + created.id()
              + " genesis transaction "
              + receipt.id()
              + " failed: "
              + receipt.failureMessage());
    }
    return new PentePrivacyGroup(paladin, created, receipt.contractAddress());
  }
}
