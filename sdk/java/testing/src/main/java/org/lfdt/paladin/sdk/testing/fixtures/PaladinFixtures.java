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
package org.lfdt.paladin.sdk.testing.fixtures;

import java.security.SecureRandom;
import java.util.UUID;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;

/**
 * Ready-made values for tests: random identifiers and receipts in the shapes a node returns.
 *
 * <p>Every receipt factory returns a {@link TransactionReceipt.Builder} pre-filled for the case, so
 * a test can override just the field it cares about before {@code build()}.
 */
public final class PaladinFixtures {

  private static final SecureRandom RANDOM = new SecureRandom();

  private PaladinFixtures() {}

  /**
   * A random Ethereum address.
   *
   * @return a new address
   */
  public static EthAddress randomAddress() {
    return EthAddress.wrap(randomBytes(EthAddress.SIZE));
  }

  /**
   * A random 32-byte value, e.g. a transaction hash.
   *
   * @return a new value
   */
  public static Bytes32 randomBytes32() {
    return Bytes32.wrap(randomBytes(Bytes32.SIZE));
  }

  /**
   * A unique key identifier on a node, e.g. {@code test.3f2a….node1}, so repeated runs against a
   * long-lived node never collide.
   *
   * @param node the node name
   * @return a new identity
   */
  public static String uniqueIdentity(final String node) {
    return "test." + UUID.randomUUID() + "." + node;
  }

  /**
   * A successful public-transaction receipt mined in block 1.
   *
   * @param id the transaction id
   * @return a pre-filled builder
   */
  public static TransactionReceipt.Builder successReceipt(final UUID id) {
    return TransactionReceipt.builder()
        .id(id)
        .sequence(1)
        .success(true)
        .transactionHash(randomBytes32())
        .blockNumber(1L)
        .transactionIndex(0L)
        .logIndex(0L)
        .source(randomAddress());
  }

  /**
   * A successful deployment receipt carrying the new contract's address.
   *
   * @param id the transaction id
   * @param contractAddress the deployed contract
   * @return a pre-filled builder
   */
  public static TransactionReceipt.Builder deployReceipt(
      final UUID id, final EthAddress contractAddress) {
    return successReceipt(id).contractAddress(contractAddress);
  }

  /**
   * A failed receipt, as for a reverted transaction.
   *
   * @param id the transaction id
   * @param failureMessage the node's failure message
   * @return a pre-filled builder
   */
  public static TransactionReceipt.Builder failedReceipt(
      final UUID id, final String failureMessage) {
    return successReceipt(id).success(false).failureMessage(failureMessage);
  }

  private static byte[] randomBytes(final int size) {
    final byte[] bytes = new byte[size];
    RANDOM.nextBytes(bytes);
    return bytes;
  }
}
