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
  NotoInstance,
} from "@lfdecentralizedtrust/paladin-sdk";
import * as fs from "fs";
import { nodeConnections, findLatestContractDataFile, getCachePath, DEFAULT_POLL_TIMEOUT } from "paladin-example-common";
import { ContractData } from "../create-mint-lock";

const logger = console;

async function main(): Promise<boolean> {
  if (nodeConnections.length < 2) {
    logger.error("The environment config must provide at least 2 nodes for this scenario.");
    return false;
  }

  logger.log("Initializing Paladin clients from the environment configuration...");
  const paladinClientNode1 = new PaladinClient(nodeConnections[0].clientOptions);
  const paladinClientNode2 = new PaladinClient(nodeConnections[1].clientOptions);

  // STEP 1: Load the saved contract data
  logger.log("STEP 1: Loading saved contract data...");
  const dataDir = getCachePath();
  const dataFile = findLatestContractDataFile(dataDir);

  if (!dataFile) {
    logger.error(`STEP 1: No contract data files found in ${dataDir}`);
    logger.error("Please run the start script first to deploy the contract and save the data.");
    return false;
  }

  const contractData: ContractData = JSON.parse(fs.readFileSync(dataFile, "utf8"));
  logger.log(`STEP 1: Loaded contract data from ${dataFile}`);

  logger.log("\n=== CACHED DATA SUMMARY ===");
  logger.log(`Timestamp:              ${contractData.timestamp}`);
  logger.log(`Token Address:          ${contractData.tokenAddress}`);
  logger.log(`Notary Verifier:        ${contractData.notaryVerifier}`);
  logger.log(`Recipient Verifier:     ${contractData.recipientVerifier}`);
  logger.log(`Mint Lock Amount:       ${contractData.mintLockAmount}`);
  logger.log(`Recipient Final Balance: ${contractData.recipientFinalBalance}`);
  logger.log(`MintLock TX Hash:       ${contractData.mintLockTransactionHash}`);
  logger.log(`SpendLock TX Hash:      ${contractData.spendLockTransactionHash}`);
  logger.log("=============================\n");

  // STEP 2: Reconnect to the token
  logger.log("STEP 2: Reconnecting to the deployed Noto token...");
  const token = new NotoInstance(paladinClientNode1, contractData.tokenAddress);
  logger.log("STEP 2: Token connection established.");

  // STEP 3: Verify the recipient's balance matches what was recorded
  logger.log("STEP 3: Verifying recipient balance...");
  const notary = paladinClientNode1.getVerifiers(`user@${nodeConnections[0].id}`)[0];
  const recipient = paladinClientNode2.getVerifiers(`user@${nodeConnections[1].id}`)[0];

  try {
    const recipientBalance = await token.using(paladinClientNode2).balanceOf(recipient, {
      account: recipient.lookup,
    });

    logger.log(`STEP 3: Recipient balance: ${recipientBalance.totalBalance} (expected: ${contractData.recipientFinalBalance})`);

    if (recipientBalance.totalBalance !== contractData.recipientFinalBalance) {
      logger.error("STEP 3: ERROR - Recipient balance does not match saved data!");
      logger.error(`Expected: ${contractData.recipientFinalBalance}`);
      logger.error(`Found:    ${recipientBalance.totalBalance}`);
      return false;
    }

    logger.log("STEP 3: Recipient balance verified successfully!");
  } catch (error) {
    logger.error("STEP 3: Failed to retrieve recipient balance!");
    logger.error(error);
    return false;
  }

  // STEP 4: Verify the notary has no remaining balance (all tokens were minted directly to recipient)
  logger.log("STEP 4: Verifying notary balance is zero...");
  try {
    const notaryBalance = await token.balanceOf(notary, { account: notary.lookup });

    logger.log(`STEP 4: Notary balance: ${notaryBalance.totalBalance} (expected: 0)`);

    if (notaryBalance.totalBalance !== "0") {
      logger.error("STEP 4: ERROR - Notary balance should be 0 after a mint-lock flow!");
      logger.error(`Found: ${notaryBalance.totalBalance}`);
      return false;
    }

    logger.log("STEP 4: Notary balance verified successfully!");
  } catch (error) {
    logger.error("STEP 4: Failed to retrieve notary balance!");
    logger.error(error);
    return false;
  }

  // STEP 5: Verify the token is still functional by minting a small amount to the notary
  logger.log("STEP 5: Verifying token is still operational with a new mint...");
  const testMintAmount = 100;
  try {
    const mintReceipt = await token
      .mint(notary, {
        to: notary,
        amount: testMintAmount,
        data: "0x",
      })
      .waitForReceipt(DEFAULT_POLL_TIMEOUT);

    if (!mintReceipt?.success) {
      logger.error("STEP 5: Test mint failed!");
      return false;
    }

    logger.log(`STEP 5: Test mint of ${testMintAmount} units succeeded (txId=${mintReceipt.id})`);

    const notaryBalanceAfterMint = await token.balanceOf(notary, { account: notary.lookup });
    logger.log(`STEP 5: Notary balance after test mint: ${notaryBalanceAfterMint.totalBalance}`);

    if (notaryBalanceAfterMint.totalBalance !== testMintAmount.toString()) {
      logger.error("STEP 5: ERROR - Notary balance after test mint is incorrect!");
      logger.error(`Expected: ${testMintAmount}`);
      logger.error(`Found:    ${notaryBalanceAfterMint.totalBalance}`);
      return false;
    }

    logger.log("STEP 5: Token operational verification successful!");
  } catch (error) {
    logger.error("STEP 5: Token functionality test failed!");
    logger.error(error);
    return false;
  }

  logger.log("\nSUCCESS: Verification completed!");
  return true;
}

if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1);
    })
    .catch((err) => {
      logger.error("Exiting with uncaught error");
      logger.error(err);
      process.exit(1);
    });
}
