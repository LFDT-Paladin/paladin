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
import java.time.Duration;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.exception.PaladinException;
import org.lfdt.paladin.sdk.client.ptx.PtxClient;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;
import org.lfdt.paladin.sdk.core.types.EthAddress;

/**
 * An in-flight private contract deployment inside a privacy group, returned by {@link
 * PentePrivacyGroup#deploy}. The contract address is private to the group, so it is read from the
 * Pente domain receipt rather than the base-ledger receipt.
 */
public final class PentePrivateDeployment {

  private final PtxClient ptx;
  private final SentTransaction transaction;

  PentePrivateDeployment(final PtxClient ptx, final SentTransaction transaction) {
    this.ptx = ptx;
    this.transaction = transaction;
  }

  /**
   * The submitted deployment transaction.
   *
   * @return the transaction handle
   */
  public SentTransaction transaction() {
    return transaction;
  }

  /**
   * The node-assigned id of the deployment transaction.
   *
   * @return a future completing with the transaction id
   */
  public CompletableFuture<UUID> id() {
    return transaction.id();
  }

  /**
   * Waits for the deployment to complete, using the default receipt timeout.
   *
   * @return a future completing with the private contract address; failing with a {@link
   *     PaladinException} if the deployment failed or its domain receipt has no contract address,
   *     or with the errors described on {@link SentTransaction#waitForReceipt()}
   */
  public CompletableFuture<EthAddress> waitForDeploy() {
    return transaction.waitForReceipt().thenCompose(this::contractAddress);
  }

  /**
   * Waits for the deployment to complete, with an explicit timeout.
   *
   * @param timeout the total time to wait for the receipt; must be positive
   * @return a future completing with the private contract address; failing with a {@link
   *     PaladinException} if the deployment failed or its domain receipt has no contract address,
   *     or with the errors described on {@link SentTransaction#waitForReceipt(Duration)}
   */
  public CompletableFuture<EthAddress> waitForDeploy(final Duration timeout) {
    return transaction.waitForReceipt(timeout).thenCompose(this::contractAddress);
  }

  private CompletableFuture<EthAddress> contractAddress(final TransactionReceipt receipt) {
    if (!receipt.success()) {
      return CompletableFuture.failedFuture(
          new PaladinException(
              "private deployment " + receipt.id() + " failed: " + receipt.failureMessage()));
    }
    return ptx.getTransactionReceiptFull(receipt.id())
        .thenApply(
            full -> {
              final JsonNode address =
                  full.domainReceipt() == null
                      ? null
                      : full.domainReceipt().path("receipt").get("contractAddress");
              if (address == null || !address.isTextual()) {
                throw new PaladinException(
                    "private deployment "
                        + receipt.id()
                        + " has no contract address in its domain receipt");
              }
              return EthAddress.fromString(address.asText());
            });
  }
}
