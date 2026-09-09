/*
 * Copyright © 2026 Kaleido, Inc.
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
    `notary@${nodeConnections[0].id}`,
  );
  const [sender] = paladinClientNode1.getVerifiers(
    `sender@${nodeConnections[0].id}`,
  );
  const [recipient] = paladinClientNode2.getVerifiers(
    `user@${nodeConnections[1].id}`,
  );

  const mintAmount = 1000;
  const transferLockAmount = 300;

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

  // Step 2: Mint tokens to the sender so they have something to lock
  logger.log(`Step 2: Minting ${mintAmount} units to sender...`);
  const mintReceipt = await token
    .mint(notary, {
      to: sender,
      amount: mintAmount,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!mintReceipt?.success) {
    logger.error("mint transaction failed!");
    return false;
  }
  const senderBalanceAfterMint = await token.balanceOf(sender, {
    account: sender.lookup,
  });
  logger.log(`Sender balance after mint: ${senderBalanceAfterMint.totalBalance}`);
  assert(
    senderBalanceAfterMint.totalBalance === mintAmount.toString(),
    `Expected sender balance to be ${mintAmount}, got ${senderBalanceAfterMint.totalBalance}`,
  );

  // Step 3: Sender creates a transfer lock with a recipient
  // Unlike a direct transfer, this locks the sender's tokens; the recipient
  // only receives them once the lock is spent.
  logger.log(
    `Step 3: Sender creating transfer lock for ${transferLockAmount} units to recipient...`,
  );
  const createTransferLockReceipt = await token
    .createTransferLock(sender, {
      from: sender,
      recipients: [{ to: recipient, amount: transferLockAmount }],
      unlockData: "0x",
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!createTransferLockReceipt?.success) {
    logger.error("createTransferLock transaction failed!");
    return false;
  }
  logger.log(
    `createTransferLock succeeded: txId=${createTransferLockReceipt.id}`,
  );

  // Step 4: Retrieve lock info from the domain receipt
  logger.log("Step 4: Retrieving lock info from domain receipt...");
  const domainReceipt = (await paladinClientNode1.ptx.getDomainReceipt(
    "noto",
    createTransferLockReceipt.id,
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

  // The sender's spendable balance is reduced by the locked amount, but any
  // remainder coin was returned to the sender immediately. The recipient does
  // NOT yet have the tokens.
  const senderBalanceAfterLock = await token.balanceOf(sender, {
    account: sender.lookup,
  });
  logger.log(
    `Sender spendable balance after lock: ${senderBalanceAfterLock.totalBalance}`,
  );
  const expectedSenderBalanceAfterLock = mintAmount - transferLockAmount;
  assert(
    senderBalanceAfterLock.totalBalance === expectedSenderBalanceAfterLock.toString(),
    `Expected sender balance to be ${expectedSenderBalanceAfterLock}, got ${senderBalanceAfterLock.totalBalance}`,
  );

  const recipientBalanceBefore = await token
    .using(paladinClientNode2)
    .balanceOf(recipient, { account: recipient.lookup });
  logger.log(
    `Recipient balance before spend: ${recipientBalanceBefore.totalBalance}`,
  );
  assert(
    recipientBalanceBefore.totalBalance === "0",
    "Recipient should have 0 balance before lock is spent",
  );

  // Step 5: Delegate the lock to the recipient
  // This allows the recipient (rather than the sender) to spend the lock.
  logger.log("Step 5: Delegating lock to recipient...");
  const recipientAddr = await paladinClientNode1.ptx.resolveVerifier(
    recipient.lookup,
    Algorithms.ECDSA_SECP256K1,
    Verifiers.ETH_ADDRESS,
  );
  const delegateReceipt = await token
    .delegateLock(sender, {
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

  // Step 6: Spend the lock (recipient finalizes the transfer)
  // Use the unlockParams from the domain receipt — they contain the lockId,
  // encoded spendInputs, and data needed to execute the spend.
  logger.log("Step 6: Spending the lock to finalize transfer...");
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

  // Step 7: Validate final balances
  logger.log("Step 7: Validating final balances...");
  const recipientBalanceAfter = await token
    .using(paladinClientNode2)
    .balanceOf(recipient, { account: recipient.lookup });
  logger.log(
    `Recipient balance after spend: ${recipientBalanceAfter.totalBalance}`,
  );
  assert(
    recipientBalanceAfter.totalBalance === transferLockAmount.toString(),
    `Expected recipient balance to be ${transferLockAmount}, got ${recipientBalanceAfter.totalBalance}`,
  );

  const senderBalanceFinal = await token.balanceOf(sender, {
    account: sender.lookup,
  });
  logger.log(`Sender final balance: ${senderBalanceFinal.totalBalance}`);
  const expectedSenderFinal = mintAmount - transferLockAmount;
  assert(
    senderBalanceFinal.totalBalance === expectedSenderFinal.toString(),
    `Expected sender final balance to be ${expectedSenderFinal}, got ${senderBalanceFinal.totalBalance}`,
  );

  logger.log("All createTransferLock operations completed successfully!");
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
