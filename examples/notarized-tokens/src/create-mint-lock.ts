/*
 * Copyright © 2025 Kaleido, Inc.
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
import PaladinClient, {
  NotoFactory,
  Algorithms,
  Verifiers,
  INotoDomainReceipt,
  INotoSpendLockParams,
} from "@lfdecentralizedtrust/paladin-sdk";
import {
  nodeConnections,
  DEFAULT_POLL_TIMEOUT,
  LONG_POLL_TIMEOUT,
} from "paladin-example-common";
import assert from "assert";

const logger = console;

async function main(): Promise<boolean> {
  // --- Initialization ---
  if (nodeConnections.length < 2) {
    logger.error(
      "The environment config must provide at least 2 nodes for this scenario.",
    );
    return false;
  }

  logger.log("Initializing Paladin clients...");
  const paladinClientNode1 = new PaladinClient(
    nodeConnections[0].clientOptions,
  );
  const paladinClientNode2 = new PaladinClient(
    nodeConnections[1].clientOptions,
  );

  const [notary] = paladinClientNode1.getVerifiers(
    `user@${nodeConnections[0].id}`,
  );
  const [recipient] = paladinClientNode2.getVerifiers(
    `user@${nodeConnections[1].id}`,
  );

  const mintLockAmount = 500;

  // Step 1: Deploy a Noto token
  logger.log("Step 1: Deploying Noto token...");
  const notoFactory = new NotoFactory(paladinClientNode1, "noto");
  const token = await notoFactory
    .newNoto(notary, {
      name: "NOTO",
      symbol: "NOTO",
      notary: notary,
      notaryMode: "basic",
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!token) {
    logger.error("Failed to deploy Noto token!");
    return false;
  }
  logger.log(`Noto token deployed at ${token.address}`);

  // Step 2: Create a mint lock with a recipient
  // Unlike a direct mint, this creates a lock that must be spent to finalize.
  logger.log(
    `Step 2: Creating mint lock for ${mintLockAmount} units to recipient...`,
  );
  const createMintLockReceipt = await token
    .createMintLock(notary, {
      recipients: [{ to: recipient, amount: mintLockAmount }],
      unlockData: "0x",
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!createMintLockReceipt?.success) {
    logger.error("createMintLock transaction failed!");
    return false;
  }
  logger.log(`createMintLock succeeded: txId=${createMintLockReceipt.id}`);

  // Step 3: Retrieve lock info from the domain receipt
  logger.log("Step 3: Retrieving lock info from domain receipt...");
  const domainReceipt = (await paladinClientNode1.ptx.getDomainReceipt(
    "noto",
    createMintLockReceipt.id,
  )) as INotoDomainReceipt | undefined;
  if (!domainReceipt?.lockInfo) {
    logger.error("Domain receipt missing lock info!");
    return false;
  }
  const lockInfo = domainReceipt.lockInfo;
  logger.log(
    `Lock created: lockId=${lockInfo.lockId}, unlockFunction=${lockInfo.unlockFunction}`,
  );
  assert(
    lockInfo.unlockFunction === "spendLock",
    "Expected unlockFunction to be 'spendLock'",
  );

  // At this point, the recipient does NOT yet have the tokens.
  const balanceBefore = await token
    .using(paladinClientNode2)
    .balanceOf(recipient, { account: recipient.lookup });
  logger.log(`Recipient balance before spend: ${balanceBefore.totalBalance}`);
  assert(
    balanceBefore.totalBalance === "0",
    "Recipient should have 0 balance before lock is spent",
  );

  // Step 4: Delegate the lock to the recipient
  // This allows the recipient (rather than the notary) to spend the lock.
  logger.log("Step 4: Delegating lock to recipient...");
  const recipientAddr = await paladinClientNode1.ptx.resolveVerifier(
    recipient.lookup,
    Algorithms.ECDSA_SECP256K1,
    Verifiers.ETH_ADDRESS,
  );
  const delegateReceipt = await token
    .delegateLock(notary, {
      lockId: lockInfo.lockId,
      delegate: recipientAddr,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!delegateReceipt?.success) {
    logger.error("delegateLock transaction failed!");
    return false;
  }
  logger.log("Lock delegated to recipient successfully.");

  // Step 5: Spend the lock (recipient finalizes the mint)
  // Use the unlockParams from the domain receipt — they contain the lockId,
  // encoded spendInputs, and data needed to execute the spend.
  logger.log("Step 5: Spending the lock to finalize mint...");
  const unlockParams = lockInfo.unlockParams as INotoSpendLockParams;
  const spendReceipt = await token
    .using(paladinClientNode2)
    .spendLock(recipient, unlockParams)
    .waitForReceipt(LONG_POLL_TIMEOUT);
  if (!spendReceipt?.success) {
    logger.error("spendLock transaction failed!");
    return false;
  }
  logger.log(`Lock spent successfully: txId=${spendReceipt.id}`);

  await new Promise((resolve) => setTimeout(resolve, 5000));

  // Step 6: Validate recipient received the tokens
  // Query from node2's perspective since the recipient's states are on node2
  logger.log("Step 6: Validating final balances...");
  const balanceAfter = await token
    .using(paladinClientNode2)
    .balanceOf(recipient, { account: recipient.lookup });
  logger.log(`Recipient balance after spend: ${balanceAfter.totalBalance}`);
  assert(
    balanceAfter.totalBalance === mintLockAmount.toString(),
    `Expected recipient balance to be ${mintLockAmount}, got ${balanceAfter.totalBalance}`,
  );

  logger.log("All createMintLock operations completed successfully!");
  return true;
}

if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1);
    })
    .catch((err) => {
      logger.error("Exiting due to an uncaught error:", err);
      process.exit(1);
    });
}
