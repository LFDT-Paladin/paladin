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
package org.lfdt.paladin.sdk.testing.containers;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertSame;

import java.time.Duration;
import java.util.Map;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.junit.jupiter.api.extension.ExtendWith;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.testing.assertions.Await;
import org.lfdt.paladin.sdk.testing.assertions.PaladinAssertions;
import org.lfdt.paladin.sdk.testing.fixtures.PaladinFixtures;

/**
 * Boots Besu and one Paladin node in Docker and drives them through the SDK. Run with {@code
 * ./gradlew :sdk:java:testing:containerTest}.
 */
@Tag("testcontainers")
@ExtendWith(PaladinNetworkExtension.class)
@Timeout(600)
class PaladinNodeSmokeTest {

  /** A constructor with no inputs; the one-byte runtime ({@code STOP}) is all a deploy needs. */
  private static final String EMPTY_CONSTRUCTOR_ABI = "[{\"type\":\"constructor\",\"inputs\":[]}]";

  @Test
  void nodeServesJsonRpcAndIndexesTheChain(
      final PaladinClient client, final PaladinTestNetwork network) {
    assertSame(network.client(PaladinTestNetwork.DEFAULT_NODE), client);
    assertFalse(client.keyManager().wallets().join().isEmpty());
    Await.until(
        "the node to index past block 1",
        Duration.ofSeconds(60),
        Duration.ofMillis(500),
        () -> client.blockIndex().getConfirmedBlockHeight().join(),
        height -> height != null && height.asUnsignedLong() > 1);
  }

  @Test
  void publicDeploymentReachesTheBaseLedger(final PaladinClient client) {
    final String deployer = PaladinFixtures.uniqueIdentity(PaladinTestNetwork.DEFAULT_NODE);
    final EthAddress signer = client.keyManager().resolveEthAddress(deployer).join();
    assertNotNull(signer);

    final TransactionReceipt receipt =
        PaladinAssertions.assertSuccess(
            client
                .newTx()
                .publicTx()
                .from(deployer)
                .constructor()
                .abiJson(EMPTY_CONSTRUCTOR_ABI)
                .bytecode("0x00")
                .inputs(Map.of())
                .send(),
            Duration.ofSeconds(60));
    assertNotNull(receipt.contractAddress());
    assertEquals(signer, receipt.source());
  }

  @Test
  void containerExposesItsEndpoints(final PaladinNodeContainer node) {
    assertEquals(PaladinTestNetwork.DEFAULT_NODE, node.nodeName());
    assertFalse(node.rpcUrl().isEmpty());
    assertFalse(node.wsUrl().isEmpty());
  }
}
