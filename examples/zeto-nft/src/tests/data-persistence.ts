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
import PaladinClient, { ZetoNonFungibleInstance } from "@lfdecentralizedtrust/paladin-sdk";
import * as fs from "fs";
import * as path from "path";
import { nodeConnections, findLatestContractDataFile, getCachePath } from "paladin-example-common";
import { newHolders, ownedNFTs, sameTokenID, transferNFT } from "../nft";

const logger = console;

export interface ContractData {
  zetoAddress: string;
  tokenName: string;
  tokenID: string;
  uri: string;
  holder: string;
  timestamp: string;
}

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 3) {
    logger.error("The environment config must provide at least 3 nodes for this scenario.");
    return false;
  }

  logger.log("Initializing Paladin clients from the environment configuration...");
  const clients = nodeConnections.map((node) => new PaladinClient(node.clientOptions));
  const holders = newHolders(clients);

  // STEP 1: Load the saved contract data
  logger.log("STEP 1: Loading saved contract data...");
  const dataDir = getCachePath();
  const dataFile = findLatestContractDataFile(dataDir);
  if (!dataFile) {
    logger.error(`STEP 1: No contract data files found in ${dataDir}`);
    logger.error("Please run the original script first to deploy the contracts and save the data.");
    return false;
  }

  const contractData: ContractData = JSON.parse(fs.readFileSync(dataFile, "utf8"));
  logger.log(`STEP 1: Loaded contract data from ${dataFile}`);

  logger.log("\n=== CACHED DATA SUMMARY ===");
  logger.log(`Data File: ${dataFile}`);
  logger.log(`Timestamp: ${contractData.timestamp}`);
  logger.log(`Zeto Address: ${contractData.zetoAddress}`);
  logger.log(`Token Name: ${contractData.tokenName}`);
  logger.log(`Token ID: ${contractData.tokenID}`);
  logger.log(`URI: ${contractData.uri}`);
  logger.log(`Holder: ${contractData.holder}`);
  logger.log("=============================\n");

  const holderIndex = holders.findIndex((h) => h.verifier.lookup === contractData.holder);
  if (holderIndex < 0) {
    logger.error(`STEP 1: ERROR - saved holder ${contractData.holder} is not one of this example's identities`);
    return false;
  }
  const holder = holders[holderIndex];
  const artwork = new ZetoNonFungibleInstance(holder.paladin, contractData.zetoAddress);

  // STEP 2: Verify the saved holder still holds the NFT
  logger.log(`STEP 2: Verifying ${holder.verifier} still holds the NFT...`);
  const held = await ownedNFTs(holder.paladin, artwork.address, holder.verifier);
  if (held.length !== 1 || !sameTokenID(held[0].tokenID, contractData.tokenID) || held[0].uri !== contractData.uri) {
    logger.error(`STEP 2: ERROR - ${holder.verifier} does not hold NFT ${contractData.tokenID}`);
    logger.error(`Found: ${JSON.stringify(held)}`);
    return false;
  }
  logger.log("STEP 2: NFT ownership verification successful!");

  // STEP 3: Test token functionality by transferring the NFT on to the next node
  const next = holders[(holderIndex + 1) % holders.length];
  logger.log(`STEP 3: Transferring the NFT to ${next.verifier}...`);
  if (!(await transferNFT(artwork, holder, next, contractData.tokenID))) {
    logger.error("STEP 3: Test transfer failed!");
    return false;
  }
  logger.log("STEP 3: Test transfer verification successful!");

  const newContractData: ContractData = {
    ...contractData,
    holder: next.verifier.lookup,
    timestamp: new Date().toISOString(),
  };
  const timestamp = new Date().toISOString().replace(/[:.]/g, "-");
  const newDataFile = path.join(dataDir, `contract-data-${timestamp}.json`);
  fs.writeFileSync(newDataFile, JSON.stringify(newContractData, null, 2));
  logger.log(`New contract data saved to ${newDataFile}`);

  logger.log("\nSUCCESS: Verification completed!");

  return true;
}

if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1);
    })
    .catch((err) => {
      console.error("Exiting with uncaught error");
      console.error(err);
      process.exit(1);
    });
}
