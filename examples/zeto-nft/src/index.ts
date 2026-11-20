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
import PaladinClient, { ZetoFactory } from "@lfdecentralizedtrust/paladin-sdk";
import * as fs from "fs";
import * as path from "path";
import {
  checkDeploy,
  checkReceipt,
  nodeConnections,
  getCachePath,
  DEFAULT_POLL_TIMEOUT,
} from "paladin-example-common";
import { ContractData } from "./tests/data-persistence";
import { newHolders, transferNFT, waitForOwnedNFTs } from "./nft";

const logger = console;

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 3) {
    logger.error("The environment config must provide at least 3 nodes for this scenario.");
    return false;
  }

  logger.log("Initializing Paladin clients from the environment configuration...");
  const clients = nodeConnections.map((node) => new PaladinClient(node.clientOptions));
  const [paladin1] = clients;

  const [issuer] = paladin1.getVerifiers(`issuer@${nodeConnections[0].id}`);
  const [alice, bob, carol] = newHolders(clients);

  const uri = "https://example.com/artwork/1";

  logger.log("Step 1: Deploying a Zeto_NfAnon token...");
  const zetoFactory = new ZetoFactory(paladin1, "zeto");
  const artwork = await zetoFactory
    .newZetoNonFungible(issuer, { tokenName: "Zeto_NfAnon" })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!checkDeploy(artwork)) return false;

  logger.log(`Step 2: Minting an NFT to ${alice.verifier}...`);
  const mintReceipt = await artwork
    .mint(issuer, { mints: [{ to: alice.verifier, uri }] })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(mintReceipt)) return false;

  const minted = await waitForOwnedNFTs(alice.paladin, artwork.address, alice.verifier, 1, DEFAULT_POLL_TIMEOUT);
  if (minted === undefined || minted[0].uri !== uri) {
    logger.error(`${alice.verifier} does not hold the minted NFT`);
    return false;
  }
  const tokenID = minted[0].tokenID;
  logger.log(`${alice.verifier} holds NFT ${tokenID} with URI ${uri}`);

  logger.log("Step 3: Transferring the NFT from node1 to node2...");
  if (!(await transferNFT(artwork, alice, bob, tokenID))) return false;

  logger.log("Step 4: Transferring the received NFT on from node2 to node3...");
  if (!(await transferNFT(artwork, bob, carol, tokenID))) return false;

  const contractData: ContractData = {
    zetoAddress: artwork.address,
    tokenName: "Zeto_NfAnon",
    tokenID,
    uri,
    holder: carol.verifier.lookup,
    timestamp: new Date().toISOString(),
  };

  const dataDir = getCachePath();
  if (!fs.existsSync(dataDir)) {
    fs.mkdirSync(dataDir, { recursive: true });
  }
  const timestamp = new Date().toISOString().replace(/[:.]/g, "-");
  const dataFile = path.join(dataDir, `contract-data-${timestamp}.json`);
  fs.writeFileSync(dataFile, JSON.stringify(contractData, null, 2));
  logger.log(`Contract data saved to ${dataFile}`);

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
